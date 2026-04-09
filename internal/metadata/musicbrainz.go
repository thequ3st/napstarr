package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/thequ3st/napstarr/internal/database"
)

var client = &http.Client{Timeout: 15 * time.Second}

const userAgent = "Napstarr/0.1 (https://github.com/thequ3st/napstarr)"

// MBSearchResult from release-group search.
type MBSearchResult struct {
	ReleaseGroups []MBReleaseGroup `json:"release-groups"`
}

type MBReleaseGroup struct {
	ID             string       `json:"id"`
	Title          string       `json:"title"`
	PrimaryType    string       `json:"primary-type"`
	FirstRelease   string       `json:"first-release-date"`
	Score          int          `json:"score"`
	ArtistCredit   []MBArtistCredit `json:"artist-credit"`
}

type MBArtistCredit struct {
	Artist MBArtist `json:"artist"`
}

type MBArtist struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SortName string `json:"sort-name"`
}

// MBRelease from release lookup.
type MBReleaseLookup struct {
	Releases []MBRelease `json:"releases"`
}

type MBRelease struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Date     string    `json:"date"`
	Country  string    `json:"country"`
	Status   string    `json:"status"`
	Media    []MBMedia `json:"media"`
}

type MBMedia struct {
	Position   int       `json:"position"`
	Format     string    `json:"format"`
	TrackCount int       `json:"track-count"`
	Tracks     []MBTrack `json:"tracks"`
}

type MBTrack struct {
	ID        string      `json:"id"`
	Number    string      `json:"number"`
	Title     string      `json:"title"`
	Length    int         `json:"length"` // milliseconds
	Position  int         `json:"position"`
	Recording MBRecording `json:"recording"`
}

type MBRecording struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Length int    `json:"length"`
}

// MBArtistLookup from artist search.
type MBArtistSearch struct {
	Artists []MBArtistFull `json:"artists"`
}

type MBArtistFull struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SortName string `json:"sort-name"`
	Type     string `json:"type"`
	Country  string `json:"country"`
	Score    int    `json:"score"`
	Tags     []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"tags"`
}

// EnrichLibrary runs metadata enrichment — Deezer first (fast), then MusicBrainz for gaps.
func EnrichLibrary(db *database.DB, artworkDir ...string) {
	log.Println("metadata: starting library enrichment")

	artDir := "/data/artwork"
	if len(artworkDir) > 0 && artworkDir[0] != "" {
		artDir = artworkDir[0]
	}

	// Phase 1: Deezer (fast, no rate limits)
	EnrichFromDeezer(db, artDir)

	// Phase 2: MusicBrainz for anything Deezer missed (slower, rate limited)
	enrichFromMusicBrainz(db)

	log.Println("metadata: enrichment complete")
}

func enrichFromMusicBrainz(db *database.DB) {
	// Only enrich artists/albums that Deezer didn't match (no musicbrainz_id yet)
	rows, err := db.Reader.Query("SELECT id, name FROM artists WHERE musicbrainz_id IS NULL OR musicbrainz_id = '' ORDER BY name")
	if err != nil {
		log.Printf("metadata: query unenriched artists: %v", err)
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

	if len(artists) == 0 {
		log.Println("metadata: MusicBrainz — nothing to enrich, Deezer got everything")
		return
	}

	log.Printf("metadata: MusicBrainz enriching %d remaining artists", len(artists))

	for i, artist := range artists {
		enrichArtist(db, artist.id, artist.name)
		time.Sleep(1100 * time.Millisecond)

		if (i+1)%10 == 0 {
			log.Printf("metadata: MusicBrainz enriched %d/%d artists", i+1, len(artists))
		}
	}
}

func enrichArtist(db *database.DB, artistID, artistName string) {
	// Search for the artist on MusicBrainz
	mbArtist, err := searchArtist(artistName)
	if err != nil || mbArtist == nil {
		return
	}

	// Update artist with MusicBrainz data
	db.Writer.Exec("UPDATE artists SET musicbrainz_id = ? WHERE id = ?",
		mbArtist.ID, artistID)

	// Now enrich each album for this artist
	albumRows, err := db.Reader.Query("SELECT id, title, year FROM albums WHERE artist_id = ?", artistID)
	if err != nil {
		return
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

	for _, album := range albums {
		time.Sleep(1100 * time.Millisecond) // rate limit
		enrichAlbum(db, album.id, artistName, album.title, mbArtist.ID)
	}
}

func enrichAlbum(db *database.DB, albumID, artistName, albumTitle, mbArtistID string) {
	// Search for the release group
	rg, err := searchReleaseGroup(artistName, albumTitle)
	if err != nil || rg == nil {
		return
	}

	// Update album with MusicBrainz data
	year := parseYear(rg.FirstRelease)
	if year > 0 {
		db.Writer.Exec("UPDATE albums SET musicbrainz_id = ?, year = ? WHERE id = ?",
			rg.ID, year, albumID)
	} else {
		db.Writer.Exec("UPDATE albums SET musicbrainz_id = ? WHERE id = ?",
			rg.ID, albumID)
	}

	// Get the best release for track info
	time.Sleep(1100 * time.Millisecond)
	release, err := getBestRelease(rg.ID)
	if err != nil || release == nil {
		return
	}

	// Match and update tracks
	enrichTracks(db, albumID, release)
}

func enrichTracks(db *database.DB, albumID string, release *MBRelease) {
	// Get existing tracks for this album
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

	// Flatten MusicBrainz tracks
	type mbTrackInfo struct {
		title    string
		duration int
		disc     int
		number   int
	}
	var mbTracks []mbTrackInfo
	for _, media := range release.Media {
		for _, track := range media.Tracks {
			dur := track.Length
			if dur == 0 {
				dur = track.Recording.Length
			}
			mbTracks = append(mbTracks, mbTrackInfo{
				title:    track.Recording.Title,
				duration: dur,
				disc:     media.Position,
				number:   track.Position,
			})
		}
	}

	// Match by position (disc + track number) or by index
	for i, local := range localTracks {
		var matched *mbTrackInfo

		// Try matching by disc + track number
		localDisc := local.disc
		localNum := 0
		if local.num != nil {
			localNum = *local.num
		}
		for _, mb := range mbTracks {
			if mb.disc == localDisc && mb.number == localNum {
				matched = &mb
				break
			}
		}

		// Fall back to index matching
		if matched == nil && i < len(mbTracks) {
			matched = &mbTracks[i]
		}

		if matched != nil {
			db.Writer.Exec(
				"UPDATE tracks SET title = ?, duration_ms = ?, disc_number = ?, track_number = ? WHERE id = ?",
				matched.title, matched.duration, matched.disc, matched.number, local.id)
		}
	}

	// Update album disc_total
	if len(release.Media) > 0 {
		db.Writer.Exec("UPDATE albums SET disc_total = ? WHERE id = ?",
			len(release.Media), albumID)
	}
}

// searchArtist finds an artist on MusicBrainz.
func searchArtist(name string) (*MBArtistFull, error) {
	query := url.QueryEscape(fmt.Sprintf(`artist:"%s"`, strings.ReplaceAll(name, `"`, ``)))
	u := fmt.Sprintf("https://musicbrainz.org/ws/2/artist/?query=%s&limit=1&fmt=json", query)

	data, err := mbRequest(u)
	if err != nil {
		return nil, err
	}

	var result MBArtistSearch
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	if len(result.Artists) == 0 || result.Artists[0].Score < 80 {
		return nil, fmt.Errorf("no confident match for artist %s", name)
	}

	return &result.Artists[0], nil
}

