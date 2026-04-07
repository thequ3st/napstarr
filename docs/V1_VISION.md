# Napstarr v1 Vision

## Core Identity

Napstarr is **Napster reborn for the self-hosted era** — decentralized social music discovery with fast, ISP-invisible P2P transfers. No central server, no domain to seize, no single point of failure.

## Design Principles

1. **Unkillable** — no central infrastructure to shut down
2. **Invisible** — all traffic indistinguishable from normal web/video call usage
3. **Self-sovereign** — your identity is a keypair, not an account on someone's server
4. **Layered anonymity** — works without Tor, better with it

## v1 Scope

### 1. Instance Identity

Each Napstarr instance generates an Ed25519 keypair on first run. Your identity IS your public key. No registration, no central authority.

- Instance ID = base58 of SHA-256(public key) — short, shareable, unique
- All messages signed with private key — impossible to impersonate
- Identity survives server moves, domain changes, IP changes

### 2. Discovery — DHT (No Central Server)

Instances find each other through a Distributed Hash Table — same technology BitTorrent uses to find peers without trackers.

**How it works:**
- On startup, instance announces itself on the DHT under its public key hash
- To find a friend, you look up their instance ID on the DHT → get their current IP/port
- No DNS dependency, no fixed URLs, no domain to seize
- Bootstrap nodes help new instances join (can be any existing Napstarr node)

**Tech:** `github.com/anacrolix/dht` — pure Go, production-grade

**Fallback:** Direct connection by URL still works for simplicity. DHT is the resilient path.

### 3. Federation — Gossip Protocol

Instead of direct connections between every pair of instances, use gossip:

- You tell 3 peers about a library update
- They each tell 3 peers
- Propagates across the network in seconds
- If any node goes down, the network routes around it
- Same principle as Bitcoin transaction propagation

**What gossips:**
- Library indexes (artist/album/track metadata — no file paths, no audio)
- Activity events (now playing, recently added)
- Peer lists (help others discover nodes)

**What doesn't gossip:**
- Private data (listening history stays local unless shared)
- File content (transfers are direct P2P only)
- Credentials (never leave the instance)

### 4. Library Browsing — Content-Addressed

Tracks identified by content hash (SHA-256), not by which instance has them.

- Browse a friend's library → see their artists, albums, tracks
- Each track has a content hash — universal identifier across all instances
- "Who has SHA-256 abc123?" → any node with that track can serve it
- Same album on two instances = same hashes = automatically deduplicated

### 5. P2P Transfers — WebRTC (ISP-Invisible)

When you want a track from someone's library:

**Transfer protocol: WebRTC Data Channels**
- Encrypted end-to-end by default (DTLS-SRTP)
- Looks like a video call to ISPs — same ports, same protocol, same traffic patterns
- NAT traversal via STUN/TURN
- No torrent protocol signatures, no cleartext metadata

**Transfer flow:**
1. Request track by content hash via gossip network
2. Closest peer with that content responds
3. WebRTC signaling exchanged (SDP offer/answer via gossip or direct)
4. Data channel established (direct P2P or TURN relay)
5. File transfers in encrypted chunks
6. Receiver verifies SHA-256 integrity
7. Added to local library, announced to network via gossip

**Why WebRTC:**
- ISP sees: UDP traffic indistinguishable from Google Meet / Discord / FaceTime
- ISP does NOT see: file names, music data, content hashes, anything useful
- Pure Go implementation: `github.com/pion/webrtc`

### 6. Activity Feed

- Real-time "Now Playing" propagated via gossip
- "Recently Added" feed across your followed peers
- Opt-in per instance — can run in stealth mode (no activity broadcast)

### 7. Optional: Tor Hidden Service

For maximum anonymity, an instance can expose itself as a Tor hidden service:
- No IP address visible to anyone, ever
- Federation traffic routed through Tor
- WebRTC transfers can fall back to Tor circuits (slower but invisible)
- ISP sees: Tor traffic. Cannot determine it's Napstarr.

