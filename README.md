# Napstarr

> Self-hosted music platform. One binary. No dependencies. Your library, your rules.

Napstarr scans your music directory, builds a library, and serves a sleek web UI with full audio playback. Everything is embedded in a single Go binary — no external database, no Node.js, no build toolchain.

## Features

- **Single binary** — everything embedded: web UI, database engine, audio streamer
- **Music scanner** — reads FLAC, MP3, M4A, OGG/Opus tags automatically
- **Web player** — album art grid, queue management, keyboard shortcuts
- **Audio streaming** — range request support for instant seeking
- **Full-text search** — FTS5-powered search across artists, albums, and tracks
- **Dark theme** — clean, modern UI that looks good at 2am
- **Federation-ready** — UUIDv7 keys, instance IDs, MusicBrainz IDs in the schema
- **Docker-ready** — ~30MB alpine image

## Quick Start

### Binary

```bash
go build -o napstarr .
./napstarr \
  --music-dir /path/to/music \
  --data-dir ./data \
  --admin-pass yourpassword
```

Open `http://localhost:8484`, log in, and hit **Scan Library**.

### Docker

```bash
docker build -t napstarr .
docker run -d \
  -p 8484:8484 \
  -v /path/to/music:/music:ro \
  -v napstarr-data:/data \
  -e NAPSTARR_ADMIN_PASS=yourpassword \
  napstarr
```

### Docker Compose

```yaml
services:
  napstarr:
    build: .
    container_name: napstarr
    ports:
      - "8484:8484"
    volumes:
      - ./data:/data
      - /path/to/music:/music:ro
    environment:
      - NAPSTARR_MUSIC_DIR=/music
      - NAPSTARR_DATA_DIR=/data
      - NAPSTARR_ADMIN_PASS=changeme
    restart: unless-stopped
```

## Configuration

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--music-dir` | `NAPSTARR_MUSIC_DIR` | `/music` | Path to music library |
| `--data-dir` | `NAPSTARR_DATA_DIR` | `/data` | Path for database + artwork cache |
| `--listen` | `NAPSTARR_LISTEN` | `:8484` | Listen address |
| `--admin-user` | `NAPSTARR_ADMIN_USER` | `admin` | Admin username |
| `--admin-pass` | `NAPSTARR_ADMIN_PASS` | *(generated)* | Admin password |

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.23 (stdlib `net/http`) |
| Database | SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGO) |
| Audio tags | [dhowden/tag](https://github.com/dhowden/tag) |
| Frontend | Vanilla JS + CSS (no framework, no build step) |
| Auth | Session-based with bcrypt |

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Space` | Play / Pause |
| `→` | Seek forward 10s |
| `←` | Seek back 10s |
| `↑` | Volume up |
| `↓` | Volume down |

## Roadmap

- [ ] **v0.2** — Federation: connect instances, browse friends' libraries
- [ ] **v0.3** — Social feed: see what friends are listening to
- [ ] **v0.4** — One-click grabs: add music from friends' libraries to your own
- [ ] **v0.5** — Listening rooms: synchronized playback across instances

## Philosophy

Napstarr exists because music is social but self-hosting is isolated. Spotify is getting worse. The *arr ecosystem solved media acquisition but not music discovery. Napster's magic wasn't piracy — it was browsing a stranger's collection and finding something you'd never heard of.

This is the foundation.

---

*A [Qu3st](https://github.com/thequ3st) project.*
