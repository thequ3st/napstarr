package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thequ3st/napstarr/internal/database"
)

// Deezer API types
type DeezerSearchResult struct {
	Data  []DeezerAlbum `json:"data"`
	Total int           `json:"total"`
}

type DeezerAlbum struct {
	ID          int          `json:"id"`
	Title       string       `json:"title"`
	Cover       string       `json:"cover"`
	CoverMedium string       `json:"cover_medium"`
	CoverBig    string       `json:"cover_big"`
	CoverXL     string       `json:"cover_xl"`
	NbTracks    int          `json:"nb_tracks"`
	ReleaseDate string       `json:"release_date"`
	Artist      DeezerArtist `json:"artist"`
	Genres      *DeezerGenres `json:"genres,omitempty"`
}

type DeezerArtist struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Picture   string `json:"picture"`
	PictureMedium string `json:"picture_medium"`
	PictureBig    string `json:"picture_big"`
	PictureXL     string `json:"picture_xl"`
	NbAlbum   int    `json:"nb_album"`
	NbFan     int    `json:"nb_fan"`
}

type DeezerGenres struct {
	Data []DeezerGenre `json:"data"`
}

type DeezerGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type DeezerAlbumFull struct {
	ID          int           `json:"id"`
	Title       string        `json:"title"`
	Cover       string        `json:"cover"`
	CoverMedium string        `json:"cover_medium"`
	CoverBig    string        `json:"cover_big"`
	CoverXL     string        `json:"cover_xl"`
	NbTracks    int           `json:"nb_tracks"`
	Duration    int           `json:"duration"`
	ReleaseDate string        `json:"release_date"`
	Genre       int           `json:"genre_id"`
	Genres      *DeezerGenres `json:"genres"`
	Artist      DeezerArtist  `json:"artist"`
	Tracks      *DeezerTracks `json:"tracks"`
}

type DeezerTracks struct {
	Data []DeezerTrack `json:"data"`
}

type DeezerTrack struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	TitleShort    string `json:"title_short"`
	Duration      int    `json:"duration"` // seconds
	TrackPosition int    `json:"track_position"` // may be 0 in album track lists
	DiskNumber    int    `json:"disk_number"`    // may be 0 in album track lists
	// Computed fields set during processing
	SeqPosition   int    // sequential position (1-based) from array order
	SeqDisc       int    // disc number derived from context
}

type DeezerArtistSearch struct {
	Data []DeezerArtist `json:"data"`
}

// EnrichFromDeezer enriches library metadata using the Deezer API.
// Faster than MusicBrainz — no auth, no strict rate limits.
func EnrichFromDeezer(db *database.DB, artworkDir string) {
	log.Println("metadata: starting Deezer enrichment")

	rows, err := db.Reader.Query("SELECT id, name FROM artists ORDER BY name")
	if err != nil {
		log.Printf("metadata: query artists: %v", err)
		return
	}

	type artistInfo struct {
		id, name string
	}
	var artists []artistInfo
	for rows.Next() {
		var a artistInfo
		rows.Scan(&a.id, &a.name)
		artists = append(artists, a)
	}
	rows.Close()

	log.Printf("metadata: Deezer enriching %d artists", len(artists))
	enriched := 0

	for _, artist := range artists {
		if enrichArtistDeezer(db, artist.id, artist.name, artworkDir) {
			enriched++
		}
		// Light rate limit to be polite
		time.Sleep(200 * time.Millisecond)
	}

	log.Printf("metadata: Deezer enrichment complete — %d/%d artists enriched", enriched, len(artists))
}

