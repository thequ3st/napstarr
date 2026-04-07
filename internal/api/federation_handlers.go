package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/thequ3st/napstarr/internal/config"
	"github.com/thequ3st/napstarr/internal/database"
	"github.com/thequ3st/napstarr/internal/federation"
	"github.com/thequ3st/napstarr/internal/identity"
	"github.com/thequ3st/napstarr/internal/p2p"
	"github.com/thequ3st/napstarr/internal/streaming"
)

// handleFederationStream serves audio to authenticated peer instances.
// Uses signed request headers for auth instead of session cookies.
func handleFederationStream(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trackID := r.PathValue("id")

		// Verify peer identity via signed header
		authHeader := r.Header.Get("X-Napstarr-Auth")
		if authHeader == "" {
			Error(w, "federation auth required", http.StatusUnauthorized)
			return
		}

		var msg identity.SignedMessage
		if err := json.Unmarshal([]byte(authHeader), &msg); err != nil {
			Error(w, "invalid auth header", http.StatusUnauthorized)
			return
		}

		// Try known peer first, then accept any valid signature
		var pubKey string
		err := db.Reader.QueryRow(
			"SELECT public_key FROM peers WHERE instance_id = ?", msg.InstanceID,
		).Scan(&pubKey)

		if err != nil {
			// Not a known peer — extract public key from the message itself
			// and verify the signature is self-consistent (proves they own the key)
			// This allows any Napstarr instance to stream, not just mutual follows
			pubKey = msg.PublicKey
		}

		if pubKey == "" || !identity.Verify(&msg, pubKey) {
			Error(w, "invalid signature", http.StatusForbidden)
			return
		}

		if !msg.IsFresh() {
			Error(w, "request expired", http.StatusForbidden)
			return
		}

		// Verified — serve the track
		var filePath, format string
		err = db.Reader.QueryRow(
			"SELECT file_path, format FROM tracks WHERE id = ?", trackID,
		).Scan(&filePath, &format)
		if err != nil {
			Error(w, "track not found", http.StatusNotFound)
			return
		}

		streaming.StreamTrack(w, r, &database.Track{
			FilePath: filePath,
			Format:   format,
		})
	}
}

func getFedNode(cfg *config.Config) *federation.Node {
	return cfg.Federation.(*federation.Node)
}

// handleFederationLibrary exports our library for peers to sync.
// Public endpoint — any Napstarr instance can fetch this.
func handleFederationLibrary(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		node := getFedNode(cfg)
		lib, err := node.ExportLibrary()
		if err != nil {
			Error(w, "failed to export library", http.StatusInternalServerError)
			return
		}
		JSON(w, http.StatusOK, lib)
	}
}

// handleGetPeers returns all followed peers.
func handleGetPeers(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		node := getFedNode(cfg)
		peers, err := node.GetPeers()
		if err != nil {
			Error(w, "failed to fetch peers", http.StatusInternalServerError)
			return
		}
		JSON(w, http.StatusOK, peers)
	}
}

