# Napstarr v1 Vision

## Core Identity

Napstarr is **Napster reborn for the self-hosted era** — federated social music discovery with fast, ISP-invisible P2P transfers between instances.

## v1 Scope

### 1. Federation — Browse Your Friends' Libraries

Each Napstarr instance is a node. You follow other instances. When you follow someone, you can:
- Browse their entire music library (artists, albums, tracks)
- See what they're listening to in real time
- See when they add new music
- Search across all instances you follow

**Protocol:** Instance-to-instance communication over WebSocket/TLS. Looks like any web app traffic to an ISP. Metadata only — library indexes, listening activity, search queries. No audio in the federation layer.

**Identity:** Each instance has a keypair generated on first run. Instance identity = public key hash. Users are `username@instance-address`. Discovery is manual for v1 (paste an instance URL to follow).

### 2. P2P Transfers — Fast, Obfuscated

When you find a track/album in a friend's library and want it:
- Click "Add to Library"
- Napstarr initiates a direct transfer from their instance to yours
- The file lands in your local library, fully yours

**Transfer protocol: WebRTC Data Channels**
- Encrypted end-to-end by default (DTLS-SRTP)
- Looks like a video call to ISPs — same ports, same protocol, same traffic patterns
- NAT traversal via STUN/TURN (use public STUN servers, self-hosted TURN as fallback)
- No torrent protocol signatures, no cleartext metadata, no identifiable patterns
- Falls back to relayed transfer through a TURN server if direct connection fails

**Why WebRTC:**
- ISPs see UDP traffic to/from STUN servers — indistinguishable from Google Meet, Discord, FaceTime
- The actual data channel is encrypted, ISP cannot inspect content
- Browser-native technology, battle-tested, NAT-friendly
- Supports large file transfers via chunking
- Go has mature WebRTC libraries (pion/webrtc)

**Transfer flow:**
1. Requesting instance sends transfer request via federation WebSocket
2. Both instances exchange WebRTC signaling (SDP offer/answer) over the federation channel
3. WebRTC data channel established (direct P2P or relayed via TURN)
4. File transfers in encrypted chunks
5. Receiving instance verifies integrity (SHA-256), adds to local library
6. Federation layer updates: "user X now has Album Y"

### 3. Activity Feed — The Social Layer

- Real-time "Now Playing" from followed instances
- "Recently Added" feed showing new music across your network
- Library stats visible on your profile (X artists, Y albums, Z tracks)
- Activity is opt-in per instance (can disable broadcasting)

## What v1 Does NOT Include

- Debrid/usenet acquisition (Lidarr handles this independently for now)
- Blockchain/reputation (solve trust when there's a network to trust)
- Listening rooms / synchronized playback (v2)
- Mobile app (Subsonic API compatibility is a v2 goal)
- Public discovery / instance directory (v2)

## Architecture

```
┌─────────────────────────────────────────────┐
│              Napstarr Instance               │
│                                              │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
│  │ Library   │  │ Player   │  │ Social    │  │
│  │ Scanner   │  │ Streamer │  │ Feed      │  │
│  └──────────┘  └──────────┘  └───────────┘  │
│                                              │
│  ┌──────────────────────────────────────┐    │
│  │        Federation Layer              │    │
│  │  - Instance identity (keypair)       │    │
│  │  - Library index sharing             │    │
│  │  - Activity broadcasting             │    │
│  │  - Transfer signaling                │    │
│  └──────────────────────────────────────┘    │
│                                              │
│  ┌──────────────────────────────────────┐    │
│  │        P2P Transfer Layer            │    │
│  │  - WebRTC data channels              │    │
│  │  - STUN/TURN NAT traversal           │    │
│  │  - Chunked encrypted file transfer   │    │
│  │  - SHA-256 integrity verification    │    │
│  └──────────────────────────────────────┘    │
│                                              │
│  ┌──────────┐                                │
│  │ SQLite   │  Single binary, zero deps      │
│  └──────────┘                                │
└─────────────────────────────────────────────┘
         │                           │
    TLS WebSocket              WebRTC (encrypted)
    (metadata only)            (file transfers)
         │                           │
         ▼                           ▼
┌─────────────────┐         ┌─────────────────┐
│ Friend's        │         │ Friend's        │
│ Instance        │◄───────►│ Instance        │
│ (federation)    │         │ (P2P transfer)  │
└─────────────────┘         └─────────────────┘

ISP sees: TLS WebSocket (like any web app) + UDP/WebRTC (like a video call)
ISP does NOT see: file names, music data, library contents, user activity
```

## Tech Stack Additions for v1

| Component | Technology |
|-----------|-----------|
| WebRTC | `github.com/pion/webrtc` (pure Go) |
| Instance identity | Ed25519 keypairs via `crypto/ed25519` |
| Signaling | Existing WebSocket infrastructure |
| NAT traversal | Public STUN servers + optional self-hosted TURN |
| File integrity | SHA-256 checksums |

## Implementation Phases

### Phase A: Instance Identity + Keypair
- Generate Ed25519 keypair on first run, store in data dir
- Instance ID = base58 of public key hash
- API endpoint: GET /api/instance/info (returns public key, name, stats)

### Phase B: Federation Protocol
- Follow an instance by URL
- Exchange library indexes (artist/album/track metadata, no file paths)
- Real-time activity feed over persistent WebSocket
- Store remote library data with instance_id in existing schema

### Phase C: Library Browsing
- UI: browse followed instances' libraries
- Search across local + remote libraries
- "Add to Library" button on remote tracks/albums

### Phase D: P2P Transfer
- WebRTC signaling via federation WebSocket
- Data channel file transfer with chunking
- Receive file → add to local library → update index
- Progress UI during transfers

### Phase E: Activity Feed
- Now Playing broadcasts
- Recently Added broadcasts
- Feed view in UI aggregating all followed instances