func enrichArtistDeezer(db *database.DB, artistID, artistName, artworkDir string) bool {
	// Search for artist
	dArtist, err := deezerSearchArtist(artistName)
	if err != nil || dArtist == nil {
		return false
	}

	log.Printf("metadata: Deezer matched %s → %s (id:%d)", artistName, dArtist.Name, dArtist.ID)

	// Get albums for this artist
	albumRows, err := db.Reader.Query("SELECT id, title, year FROM albums WHERE artist_id = ?", artistID)
	if err != nil {
		return false
	}

	type albumInfo struct {
		id, title string
		year      *int
	}
	var albums []albumInfo
	for albumRows.Next() {
		var a albumInfo
		albumRows.Scan(&a.id, &a.title, &a.year)
		albums = append(albums, a)
	}
	albumRows.Close()

	matched := 0
	for _, album := range albums {
		time.Sleep(150 * time.Millisecond)
		if enrichAlbumDeezer(db, album.id, artistName, album.title, dArtist.ID, artworkDir) {
			matched++
		}
	}

	return matched > 0
}

func enrichAlbumDeezer(db *database.DB, albumID, artistName, albumTitle string, deezerArtistID int, artworkDir string) bool {
	// Search for the album
	dAlbum, err := deezerSearchAlbum(artistName, albumTitle)
	if err != nil || dAlbum == nil {
		return false
	}

	// Update album year and genre
	year := parseYear(dAlbum.ReleaseDate)
	if year > 0 {
		db.Writer.Exec("UPDATE albums SET year = ? WHERE id = ? AND (year IS NULL OR year = 0)", year, albumID)
	}

	if dAlbum.Genres != nil && len(dAlbum.Genres.Data) > 0 {
		genre := dAlbum.Genres.Data[0].Name
		db.Writer.Exec("UPDATE albums SET genre = ? WHERE id = ? AND (genre = '' OR genre IS NULL)", genre, albumID)
	}

	// Fetch album art
	fetchDeezerArt(dAlbum.CoverXL, dAlbum.CoverBig, dAlbum.CoverMedium, albumID, artworkDir, db)

	// Get full album with tracks
	time.Sleep(150 * time.Millisecond)
	fullAlbum, err := deezerGetAlbum(dAlbum.ID)
	if err != nil || fullAlbum == nil || fullAlbum.Tracks == nil {
		return true // got album info even without tracks
	}

	// Match and update tracks
	enrichTracksDeezer(db, albumID, fullAlbum)

	return true
}

func enrichTracksDeezer(db *database.DB, albumID string, album *DeezerAlbumFull) {
	rows, err := db.Reader.Query(
		"SELECT id, title, track_number, disc_number FROM tracks WHERE album_id = ? ORDER BY disc_number, track_number",
		albumID)
	if err != nil {
		return
	}

	type localTrack struct {
		id    string
		title string
		num   *int
		disc  int
	}
	var localTracks []localTrack
	for rows.Next() {
		var t localTrack
		rows.Scan(&t.id, &t.title, &t.num, &t.disc)
		localTracks = append(localTracks, t)
	}
	rows.Close()

	dTracks := album.Tracks.Data

	// Compute sequential positions if Deezer doesn't provide them
	for i := range dTracks {
		dTracks[i].SeqPosition = i + 1
		dTracks[i].SeqDisc = 1
		if dTracks[i].TrackPosition > 0 {
			dTracks[i].SeqPosition = dTracks[i].TrackPosition
		}
		if dTracks[i].DiskNumber > 0 {
			dTracks[i].SeqDisc = dTracks[i].DiskNumber
		}
	}

	// Build a used-tracker to prevent double-matching
	used := make(map[int]bool)

	for i, local := range localTracks {
		var matched *DeezerTrack
		matchIdx := -1

		// 1. Match by disc + track number (if we have valid numbers)
		localDisc := local.disc
		localNum := 0
		if local.num != nil {
			localNum = *local.num
		}
		if localNum > 0 {
			for j := range dTracks {
				if !used[j] && dTracks[j].SeqDisc == localDisc && dTracks[j].SeqPosition == localNum {
					matched = &dTracks[j]
					matchIdx = j
					break
				}
			}
		}

		// 2. Match by title similarity
		if matched == nil {
			bestScore := 0.0
			for j := range dTracks {
				if used[j] {
					continue
				}
				s := titleSimilarity(local.title, dTracks[j].TitleShort)
				if s == 0 {
					s = titleSimilarity(local.title, dTracks[j].Title)
				}
				if s > bestScore && s > 0.3 {
					bestScore = s
					matched = &dTracks[j]
					matchIdx = j
				}
			}
		}

		// 3. Fall back to sequential index (only if counts match)
		if matched == nil && len(localTracks) == len(dTracks) && i < len(dTracks) && !used[i] {
			matched = &dTracks[i]
			matchIdx = i
		}

		if matched != nil && matchIdx >= 0 {
			used[matchIdx] = true
			title := matched.TitleShort
			if title == "" {
				title = matched.Title
			}
			durationMs := matched.Duration * 1000
			db.Writer.Exec(
				"UPDATE tracks SET title = ?, duration_ms = ?, disc_number = ?, track_number = ? WHERE id = ?",
				title, durationMs, matched.SeqDisc, matched.SeqPosition, local.id)
		}
	}
}

