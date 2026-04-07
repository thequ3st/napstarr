package scanner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// ExtractArtwork reads embedded artwork from an audio file and writes it to disk.
// Skips if artwork already exists for the given album.
func ExtractArtwork(f *os.File, albumID string, artworkDir string) error {
	outPath := filepath.Join(artworkDir, albumID+".jpg")

	// Skip if already extracted.
	if _, err := os.Stat(outPath); err == nil {
		return nil
	}

	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seek: %w", err)
	}

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil
	}

	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil
	}

	if err := os.MkdirAll(artworkDir, 0755); err != nil {
		return fmt.Errorf("create artwork dir: %w", err)
	}

	return os.WriteFile(outPath, pic.Data, 0644)
}

// FetchArtwork tries to download album art from Cover Art Archive (MusicBrainz).
// Falls back gracefully — this is best-effort.
func FetchArtwork(artistName, albumTitle, albumID, artworkDir string) error {
	outPath := filepath.Join(artworkDir, albumID+".jpg")

	// Skip if already exists.
	if _, err := os.Stat(outPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(artworkDir, 0755); err != nil {
		return fmt.Errorf("create artwork dir: %w", err)
	}

	// Step 1: Search MusicBrainz for the release group
	mbid, err := searchMusicBrainz(artistName, albumTitle)
	if err != nil || mbid == "" {
		return fmt.Errorf("musicbrainz search: no results for %s - %s", artistName, albumTitle)
	}

	// Step 2: Fetch cover art from Cover Art Archive
	coverURL := fmt.Sprintf("https://coverartarchive.org/release-group/%s/front-500", mbid)
	resp, err := httpClient.Get(coverURL)
	if err != nil {
		return fmt.Errorf("cover art fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("cover art: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // 5MB max
	if err != nil {
		return fmt.Errorf("cover art read: %w", err)
	}

	return os.WriteFile(outPath, data, 0644)
}

func searchMusicBrainz(artist, album string) (string, error) {
	query := fmt.Sprintf(`releasegroup:"%s" AND artist:"%s"`,
		strings.ReplaceAll(album, `"`, ``),
		strings.ReplaceAll(artist, `"`, ``),
	)

	u := fmt.Sprintf("https://musicbrainz.org/ws/2/release-group/?query=%s&limit=1&fmt=json",
		url.QueryEscape(query))

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Napstarr/0.1 (https://github.com/thequ3st/napstarr)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("musicbrainz: HTTP %d", resp.StatusCode)
	}

	var result struct {
		ReleaseGroups []struct {
			ID    string `json:"id"`
			Score int    `json:"score"`
		} `json:"release-groups"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.ReleaseGroups) == 0 || result.ReleaseGroups[0].Score < 80 {
		return "", fmt.Errorf("no confident match")
	}

	return result.ReleaseGroups[0].ID, nil
}
