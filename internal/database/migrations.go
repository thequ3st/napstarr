package database

import "fmt"

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id            TEXT PRIMARY KEY,
		instance_id   TEXT NOT NULL DEFAULT 'local',
		username      TEXT NOT NULL UNIQUE,
		display_name  TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL,
		is_admin      INTEGER NOT NULL DEFAULT 0,
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`,

	`CREATE TABLE IF NOT EXISTS artists (
		id             TEXT PRIMARY KEY,
		name           TEXT NOT NULL,
		sort_name      TEXT NOT NULL,
		musicbrainz_id TEXT,
		created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`,
	`CREATE INDEX IF NOT EXISTS idx_artists_name ON artists(sort_name)`,
	`CREATE INDEX IF NOT EXISTS idx_artists_name_lower ON artists(LOWER(name))`,

	`CREATE TABLE IF NOT EXISTS albums (
		id             TEXT PRIMARY KEY,
		artist_id      TEXT NOT NULL REFERENCES artists(id),
		title          TEXT NOT NULL,
		year           INTEGER,
		genre          TEXT NOT NULL DEFAULT '',
		disc_total     INTEGER NOT NULL DEFAULT 1,
		musicbrainz_id TEXT,
		has_artwork    INTEGER NOT NULL DEFAULT 0,
		created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`,
	`CREATE INDEX IF NOT EXISTS idx_albums_artist ON albums(artist_id)`,

	`CREATE TABLE IF NOT EXISTS tracks (
		id             TEXT PRIMARY KEY,
		album_id       TEXT NOT NULL REFERENCES albums(id),
		artist_id      TEXT NOT NULL REFERENCES artists(id),
		title          TEXT NOT NULL,
		track_number   INTEGER,
		disc_number    INTEGER NOT NULL DEFAULT 1,
		duration_ms    INTEGER NOT NULL DEFAULT 0,
		file_path      TEXT NOT NULL UNIQUE,
		file_size      INTEGER NOT NULL DEFAULT 0,
		format         TEXT NOT NULL,
		bitrate        INTEGER,
		sample_rate    INTEGER,
		musicbrainz_id TEXT,
		created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album_id)`,
	`CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist_id)`,

	`CREATE TABLE IF NOT EXISTS listening_history (
		id          TEXT PRIMARY KEY,
		user_id     TEXT NOT NULL REFERENCES users(id),
		track_id    TEXT NOT NULL REFERENCES tracks(id),
		listened_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		duration_ms INTEGER NOT NULL DEFAULT 0,
		instance_id TEXT NOT NULL DEFAULT 'local'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_history_user ON listening_history(user_id, listened_at DESC)`,

	`CREATE TABLE IF NOT EXISTS sessions (
		token      TEXT PRIMARY KEY,
		user_id    TEXT NOT NULL REFERENCES users(id),
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		expires_at TEXT NOT NULL
	)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(
		title, artist_name, album_title,
		content='',
		tokenize='unicode61'
	)`,
}

func (db *DB) migrate() error {
	// Create migration tracking table
	if _, err := db.Writer.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	var currentVersion int
	db.Writer.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)

	for i := currentVersion; i < len(migrations); i++ {
		if _, err := db.Writer.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := db.Writer.Exec("INSERT INTO schema_migrations (version) VALUES (?)", i+1); err != nil {
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
	}

	return nil
}