func fetchDeezerArt(xlURL, bigURL, medURL, albumID, artworkDir string, db *database.DB) {
	outPath := filepath.Join(artworkDir, albumID+".jpg")
	if _, err := os.Stat(outPath); err == nil {
		return // already have it
	}

	os.MkdirAll(artworkDir, 0755)

	// Try XL first, then big, then medium
	for _, artURL := range []string{xlURL, bigURL, medURL} {
		if artURL == "" {
			continue
		}
		resp, err := client.Get(artURL)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
		resp.Body.Close()
		if err != nil || len(data) < 1000 {
			continue // too small, probably a placeholder
		}
		os.WriteFile(outPath, data, 0644)
		db.Writer.Exec("UPDATE albums SET has_artwork = 1 WHERE id = ?", albumID)
		return
	}
}

// Deezer API calls

func deezerSearchArtist(name string) (*DeezerArtist, error) {
	u := fmt.Sprintf("https://api.deezer.com/search/artist?q=%s&limit=1", url.QueryEscape(name))
	data, err := deezerRequest(u)
	if err != nil {
		return nil, err
	}

	var result DeezerArtistSearch
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no match for %s", name)
	}

	// Verify name similarity
	if !similarName(name, result.Data[0].Name) {
		return nil, fmt.Errorf("name mismatch: %s vs %s", name, result.Data[0].Name)
	}

	return &result.Data[0], nil
}

func deezerSearchAlbum(artist, album string) (*DeezerAlbumFull, error) {
	q := fmt.Sprintf(`artist:"%s" album:"%s"`, artist, album)
	u := fmt.Sprintf("https://api.deezer.com/search/album?q=%s&limit=5", url.QueryEscape(q))
	data, err := deezerRequest(u)
	if err != nil {
		return nil, err
	}

	var result DeezerSearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no match for %s - %s", artist, album)
	}

	// Pick best match by title similarity
	best := &result.Data[0]
	bestScore := titleSimilarity(album, best.Title)
	for i := 1; i < len(result.Data); i++ {
		s := titleSimilarity(album, result.Data[i].Title)
		if s > bestScore {
			best = &result.Data[i]
			bestScore = s
		}
	}

	// Get full album info
	return deezerGetAlbum(best.ID)
}

func deezerGetAlbum(id int) (*DeezerAlbumFull, error) {
	u := fmt.Sprintf("https://api.deezer.com/album/%d", id)
	data, err := deezerRequest(u)
	if err != nil {
		return nil, err
	}

	var album DeezerAlbumFull
	if err := json.Unmarshal(data, &album); err != nil {
		return nil, err
	}
	return &album, nil
}

func deezerRequest(u string) ([]byte, error) {
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
}

// similarName checks if two artist names are similar enough.
func similarName(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == b {
		return true
	}
	// Handle "The X" vs "X"
	a = strings.TrimPrefix(a, "the ")
	b = strings.TrimPrefix(b, "the ")
	return a == b
}

// titleSimilarity scores how similar two album titles are (0-1).
func titleSimilarity(a, b string) float64 {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == b {
		return 1.0
	}
	// Check containment
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter := len(a)
		if len(b) < shorter {
			shorter = len(b)
		}
		longer := len(a)
		if len(b) > longer {
			longer = len(b)
		}
		return float64(shorter) / float64(longer)
	}
	return 0
}
