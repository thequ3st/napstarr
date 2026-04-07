package api

import (
	"encoding/json"
	"net/http"

	"github.com/thequ3st/napstarr/internal/config"
	"github.com/thequ3st/napstarr/internal/database"
	"github.com/thequ3st/napstarr/internal/federation"
)

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
