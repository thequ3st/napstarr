package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/thequ3st/napstarr/internal/config"
	"github.com/thequ3st/napstarr/internal/database"
	"github.com/thequ3st/napstarr/internal/identity"
	"github.com/thequ3st/napstarr/internal/ws"
	"github.com/thequ3st/napstarr/web"
)

// NewRouter creates the HTTP handler with all routes wired up.
func NewRouter(db *database.DB, cfg *config.Config, hub *ws.Hub, inst *identity.Instance) http.Handler {
	mux := http.NewServeMux()

	// Auth routes
	mux.HandleFunc("POST /api/auth/login", handleLogin(db))
	mux.HandleFunc("POST /api/auth/logout", AuthRequired(handleLogout(db), db))

	// Artist routes
	mux.HandleFunc("GET /api/artists", handleListArtists(db))
	mux.HandleFunc("GET /api/artists/{id}", handleGetArtist(db))
	mux.HandleFunc("GET /api/artists/{id}/albums", handleGetArtistAlbums(db))

	// Album routes
	mux.HandleFunc("GET /api/albums", handleListAlbums(db))
	mux.HandleFunc("GET /api/albums/{id}", handleGetAlbum(db))

	// Track routes
	mux.HandleFunc("GET /api/tracks/{id}", handleGetTrack(db))

	// Streaming (auth required)
	mux.HandleFunc("GET /api/stream/{id}", handleStream(db))

	// Artwork (public for img tags)
	mux.HandleFunc("GET /api/artwork/album/{id}", handleArtwork(cfg))

	// Search (auth required)
	mux.HandleFunc("GET /api/search", AuthRequired(handleSearch(db), db))

	// Library management
	mux.HandleFunc("POST /api/library/scan", AuthRequired(AdminRequired(handleScan(db, cfg, hub)), db))
	mux.HandleFunc("GET /api/library/stats", AuthRequired(handleStats(db), db))

	// Listen history
	mux.HandleFunc("POST /api/history", AuthRequired(handleRecordListen(db), db))
	mux.HandleFunc("GET /api/history", AuthRequired(handleGetHistory(db), db))

	// Instance identity (public — other instances need this to connect)
	mux.HandleFunc("GET /api/instance", handleInstance(inst, db))

	// Federation: public endpoints for peers
	mux.HandleFunc("GET /api/federation/library", handleFederationLibrary(cfg))
	mux.HandleFunc("GET /api/federation/stream/{id}", handleFederationStream(db, cfg))

	// Federation: manage peers (auth required)
	mux.HandleFunc("GET /api/peers", AuthRequired(handleGetPeers(cfg), db))
	mux.HandleFunc("POST /api/peers", AuthRequired(AdminRequired(handleFollowPeer(cfg)), db))
	mux.HandleFunc("DELETE /api/peers/{id}", AuthRequired(AdminRequired(handleUnfollowPeer(cfg)), db))
	mux.HandleFunc("POST /api/peers/{id}/sync", AuthRequired(AdminRequired(handleSyncPeer(cfg)), db))

	// Remote library browsing (auth required)
	mux.HandleFunc("GET /api/peers/{id}/artists", AuthRequired(handleRemoteArtists(db), db))
	mux.HandleFunc("GET /api/peers/{id}/albums", AuthRequired(handleRemoteAlbums(db), db))

	// Remote streaming — play a track from a peer without downloading
	mux.HandleFunc("GET /api/stream/remote/{peerId}/{trackId}", AuthRequired(handleRemoteStream(db, cfg), db))

	// Transfer — download a track from a peer permanently
	mux.HandleFunc("POST /api/transfer", AuthRequired(AdminRequired(handleTransferRequest(db, cfg)), db))

	// WebSocket
	mux.HandleFunc("GET /api/ws", AuthRequired(handleWebSocket(hub), db))

	// SPA handler — serve embedded web/dist files, fallback to index.html
	distFS, _ := fs.Sub(web.DistFS, "dist")
	fileServer := http.FileServer(http.FS(distFS))

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the exact file first
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if the file exists in the embedded FS
		f, err := distFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback to index.html for SPA routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	return RequestLogger(mux)
}
