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
| **Networking** | `github.com/libp2p/go-libp2p` | **The stack.** DHT, NAT traversal, encryption, peer discovery, file transfer — all in one. Used by IPFS, Ethereum, Filecoin. Millions of nodes. Blocking libp2p means blocking half of Web3. |
| Identity | libp2p peer IDs (Ed25519) | Native to libp2p, same crypto we planned |
| Discovery | libp2p Kademlia DHT | Production-grade, replaces custom DHT |
| NAT traversal | libp2p AutoRelay + hole-punching | Handles STUN/TURN/relay automatically |
| Transport | QUIC + WebRTC + WebTransport | Multiple transports, ISP sees standard traffic |
| Encryption | Noise protocol (via libp2p) | Same as Signal/WireGuard, no TLS fingerprint for DPI |
| File transfer | libp2p streams | Multiplexed, encrypted, over any transport |
| Gossip | libp2p GossipSub | Built-in pub/sub for activity feeds and library updates |
| Content hashing | `crypto/sha256` | Stdlib, universal |
| Anonymity | Tor SOCKS proxy (optional) | Route libp2p through Tor |
| Everything else | Same as v0.1 | SQLite, tag reader, embedded UI |

### Why libp2p over custom WebRTC + DHT + gossip

We were going to build three separate systems (Phases C, D, E). libp2p IS all three in one battle-tested package:
- **Phase C (P2P transfer)** → libp2p streams over QUIC/WebRTC
- **Phase D (DHT discovery)** → libp2p Kademlia DHT
- **Phase E (gossip)** → libp2p GossipSub pub/sub
- **NAT traversal** → libp2p AutoRelay + hole-punching (free)
- **Encryption** → Noise protocol (harder to fingerprint than TLS)
- **Multi-transport** → QUIC, WebRTC, WebTransport, TCP — automatically picks the best one

The protocol is already running on millions of nodes. Trying to block it means blocking IPFS, Ethereum, Filecoin. That's our camouflage.

## Implementation Phases

### Phase A: Instance Identity ✅ DONE
- Ed25519 keypair generated on first run, persisted to identity.json
- Instance ID = base58(SHA-256(pubkey))
- API: GET /api/instance — returns pubkey, ID, name, stats
- Signed messages with replay protection

### Phase B: Direct Federation ✅ DONE
- Follow by URL (POST /api/peers {address})
- Library export/import between instances
- Remote library stored in remote_artists/albums/tracks tables
- Browse peer libraries via API

### Phase C: libp2p Integration ← NEXT
- Replace URL-based federation with libp2p host
- Generate libp2p peer ID from existing Ed25519 key
- Join the libp2p Kademlia DHT on startup
- Announce instance on DHT — discoverable without fixed URLs
- Connect to peers via libp2p (auto NAT traversal, hole-punching)
- Multiple transports: QUIC (default), WebRTC, WebTransport

### Phase D: P2P File Transfer
- libp2p stream protocol for file transfer (/napstarr/transfer/1.0.0)
- Content-hash based requests ("send me SHA-256 abc123")
- Chunked transfer with progress reporting
- Verify integrity on receive, add to local library
- "Add to Library" button on remote tracks in UI
- Transfer progress UI

### Phase E: GossipSub
- libp2p GossipSub for decentralized pub/sub
- Topics: library-updates, now-playing, peer-discovery
- Library changes propagate across network automatically
- Activity feed aggregated from subscribed topics
- Network self-heals — no direct dependency on any single peer

### Phase F: Tor Integration (Optional)
- Route libp2p through Tor SOCKS proxy
- Run as Tor hidden service
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