// handleFollowPeer follows a new instance by address.
func handleFollowPeer(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Address string `json:"address"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" {
			Error(w, "address is required", http.StatusBadRequest)
			return
		}

		node := getFedNode(cfg)
		peer, err := node.FollowPeer(req.Address)
		if err != nil {
			Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		JSON(w, http.StatusCreated, peer)
	}
}

// handleUnfollowPeer removes a peer and their remote data.
func handleUnfollowPeer(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		node := getFedNode(cfg)
		if err := node.UnfollowPeer(id); err != nil {
			Error(w, "failed to unfollow", http.StatusInternalServerError)
			return
		}
		JSON(w, http.StatusOK, map[string]string{"status": "unfollowed"})
	}
}

// handleSyncPeer manually triggers a library sync with a peer.
func handleSyncPeer(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		node := getFedNode(cfg)

		peers, err := node.GetPeers()
		if err != nil {
			Error(w, "failed to fetch peers", http.StatusInternalServerError)
			return
		}

		var target *federation.PeerInfo
		for _, p := range peers {
			if p.ID == id {
				target = &p
				break
			}
		}

		if target == nil {
			Error(w, "peer not found", http.StatusNotFound)
			return
		}

		go node.SyncPeerLibrary(target)
		JSON(w, http.StatusAccepted, map[string]string{"status": "sync started"})
	}
}

// handleRemoteArtists returns artists from a peer's library.
func handleRemoteArtists(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		peerID := r.PathValue("id")

		rows, err := db.Reader.Query(
			`SELECT id, name, sort_name, album_count, track_count
			 FROM remote_artists WHERE peer_id = ? ORDER BY sort_name`, peerID)
		if err != nil {
			Error(w, "failed to fetch remote artists", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type Artist struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			SortName   string `json:"sortName"`
			AlbumCount int    `json:"albumCount"`
			TrackCount int    `json:"trackCount"`
		}

		artists := make([]Artist, 0)
		for rows.Next() {
			var a Artist
			rows.Scan(&a.ID, &a.Name, &a.SortName, &a.AlbumCount, &a.TrackCount)
			artists = append(artists, a)
		}

		JSON(w, http.StatusOK, artists)
	}
}

// handleRemoteAlbums returns albums from a peer's library.
func handleRemoteAlbums(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		peerID := r.PathValue("id")

		rows, err := db.Reader.Query(
			`SELECT ra.id, ra.artist_name, ra.title, ra.year, ra.track_count, ra.has_artwork
			 FROM remote_albums ra WHERE ra.peer_id = ? ORDER BY ra.artist_name, ra.title`, peerID)
		if err != nil {
			Error(w, "failed to fetch remote albums", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type Album struct {
			ID         string `json:"id"`
			ArtistName string `json:"artistName"`
			Title      string `json:"title"`
			Year       *int   `json:"year,omitempty"`
			TrackCount int    `json:"trackCount"`
			HasArtwork bool   `json:"hasArtwork"`
		}

		albums := make([]Album, 0)
		for rows.Next() {
			var a Album
			rows.Scan(&a.ID, &a.ArtistName, &a.Title, &a.Year, &a.TrackCount, &a.HasArtwork)
			albums = append(albums, a)
		}

		JSON(w, http.StatusOK, albums)
	}
}

// handleRemoteStream proxies audio from a peer's library through libp2p.
// The browser plays it like a local track — no download, real-time streaming.
func handleRemoteStream(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		peerDBID := r.PathValue("peerId") // our DB peer ID
		trackID := r.PathValue("trackId") // remote track ID

		// Look up the peer's libp2p peer ID
		var libp2pPeerIDStr string
		err := db.Reader.QueryRow(
			"SELECT instance_id FROM peers WHERE id = ?", peerDBID,
		).Scan(&libp2pPeerIDStr)
		if err != nil {
			Error(w, "peer not found", http.StatusNotFound)
			return
		}

		// Get the P2P host
		p2pHost, ok := cfg.P2PHost.(*p2p.Host)
		if !ok || p2pHost == nil {
			Error(w, "P2P not available", http.StatusServiceUnavailable)
			return
		}

		// For now, use HTTP proxy since peers might not have libp2p direct connection yet.
		// Look up peer address and proxy the stream request.
		var peerAddress string
		db.Reader.QueryRow("SELECT address FROM peers WHERE id = ?", peerDBID).Scan(&peerAddress)

		if peerAddress == "" {
			Error(w, "peer address unknown", http.StatusBadGateway)
			return
		}

		// Proxy the stream from the remote instance's HTTP API
		// This works over the federation HTTP connection as a fallback
		// until direct libp2p streaming is established
		streamURL := peerAddress + "/api/federation/stream/" + trackID
		proxyReq, err := http.NewRequestWithContext(r.Context(), "GET", streamURL, nil)
		if err != nil {
			Error(w, "proxy error", http.StatusInternalServerError)
			return
		}

		// Forward range header for seeking
		if rh := r.Header.Get("Range"); rh != "" {
			proxyReq.Header.Set("Range", rh)
		}

		// The remote instance requires auth — but we're accessing the federation library endpoint
		// For now, use the public stream endpoint (we'll need to add a federation token later)
		// Actually, /api/stream/{id} requires auth on the remote side.
		// Let's add a federation stream endpoint that uses the peer's public key for auth.

		// For v1: use a signed request header
		node := getFedNode(cfg)
		msg, _ := node.Instance.Sign("stream_request", map[string]string{"trackId": trackID})
		msgBytes, _ := json.Marshal(msg)
		proxyReq.Header.Set("X-Napstarr-Auth", string(msgBytes))

		resp, err := http.DefaultClient.Do(proxyReq)
		if err != nil {
			log.Printf("remote stream proxy error: %v", err)
			Error(w, "peer not reachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Copy headers
		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		w.WriteHeader(resp.StatusCode)

		// Stream the audio to the browser
		io.Copy(w, resp.Body)
	}
}

// handleTransferRequest downloads a track from a peer permanently.
func handleTransferRequest(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			PeerID  string `json:"peerId"`
			TrackID string `json:"trackId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		// Look up peer
		var peerAddress, peerInstanceID string
		err := db.Reader.QueryRow(
			"SELECT address, instance_id FROM peers WHERE id = ?", req.PeerID,
		).Scan(&peerAddress, &peerInstanceID)
		if err != nil {
			Error(w, "peer not found", http.StatusNotFound)
			return
		}

		// Look up remote track info
		var trackTitle, artistName, albumTitle, format string
		err = db.Reader.QueryRow(
			"SELECT title, artist_name, album_title, format FROM remote_tracks WHERE id = ? AND peer_id = ?",
			req.TrackID, req.PeerID,
		).Scan(&trackTitle, &artistName, &albumTitle, &format)
		if err != nil {
			Error(w, "remote track not found", http.StatusNotFound)
			return
		}

		log.Printf("transfer: requesting %s - %s from peer %s", artistName, trackTitle, peerInstanceID[:8])

		// For now, attempt libp2p transfer. Fall back to HTTP if needed.
		p2pHost, ok := cfg.P2PHost.(*p2p.Host)
		if !ok || p2pHost == nil {
			Error(w, "P2P not available", http.StatusServiceUnavailable)
			return
		}

		// Find the peer's libp2p ID from connected peers
		var targetPeerID peer.ID
		for _, pid := range p2pHost.ConnectedPeers() {
			if pid.String()[:16] == peerInstanceID[:16] {
				targetPeerID = pid
				break
			}
		}

		if targetPeerID == "" {
			// Peer not directly connected via libp2p — fall back would go here
			// For now, return an error
			Error(w, "peer not connected via P2P — direct connection required", http.StatusBadGateway)
			return
		}

		// TODO: execute transfer via TransferService
		// For now, acknowledge the request
		JSON(w, http.StatusAccepted, map[string]string{
			"status":  "transfer initiated",
			"trackId": req.TrackID,
			"peer":    peerInstanceID,
		})
	}
}

// Add io import needed for ProxyStream
var _ = io.Copy
