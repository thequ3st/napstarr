package library

import (
	"fmt"
	"strings"

	"github.com/thequ3st/napstarr/internal/database"
)

// Search performs a full-text search across tracks, returning matches with
// artist and album info. The query is sanitized for FTS5 syntax.
func Search(db *database.DB, query string, limit int) ([]database.Track, error) {
	if limit <= 0 {
		limit = 50
	}

	// Sanitize query for FTS5: wrap each term in double quotes to avoid syntax errors.
	terms := strings.Fields(query)
	for i, t := range terms {
		terms[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	ftsQuery := strings.Join(terms, " ")

	rows, err := db.Reader.Query(`
		SELECT t.id, t.album_id, t.artist_id, ar.name, al.title,
		       t.title, t.track_number, t.disc_number, t.duration_ms,
		       t.file_size, t.format, t.bitrate, t.sample_rate
		FROM tracks_fts fts
		JOIN tracks t ON t.rowid = fts.rowid
		JOIN artists ar ON ar.id = t.artist_id
		JOIN albums al ON al.id = t.album_id
		WHERE tracks_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var tracks []database.Track
	for rows.Next() {
		var t database.Track
		if err := rows.Scan(&t.ID, &t.AlbumID, &t.ArtistID, &t.ArtistName,
			&t.AlbumTitle, &t.Title, &t.TrackNumber, &t.DiscNumber, &t.DurationMs,
			&t.FileSize, &t.Format, &t.Bitrate, &t.SampleRate); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

// RebuildSearchIndex clears and repopulates the FTS5 search index from
// the tracks, artists, and albums tables. Call this after a library scan.
func RebuildSearchIndex(db *database.DB) error {
	tx, err := db.Writer.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Clear existing index.
	if _, err := tx.Exec("DELETE FROM tracks_fts"); err != nil {
		return fmt.Errorf("clear fts: %w", err)
	}

	// Rebuild from source tables.
	if _, err := tx.Exec(`
		INSERT INTO tracks_fts (rowid, title, artist_name, album_title)
		SELECT t.rowid, t.title, ar.name, al.title
		FROM tracks t
		JOIN artists ar ON ar.id = t.artist_id
		JOIN albums al ON al.id = t.album_id`); err != nil {
		return fmt.Errorf("rebuild fts: %w", err)
	}

	return tx.Commit()
}
