package scanner

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"github.com/thequ3st/napstarr/internal/database"
)

var audioExts = map[string]string{
	".flac": "flac",
	".mp3":  "mp3",
	".m4a":  "m4a",
	".ogg":  "ogg",
	".opus": "opus",
}

// Scan walks musicDir, reads audio metadata, upserts artists/albums/tracks,
// extracts artwork, and removes DB entries for files that no longer exist.
func Scan(db *database.DB, musicDir string, artworkDir string, progress func(int, int)) error {
	// Phase 1: collect all audio file paths.
	// Custom walk that follows symlinks (critical for FUSE/debrid mount setups
	// where directories are often symlinked).
	var files []string
	fmt.Fprintf(os.Stderr, "scanner: walking %s\n", musicDir)
	err := walkFollowSymlinks(musicDir, func(path string) {
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := audioExts[ext]; ok {
			files = append(files, path)
		}
	})
	fmt.Fprintf(os.Stderr, "scanner: walk complete, found %d files\n", len(files))
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	total := len(files)
	if progress != nil {
		progress(0, total)
	}

	// Phase 2: process files in batched transactions.
	const batchSize = 500
	seenPaths := make(map[string]bool, total)

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}
		batch := files[i:end]

		tx, err := db.Writer.Begin()
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}

		for _, path := range batch {
			seenPaths[path] = true
			if err := processFile(tx, path, musicDir, artworkDir); err != nil {
				// Log but don't abort the whole scan for one bad file.
				fmt.Fprintf(os.Stderr, "scanner: skipping %s: %v\n", path, err)
			}
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}

		if progress != nil {
			progress(end, total)
		}
	}

	// Phase 3: remove tracks whose files no longer exist.
	if err := cleanupMissing(db, seenPaths); err != nil {
		return fmt.Errorf("cleanup: %w", err)
	}

	// Phase 4: fetch missing album artwork from Cover Art Archive (async, non-blocking).
	go fetchMissingArtwork(db, artworkDir)

	return nil
}

func fetchMissingArtwork(db *database.DB, artworkDir string) {
	rows, err := db.Reader.Query(`
		SELECT al.id, ar.name, al.title
		FROM albums al
		JOIN artists ar ON ar.id = al.artist_id
		WHERE al.has_artwork = 0`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "artwork fetch query: %v\n", err)
		return
	}
	defer rows.Close()

	type albumInfo struct {
		id, artist, title string
	}
	var missing []albumInfo
	for rows.Next() {
		var a albumInfo
		rows.Scan(&a.id, &a.artist, &a.title)
		missing = append(missing, a)
	}

	fmt.Fprintf(os.Stderr, "artwork: fetching art for %d albums\n", len(missing))
	fetched := 0
	for _, a := range missing {
		if err := FetchArtwork(a.artist, a.title, a.id, artworkDir); err == nil {
			db.Writer.Exec("UPDATE albums SET has_artwork = 1 WHERE id = ?", a.id)
			fetched++
			fmt.Fprintf(os.Stderr, "artwork: fetched %s - %s (%d/%d)\n", a.artist, a.title, fetched, len(missing))
		}
		// Rate limit: MusicBrainz asks for 1 req/sec
		time.Sleep(1100 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "artwork: done, fetched %d/%d\n", fetched, len(missing))
}

