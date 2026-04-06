# Napstarr

> Federated social music discovery for self-hosted libraries. Napster meets the *arr ecosystem.

## The Problem

Spotify is getting worse — price hikes, disappearing albums, algorithmic slop, podcast spam. Self-hosted music (Lidarr + Plex/Jellyfin) solves the library problem, but it's isolated. You can't see what your friends are listening to, share playlists that actually play, or discover music through people you trust instead of algorithms.

## The Idea

Napstarr is a **social layer on top of self-hosted music libraries**. It's not a file sharing tool — debrid already solved that. It's the thing that made Napster actually great: discovering music through real people.

## Core Features

- **Federated** — each instance runs on someone's server alongside their *arr stack
- **Follow friends** — see their listening activity, shared playlists, what they're adding to their library
- **One-click grabs** — when a friend shares a track you don't have, one click adds it to your Lidarr wanted list
- **Listening rooms** — synchronized playback across instances
- **No central server** — your data stays on your hardware

## Architecture: ISP-Safe by Design

The critical design principle: **never transfer actual content between peers.**

### What flows between Napstarr instances (metadata only):
- "User X has Album Y" — just a database entry
- Playlist data — track IDs, order, metadata
- Listening activity — what's playing, timestamps
- Social — follows, likes, comments

### What DOESN'T flow between instances:
- Audio files
- Torrent traffic
- Any copyrighted content

### How content acquisition works:
1. You see a friend is listening to an album you don't have
2. You click "Add to Library"
3. Napstarr tells your Lidarr instance to search for it
4. Lidarr grabs it through your existing debrid pipeline (altmount/nzbdav/decypharr)
5. Debrid CDN delivers it over HTTPS — indistinguishable from any streaming traffic

**ISP sees:** encrypted HTTPS to debrid CDN + encrypted WebSocket between Napstarr instances (just metadata). No P2P, no torrent protocol, no file transfers between residential IPs.

## Tech Stack (Proposed)

| Layer | Tech | Purpose |
|-------|------|---------|
| **Backend** | Go or Python | Federation protocol, API, WebSocket server |
| **Frontend** | React or Svelte | Web UI, music player, social feed |
| **Federation** | ActivityPub or custom | Instance-to-instance communication |
| **Database** | SQLite or Postgres | Library metadata, social graph |
| **Integration** | Lidarr API, Subsonic API | Library management, audio streaming |
| **Audio** | Subsonic/Navidrome API | Actual playback from user's own server |
| **Transport** | WebSocket over TLS | Real-time sync, listening rooms |

## Integration Points

- **Lidarr** — search & grab missing music on demand
- **Plex/Jellyfin/Navidrome** — audio playback (Subsonic API is the common protocol)
- **MusicBrainz** — canonical track/album/artist identification
- **Last.fm/ListenBrainz** — optional scrobbling, existing social music data

## Inspiration

- **Napster** — social music discovery (the soul, not the piracy)
- **Mastodon/ActivityPub** — federated social networking
- **Lidarr/*arr** — self-hosted media management
- **Spotify** — what it used to be before it got bad

## Status

Idea phase. This README is the spec.

---

*A [Qu3st](https://github.com/thequ3st) project.*
