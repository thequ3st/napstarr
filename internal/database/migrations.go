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

	// Federation: peers we follow
	`CREATE TABLE IF NOT EXISTS peers (
		id            TEXT PRIMARY KEY,
		instance_id   TEXT NOT NULL UNIQUE,
		name          TEXT NOT NULL DEFAULT '',
		public_key    TEXT NOT NULL,
		address       TEXT NOT NULL DEFAULT '',
		last_seen     TEXT,
		last_synced   TEXT,
		status        TEXT NOT NULL DEFAULT 'active',
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`,
	`CREATE INDEX IF NOT EXISTS idx_peers_instance ON peers(instance_id)`,

	// Remote library: artists/albums/tracks from peers
	`CREATE TABLE IF NOT EXISTS remote_artists (
		id            TEXT PRIMARY KEY,
		peer_id       TEXT NOT NULL REFERENCES peers(id),
		name          TEXT NOT NULL,
		sort_name     TEXT NOT NULL,
		album_count   INTEGER NOT NULL DEFAULT 0,
		track_count   INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_remote_artists_peer ON remote_artists(peer_id)`,

	`CREATE TABLE IF NOT EXISTS remote_albums (
		id            TEXT PRIMARY KEY,
		peer_id       TEXT NOT NULL REFERENCES peers(id),
		artist_id     TEXT NOT NULL REFERENCES remote_artists(id),
		artist_name   TEXT NOT NULL DEFAULT '',
		title         TEXT NOT NULL,
		year          INTEGER,
		track_count   INTEGER NOT NULL DEFAULT 0,
		content_hash  TEXT,
		has_artwork   INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_remote_albums_peer ON remote_albums(peer_id)`,

	`CREATE TABLE IF NOT EXISTS remote_tracks (
		id            TEXT PRIMARY KEY,
		peer_id       TEXT NOT NULL REFERENCES peers(id),
		album_id      TEXT NOT NULL REFERENCES remote_albums(id),
		artist_name   TEXT NOT NULL DEFAULT '',
		album_title   TEXT NOT NULL DEFAULT '',
		title         TEXT NOT NULL,
		track_number  INTEGER,
		disc_number   INTEGER NOT NULL DEFAULT 1,
		duration_ms   INTEGER NOT NULL DEFAULT 0,
		format        TEXT NOT NULL DEFAULT '',
		content_hash  TEXT NOT NULL,
		file_size     INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_remote_tracks_peer ON remote_tracks(peer_id)`,
	`CREATE INDEX IF NOT EXISTS idx_remote_tracks_album ON remote_tracks(album_id)`,
	`CREATE INDEX IF NOT EXISTS idx_remote_tracks_hash ON remote_tracks(content_hash)`,

	// Activity feed from peers
	`CREATE TABLE IF NOT EXISTS peer_activity (
		id            TEXT PRIMARY KEY,
		peer_id       TEXT NOT NULL REFERENCES peers(id),
		type          TEXT NOT NULL,
		data          TEXT NOT NULL DEFAULT '{}',
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`,
	`CREATE INDEX IF NOT EXISTS idx_peer_activity_time ON peer_activity(created_at DESC)`,
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