func processFile(tx *sql.Tx, path string, musicDir string, artworkDir string) error {
	ext := strings.ToLower(filepath.Ext(path))
	format := audioExts[ext]

	var artistName, albumTitle, trackTitle, genre string
	var year, trackNum, discNum int
	var fileSize int64

	// Try to open and read tags — but don't fail if FUSE mount is unresponsive.
	f, openErr := os.Open(path)
	if openErr == nil {
		defer f.Close()

		if info, err := f.Stat(); err == nil {
			fileSize = info.Size()
		}

		if m, tagErr := tag.ReadFrom(f); tagErr == nil && m != nil {
			artistName = strings.TrimSpace(m.Artist())
			albumTitle = strings.TrimSpace(m.Album())
			trackTitle = strings.TrimSpace(m.Title())
			genre = strings.TrimSpace(m.Genre())
			year = m.Year()
			trackNum, _ = m.Track()
			discNum, _ = m.Disc()
		}

		// Try artwork extraction.
		if _, seekErr := f.Seek(0, 0); seekErr == nil {
			// defer artwork to after we have albumID
		}
	}

	// Always fall back to path-based metadata for anything missing.
	// Expected structure: musicDir/Artist/Album (Year)/Artist - Album - 01 - Track.flac
	base := filepath.Base(path)
	baseName := strings.TrimSuffix(base, filepath.Ext(base))
	albumDir := filepath.Dir(path)
	albumDirName := filepath.Base(albumDir)
	artistDir := filepath.Dir(albumDir)
	artistDirName := filepath.Base(artistDir)

	if trackTitle == "" {
		trackTitle = parseTrackTitle(baseName)
	}
	if albumTitle == "" {
		albumTitle, year = parseAlbumDir(albumDirName)
	}
	if artistName == "" {
		if artistDir != musicDir && artistDir != "." {
			artistName = artistDirName
		} else {
			artistName = "Unknown Artist"
		}
	}
	if trackNum == 0 {
		trackNum = parseTrackNum(baseName)
	}
	if discNum < 1 {
		discNum = 1
	}

	// Upsert artist.
	artistID, err := upsertArtist(tx, artistName)
	if err != nil {
		return fmt.Errorf("upsert artist: %w", err)
	}

	// Upsert album.
	var yearPtr *int
	if year > 0 {
		yearPtr = &year
	}
	albumID, err := upsertAlbum(tx, artistID, albumTitle, yearPtr, genre, discNum)
	if err != nil {
		return fmt.Errorf("upsert album: %w", err)
	}

	// Upsert track.
	var trackNumPtr *int
	if trackNum > 0 {
		trackNumPtr = &trackNum
	}
	err = upsertTrack(tx, albumID, artistID, trackTitle, trackNumPtr, discNum, path, fileSize, format)
	if err != nil {
		return fmt.Errorf("upsert track: %w", err)
	}

	// Try embedded artwork only (fast, no network). Network art fetch happens after scan.
	if f != nil {
		if _, seekErr := f.Seek(0, 0); seekErr == nil {
			_ = ExtractArtwork(f, albumID, artworkDir)
		}
	}

	artFile := filepath.Join(artworkDir, albumID+".jpg")
	if _, statErr := os.Stat(artFile); statErr == nil {
		tx.Exec("UPDATE albums SET has_artwork = 1 WHERE id = ?", albumID)
	}

	return nil
}

// parseTrackTitle extracts track title from filename like "Artist - Album - 01 - Track Title"
func parseTrackTitle(baseName string) string {
	parts := strings.Split(baseName, " - ")
	if len(parts) >= 4 {
		return strings.Join(parts[3:], " - ")
	}
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return baseName
}

// parseAlbumDir extracts album title and year from "Album Name (2006)"
func parseAlbumDir(dirName string) (string, int) {
	// Try to extract year from parentheses at the end
	if idx := strings.LastIndex(dirName, "("); idx > 0 {
		yearPart := strings.TrimRight(dirName[idx+1:], ")")
		yearPart = strings.TrimSpace(yearPart)
		if len(yearPart) == 4 {
			year := 0
			fmt.Sscanf(yearPart, "%d", &year)
			if year >= 1900 && year <= 2100 {
				return strings.TrimSpace(dirName[:idx]), year
			}
		}
	}
	return dirName, 0
}

// parseTrackNum extracts track number from filename like "Artist - Album - 01 - Track"
func parseTrackNum(baseName string) int {
	parts := strings.Split(baseName, " - ")
	if len(parts) >= 3 {
		num := 0
		fmt.Sscanf(strings.TrimSpace(parts[2]), "%d", &num)
		if num > 0 {
			return num
		}
	}
	// Try leading digits
	num := 0
	fmt.Sscanf(baseName, "%d", &num)
	return num
}

func computeSortName(name string) string {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "the ") && len(name) > 4 {
		return strings.TrimSpace(name[4:]) + ", " + name[:3]
	}
	return name
}

