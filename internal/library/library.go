package library

import (
	"database/sql"
	"fmt"

	"github.com/thequ3st/napstarr/internal/database"
)

// GetArtists returns all artists with album and track counts, sorted by sort_name.
func GetArtists(db *database.DB) ([]database.Artist, error) {
	rows, err := db.Reader.Query(`
		SELECT a.id, a.name, a.sort_name, COALESCE(a.musicbrainz_id, ''),
		       COUNT(DISTINCT al.id), COUNT(DISTINCT t.id)
		FROM artists a
		LEFT JOIN albums al ON al.artist_id = a.id
		LEFT JOIN tracks t ON t.artist_id = a.id
		GROUP BY a.id
		ORDER BY a.sort_name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("query artists: %w", err)
	}
	defer rows.Close()

	var artists []database.Artist
	for rows.Next() {
		var a database.Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.SortName, &a.MusicBrainzID,
			&a.AlbumCount, &a.TrackCount); err != nil {
			return nil, fmt.Errorf("scan artist: %w", err)
		}
		artists = append(artists, a)
	}
	return artists, rows.Err()
}

// GetArtist returns a single artist by ID.
func GetArtist(db *database.DB, id string) (*database.Artist, error) {
	var a database.Artist
	err := db.Reader.QueryRow(`
		SELECT a.id, a.name, a.sort_name, COALESCE(a.musicbrainz_id, ''),
		       COUNT(DISTINCT al.id), COUNT(DISTINCT t.id)
		FROM artists a
		LEFT JOIN albums al ON al.artist_id = a.id
		LEFT JOIN tracks t ON t.artist_id = a.id
		WHERE a.id = ?
		GROUP BY a.id`, id,
	).Scan(&a.ID, &a.Name, &a.SortName, &a.MusicBrainzID, &a.AlbumCount, &a.TrackCount)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("artist not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query artist: %w", err)
	}
	return &a, nil
}

// GetArtistAlbums returns all albums for an artist with track counts.
func GetArtistAlbums(db *database.DB, artistID string) ([]database.Album, error) {
	rows, err := db.Reader.Query(`
		SELECT al.id, al.artist_id, ar.name, al.title, al.year, al.genre,
		       al.disc_total, COALESCE(al.musicbrainz_id, ''), al.has_artwork,
		       COUNT(t.id)
		FROM albums al
		JOIN artists ar ON ar.id = al.artist_id
		LEFT JOIN tracks t ON t.album_id = al.id
		WHERE al.artist_id = ?
		GROUP BY al.id
		ORDER BY al.year, al.title COLLATE NOCASE`, artistID)
	if err != nil {
		return nil, fmt.Errorf("query albums: %w", err)
	}
	defer rows.Close()

	var albums []database.Album
	for rows.Next() {
		var a database.Album
		if err := rows.Scan(&a.ID, &a.ArtistID, &a.ArtistName, &a.Title, &a.Year,
			&a.Genre, &a.DiscTotal, &a.MusicBrainzID, &a.HasArtwork, &a.TrackCount); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

// GetAlbums returns all albums with artist names, sorted by artist sort_name then year.
func GetAlbums(db *database.DB) ([]database.Album, error) {
	rows, err := db.Reader.Query(`
		SELECT al.id, al.artist_id, ar.name, al.title, al.year, al.genre,
		       al.disc_total, COALESCE(al.musicbrainz_id, ''), al.has_artwork,
		       COUNT(t.id)
		FROM albums al
		JOIN artists ar ON ar.id = al.artist_id
		LEFT JOIN tracks t ON t.album_id = al.id
		GROUP BY al.id
		ORDER BY ar.sort_name COLLATE NOCASE, al.year, al.title COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("query albums: %w", err)
	}
	defer rows.Close()

	var albums []database.Album
	for rows.Next() {
		var a database.Album
		if err := rows.Scan(&a.ID, &a.ArtistID, &a.ArtistName, &a.Title, &a.Year,
			&a.Genre, &a.DiscTotal, &a.MusicBrainzID, &a.HasArtwork, &a.TrackCount); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

// GetAlbum returns a single album by ID, including its tracks.
func GetAlbum(db *database.DB, id string) (*database.Album, error) {
	var a database.Album
	err := db.Reader.QueryRow(`
		SELECT al.id, al.artist_id, ar.name, al.title, al.year, al.genre,
		       al.disc_total, COALESCE(al.musicbrainz_id, ''), al.has_artwork
		FROM albums al
		JOIN artists ar ON ar.id = al.artist_id
		WHERE al.id = ?`, id,
	).Scan(&a.ID, &a.ArtistID, &a.ArtistName, &a.Title, &a.Year,
		&a.Genre, &a.DiscTotal, &a.MusicBrainzID, &a.HasArtwork)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("album not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query album: %w", err)
	}

	rows, err := db.Reader.Query(`
		SELECT t.id, t.album_id, t.artist_id, ar.name, al.title,
		       t.title, t.track_number, t.disc_number, t.duration_ms,
		       t.file_size, t.format, t.bitrate, t.sample_rate
		FROM tracks t
		JOIN artists ar ON ar.id = t.artist_id
		JOIN albums al ON al.id = t.album_id
		WHERE t.album_id = ?
		ORDER BY t.disc_number, t.track_number, t.title COLLATE NOCASE`, id)
	if err != nil {
		return nil, fmt.Errorf("query tracks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t database.Track
		if err := rows.Scan(&t.ID, &t.AlbumID, &t.ArtistID, &t.ArtistName,
			&t.AlbumTitle, &t.Title, &t.TrackNumber, &t.DiscNumber, &t.DurationMs,
			&t.FileSize, &t.Format, &t.Bitrate, &t.SampleRate); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		a.Tracks = append(a.Tracks, t)
	}
	a.TrackCount = len(a.Tracks)

	return &a, rows.Err()
}

// GetTrack returns a single track by ID, including its file_path.
func GetTrack(db *database.DB, id string) (*database.Track, error) {
	var t database.Track
	err := db.Reader.QueryRow(`
		SELECT t.id, t.album_id, t.artist_id, ar.name, al.title,
		       t.title, t.track_number, t.disc_number, t.duration_ms,
		       t.file_path, t.file_size, t.format, t.bitrate, t.sample_rate
		FROM tracks t
		JOIN artists ar ON ar.id = t.artist_id
		JOIN albums al ON al.id = t.album_id
		WHERE t.id = ?`, id,
	).Scan(&t.ID, &t.AlbumID, &t.ArtistID, &t.ArtistName, &t.AlbumTitle,
		&t.Title, &t.TrackNumber, &t.DiscNumber, &t.DurationMs,
		&t.FilePath, &t.FileSize, &t.Format, &t.Bitrate, &t.SampleRate)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("track not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query track: %w", err)
	}
	return &t, nil
}

// GetStats returns aggregate library statistics.
func GetStats(db *database.DB) (*database.LibraryStats, error) {
	var s database.LibraryStats
	err := db.Reader.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM artists),
			(SELECT COUNT(*) FROM albums),
			(SELECT COUNT(*) FROM tracks),
			(SELECT COALESCE(SUM(file_size), 0) / (1024 * 1024) FROM tracks)
	`).Scan(&s.ArtistCount, &s.AlbumCount, &s.TrackCount, &s.TotalSizeMB)
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}
	return &s, nil
}

// GetRecentAlbums returns the most recently added albums.
func GetRecentAlbums(db *database.DB, limit int) ([]database.Album, error) {
	rows, err := db.Reader.Query(`
		SELECT al.id, al.artist_id, ar.name, al.title, al.year, al.genre,
		       al.disc_total, COALESCE(al.musicbrainz_id, ''), al.has_artwork,
		       COUNT(t.id)
		FROM albums al
		JOIN artists ar ON ar.id = al.artist_id
		LEFT JOIN tracks t ON t.album_id = al.id
		GROUP BY al.id
		ORDER BY al.created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent albums: %w", err)
	}
	defer rows.Close()

	var albums []database.Album
	for rows.Next() {
		var a database.Album
		if err := rows.Scan(&a.ID, &a.ArtistID, &a.ArtistName, &a.Title, &a.Year,
			&a.Genre, &a.DiscTotal, &a.MusicBrainzID, &a.HasArtwork, &a.TrackCount); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}