// searchReleaseGroup finds an album on MusicBrainz.
func searchReleaseGroup(artist, album string) (*MBReleaseGroup, error) {
	query := url.QueryEscape(fmt.Sprintf(`releasegroup:"%s" AND artist:"%s"`,
		strings.ReplaceAll(album, `"`, ``),
		strings.ReplaceAll(artist, `"`, ``)))
	u := fmt.Sprintf("https://musicbrainz.org/ws/2/release-group/?query=%s&limit=1&fmt=json", query)

	data, err := mbRequest(u)
	if err != nil {
		return nil, err
	}

	var result MBSearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	if len(result.ReleaseGroups) == 0 || result.ReleaseGroups[0].Score < 75 {
		return nil, fmt.Errorf("no confident match for %s - %s", artist, album)
	}

	return &result.ReleaseGroups[0], nil
}

// getBestRelease gets the best release for a release group (prefer official, CD, with track info).
func getBestRelease(releaseGroupID string) (*MBRelease, error) {
	u := fmt.Sprintf("https://musicbrainz.org/ws/2/release/?release-group=%s&inc=recordings+media&status=official&fmt=json",
		releaseGroupID)

	data, err := mbRequest(u)
	if err != nil {
		return nil, err
	}

	var result MBReleaseLookup
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	if len(result.Releases) == 0 {
		return nil, fmt.Errorf("no releases for group %s", releaseGroupID)
	}

	// Pick the best release: prefer ones with tracks, CD format, US/GB country
	best := &result.Releases[0]
	bestScore := scoreRelease(best)
	for i := 1; i < len(result.Releases); i++ {
		s := scoreRelease(&result.Releases[i])
		if s > bestScore {
			best = &result.Releases[i]
			bestScore = s
		}
	}

	return best, nil
}

func scoreRelease(r *MBRelease) int {
	score := 0
	// Has tracks
	for _, m := range r.Media {
		score += len(m.Tracks) * 2
	}
	// Official
	if r.Status == "Official" {
		score += 10
	}
	// Preferred countries
	switch r.Country {
	case "US":
		score += 5
	case "GB":
		score += 4
	case "XW": // worldwide
		score += 3
	}
	// CD format preferred
	for _, m := range r.Media {
		if strings.Contains(strings.ToLower(m.Format), "cd") {
			score += 3
		}
		if strings.Contains(strings.ToLower(m.Format), "digital") {
			score += 2
		}
	}
	return score
}

func mbRequest(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 503 {
		// Rate limited — wait and retry
		time.Sleep(2 * time.Second)
		return mbRequest(url)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MusicBrainz HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
}

func parseYear(dateStr string) int {
	if len(dateStr) < 4 {
		return 0
	}
	var y int
	fmt.Sscanf(dateStr[:4], "%d", &y)
	return y
}