func upsertArtist(tx *sql.Tx, name string) (string, error) {
	lowerName := strings.ToLower(name)

	var id string
	err := tx.QueryRow("SELECT id FROM artists WHERE LOWER(name) = ?", lowerName).Scan(&id)
	if err == nil {
		return id, nil
	}

	id = database.NewID()
	sortName := computeSortName(name)
	_, err = tx.Exec(
		"INSERT INTO artists (id, name, sort_name) VALUES (?, ?, ?)",
		id, name, sortName,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func upsertAlbum(tx *sql.Tx, artistID, title string, year *int, genre string, discNum int) (string, error) {
	lowerTitle := strings.ToLower(title)

	var id string
	var err error
	if year != nil {
		err = tx.QueryRow(
			"SELECT id FROM albums WHERE artist_id = ? AND LOWER(title) = ? AND year = ?",
			artistID, lowerTitle, *year,
		).Scan(&id)
	} else {
		err = tx.QueryRow(
			"SELECT id FROM albums WHERE artist_id = ? AND LOWER(title) = ? AND year IS NULL",
			artistID, lowerTitle,
		).Scan(&id)
	}

	if err == nil {
		// Update disc_total if this disc number is higher.
		tx.Exec("UPDATE albums SET disc_total = MAX(disc_total, ?) WHERE id = ?", discNum, id)
		if genre != "" {
			tx.Exec("UPDATE albums SET genre = ? WHERE id = ? AND genre = ''", genre, id)
		}
		return id, nil
	}

	id = database.NewID()
	_, err = tx.Exec(
		"INSERT INTO albums (id, artist_id, title, year, genre, disc_total) VALUES (?, ?, ?, ?, ?, ?)",
		id, artistID, title, year, genre, discNum,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func upsertTrack(tx *sql.Tx, albumID, artistID, title string, trackNum *int, discNum int, filePath string, fileSize int64, format string) error {
	var existingID string
	err := tx.QueryRow("SELECT id FROM tracks WHERE file_path = ?", filePath).Scan(&existingID)

	if err == nil {
		// Update existing track.
		_, err = tx.Exec(
			`UPDATE tracks SET album_id = ?, artist_id = ?, title = ?, track_number = ?,
			 disc_number = ?, file_size = ?, format = ? WHERE id = ?`,
			albumID, artistID, title, trackNum, discNum, fileSize, format, existingID,
		)
		return err
	}

	id := database.NewID()
	_, err = tx.Exec(
		`INSERT INTO tracks (id, album_id, artist_id, title, track_number, disc_number,
		 file_path, file_size, format) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, albumID, artistID, title, trackNum, discNum, filePath, fileSize, format,
	)
	return err
}

// walkFollowSymlinks walks a directory tree, following symlinks into directories.
func walkFollowSymlinks(root string, fn func(path string)) error {
	return walkDir(root, fn, 0)
}

func walkDir(dir string, fn func(string), depth int) error {
	if depth > 20 {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanner: walkDir ReadDir error %s: %v\n", dir, err)
		return nil
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		info, err := os.Stat(path)
		if err != nil {
			// Stat failed — for FUSE mounts, the file might exist but not be stattable.
			// If the entry looks like an audio file, add it anyway.
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if _, ok := audioExts[ext]; ok {
				fn(path)
			} else if entry.Type()&os.ModeSymlink != 0 {
				// It's a symlink to something we can't stat — try ReadDir on it
				// in case it's a directory symlink pointing into FUSE
				subEntries, subErr := os.ReadDir(path)
				if subErr == nil && len(subEntries) > 0 {
					walkDir(path, fn, depth+1)
				}
			}
			continue
		}

		if info.IsDir() {
			walkDir(path, fn, depth+1)
		} else {
			fn(path)
		}
	}

	return nil
}

func cleanupMissing(db *database.DB, seenPaths map[string]bool) error {
	rows, err := db.Reader.Query("SELECT id, file_path FROM tracks")
	if err != nil {
		return err
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var id, fp string
		if err := rows.Scan(&id, &fp); err != nil {
			continue
		}
		if !seenPaths[fp] {
			toDelete = append(toDelete, id)
		}
	}

	if len(toDelete) == 0 {
		return nil
	}

	tx, err := db.Writer.Begin()
	if err != nil {
		return err
	}

	for _, id := range toDelete {
		tx.Exec("DELETE FROM tracks WHERE id = ?", id)
	}

	return tx.Commit()
}