This is optional — everything works without Tor, just with less anonymity.

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                  Napstarr Instance                    │
│                                                      │
│  ┌───────────┐  ┌───────────┐  ┌──────────────────┐ │
│  │ Library   │  │ Player    │  │ Social Feed      │ │
│  │ Scanner   │  │ Streamer  │  │ Activity         │ │
│  └───────────┘  └───────────┘  └──────────────────┘ │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │            Identity Layer                    │    │
│  │  Ed25519 keypair — instance ID = pubkey hash │    │
│  └──────────────────────────────────────────────┘    │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │            Discovery Layer (DHT)             │    │
│  │  Find peers by instance ID, no DNS needed    │    │
│  └──────────────────────────────────────────────┘    │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │            Federation Layer (Gossip)          │    │
│  │  Library indexes, activity, peer discovery   │    │
│  │  Propagates through network, no direct deps  │    │
│  └──────────────────────────────────────────────┘    │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │            Transfer Layer (WebRTC)            │    │
│  │  Content-addressed, encrypted, ISP-invisible │    │
│  │  STUN/TURN for NAT traversal                 │    │
│  └──────────────────────────────────────────────┘    │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │            Anonymity Layer (Tor) [optional]   │    │
│  │  Hidden service, onion-routed federation     │    │
│  └──────────────────────────────────────────────┘    │
│                                                      │
│  ┌──────────┐                                        │
│  │ SQLite   │  Single binary, zero external deps     │
│  └──────────┘                                        │
└──────────────────────────────────────────────────────┘
         │              │              │
    DHT (UDP)     Gossip (TLS)    WebRTC (encrypted UDP)
    find peers    share metadata   transfer files
         │              │              │
         ▼              ▼              ▼
    ┌─────────────────────────────────────┐
    │         The Napstarr Network        │
    │                                     │
    │   No central server. No registry.   │
    │   No domain. No company.            │
    │   Just instances finding each       │
    │   other and sharing music.          │
    │                                     │
    │   To shut it down, shut down        │
    │   every single node. Good luck.     │
    └─────────────────────────────────────┘

ISP sees:
  - DHT: UDP packets (same as any torrent client — millions of users do this)
  - Gossip: TLS connections (same as any HTTPS website)
  - Transfers: WebRTC UDP (same as any video call)
  - Optional: Tor traffic (same as any Tor user)

ISP does NOT see:
  - That it's Napstarr
  - What music you have
  - Who you're sharing with
  - What you're transferring
```

## Tech Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| Identity | `crypto/ed25519` | Stdlib, fast, small keys |
| Discovery | `github.com/anacrolix/dht` | Pure Go, production BitTorrent DHT |
| Gossip | Custom over `github.com/coder/websocket` | Lightweight, TLS-native |
| Transfers | `github.com/pion/webrtc` | Pure Go WebRTC, no CGO |
| Content hashing | `crypto/sha256` | Stdlib, universal |
| Anonymity | `golang.org/x/net/proxy` + Tor SOCKS | Optional, standard integration |
| Everything else | Same as v0.1 | SQLite, tag reader, embedded UI |

## Implementation Phases

### Phase A: Instance Identity
- Generate Ed25519 keypair on first run, persist in data dir
- Instance ID = base58(SHA-256(pubkey))
- API: GET /api/instance — returns pubkey, ID, name, stats
- Sign all outbound messages with private key

### Phase B: Direct Federation (Simple)
- Follow by URL first (before DHT)
- Persistent WebSocket between instances
- Exchange library indexes (signed metadata)
- Store remote library with instance_id in existing schema
- UI: browse remote libraries

### Phase C: P2P Transfer (WebRTC)
- Signaling via federation WebSocket
- WebRTC data channel for file transfer
- Content-hash verification
- "Add to Library" button on remote tracks
- Transfer progress UI

### Phase D: DHT Discovery
- Replace URL-based following with DHT lookup
- Announce instance on DHT at startup
- Find peers by instance ID without knowing their URL
- Bootstrap from hardcoded seed nodes (any Napstarr instance can be a seed)

### Phase E: Gossip Protocol
- Replace direct WebSocket federation with gossip
- Library updates propagate through network
- Activity feed propagates through network
- Peer discovery propagates through network
- Network self-heals when nodes go down

### Phase F: Tor Integration (Optional)
- Run as Tor hidden service
- Route federation through Tor
- Fallback transfers through Tor circuits
- Config flag: --tor / NAPSTARR_TOR=true

## What Makes This Unkillable

| Attack | Defense |
|--------|---------|
| Seize the domain | No domain needed — DHT discovery |
| Shut down the server | Network routes around it — gossip protocol |
| Block the IP | Tor hidden service — no IP exposed |
| Deep packet inspection | WebRTC = video call traffic, TLS = HTTPS, DHT = standard UDP |
| Subpoena the company | No company — open source, decentralized |
| Take down the repo | Already on every instance's disk — binary is self-contained |
| Block the protocol | Indistinguishable from Chrome, Discord, FaceTime |
