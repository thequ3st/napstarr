package scanner

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	var files []string
	err := filepath.WalkDir(musicDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := audioExts[ext]; ok {
			files = append(files, path)
		}
		return nil
	})
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

	return nil
}

func processFile(tx *sql.Tx, path string, musicDir string, artworkDir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	format := audioExts[ext]

	info, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := info.Size()

	// Read tags; tolerate missing metadata.
	m, tagErr := tag.ReadFrom(f)

	var artistName, albumTitle, trackTitle, genre string
	var year, trackNum, discNum int

	if tagErr == nil && m != nil {
		artistName = strings.TrimSpace(m.Artist())
		albumTitle = strings.TrimSpace(m.Album())
		trackTitle = strings.TrimSpace(m.Title())
		genre = strings.TrimSpace(m.Genre())
		year = m.Year()
		trackNum, _ = m.Track()
		discNum, _ = m.Disc()
	}

	// Fall back to directory names if tags are empty.
	if trackTitle == "" {
		base := filepath.Base(path)
		trackTitle = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if albumTitle == "" {
		albumTitle = filepath.Base(filepath.Dir(path))
	}
	if artistName == "" {
		albumDir := filepath.Dir(path)
		artistDir := filepath.Dir(albumDir)
		if artistDir != musicDir && artistDir != "." {
			artistName = filepath.Base(artistDir)
		} else {
			artistName = "Unknown Artist"
		}
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

	// Extract artwork (best-effort).
	if _, seekErr := f.Seek(0, 0); seekErr == nil {
		_ = ExtractArtwork(f, albumID, artworkDir)
	}

	// Mark album as having artwork if file exists.
	artFile := filepath.Join(artworkDir, albumID+".jpg")
	if _, statErr := os.Stat(artFile); statErr == nil {
		tx.Exec("UPDATE albums SET has_artwork = 1 WHERE id = ?", albumID)
	}

	return nil
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
