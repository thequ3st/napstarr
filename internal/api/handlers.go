package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/thequ3st/napstarr/internal/auth"
	"github.com/thequ3st/napstarr/internal/config"
	"github.com/thequ3st/napstarr/internal/database"
	"github.com/thequ3st/napstarr/internal/identity"
	"github.com/thequ3st/napstarr/internal/library"
	"github.com/thequ3st/napstarr/internal/scanner"
	"github.com/thequ3st/napstarr/internal/streaming"
	"github.com/thequ3st/napstarr/internal/ws"
)

func handleLogin(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		token, err := auth.Login(db, req.Username, req.Password)
		if err != nil {
			Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 30,
		})

		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleLogout(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err == nil {
			auth.Logout(db, cookie.Value)
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})

		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleListArtists(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artists, err := library.GetArtists(db)
		if err != nil {
			Error(w, "failed to fetch artists", http.StatusInternalServerError)
			return
		}
		JSON(w, http.StatusOK, artists)
	}
}

func handleGetArtist(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		artist, err := library.GetArtist(db, id)
		if err != nil {
			Error(w, "artist not found", http.StatusNotFound)
			return
		}
		JSON(w, http.StatusOK, artist)
	}
}

func handleGetArtistAlbums(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		albums, err := library.GetArtistAlbums(db, id)
		if err != nil {
			Error(w, "failed to fetch albums", http.StatusInternalServerError)
			return
		}
		JSON(w, http.StatusOK, albums)
	}
}

func handleListAlbums(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("recent") == "true" {
			albums, err := library.GetRecentAlbums(db, 20)
			if err != nil {
				Error(w, "failed to fetch recent albums", http.StatusInternalServerError)
				return
			}
			JSON(w, http.StatusOK, albums)
			return
		}

		albums, err := library.GetAlbums(db)
		if err != nil {
			Error(w, "failed to fetch albums", http.StatusInternalServerError)
			return
		}
		JSON(w, http.StatusOK, albums)
	}
}

func handleGetAlbum(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		album, err := library.GetAlbum(db, id)
		if err != nil {
			Error(w, "album not found", http.StatusNotFound)
			return
		}
		JSON(w, http.StatusOK, album)
	}
}

func handleGetTrack(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		track, err := library.GetTrack(db, id)
		if err != nil {
			Error(w, "track not found", http.StatusNotFound)
			return
		}
		JSON(w, http.StatusOK, track)
	}
}

func handleStream(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		track, err := library.GetTrack(db, id)
		if err != nil {
			Error(w, "track not found", http.StatusNotFound)
			return
		}
		streaming.StreamTrack(w, r, track)
	}
}

func handleArtwork(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		artPath := filepath.Join(cfg.DataDir, "artwork", id+".jpg")

		if _, err := os.Stat(artPath); os.IsNotExist(err) {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Header().Set("Cache-Control", "public, max-age=3600")
			w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="300" height="300"><rect width="300" height="300" fill="#1e1e1e"/><text x="150" y="160" text-anchor="middle" fill="#555" font-size="48" font-family="sans-serif">&#9835;</text></svg>`))
			return
		}

		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, artPath)
	}
}

func handleSearch(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			JSON(w, http.StatusOK, []database.Track{})
			return
		}

		results, err := library.Search(db, q, 50)
		if err != nil {
			Error(w, "search failed", http.StatusInternalServerError)
			return
		}
		JSON(w, http.StatusOK, results)
	}
}

func handleScan(db *database.DB, cfg *config.Config, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("scan panic: %v", r)
					hub.Broadcast("scan_error", map[string]string{"error": fmt.Sprintf("panic: %v", r)})
				}
			}()

			log.Printf("scan: starting scan of %s", cfg.MusicDir)
			hub.Broadcast("scan_started", map[string]string{"status": "started"})

			artworkDir := filepath.Join(cfg.DataDir, "artwork")
			err := scanner.Scan(db, cfg.MusicDir, artworkDir, func(scanned, total int) {
				log.Printf("scan: %d/%d", scanned, total)
				hub.Broadcast("scan_progress", map[string]int{
					"scanned": scanned,
					"total":   total,
				})
			})

			if err != nil {
				log.Printf("scan error: %v", err)
				hub.Broadcast("scan_error", map[string]string{"error": err.Error()})
				return
			}

			// Rebuild search index after scan
			if err := library.RebuildSearchIndex(db); err != nil {
				log.Printf("search index rebuild error: %v", err)
			}

			hub.Broadcast("scan_complete", map[string]string{"status": "complete"})
		}()

		JSON(w, http.StatusAccepted, map[string]string{"status": "scan started"})
	}
}

func handleStats(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := library.GetStats(db)
		if err != nil {
			Error(w, "failed to fetch stats", http.StatusInternalServerError)
			return
		}
		JSON(w, http.StatusOK, stats)
	}
}

func handleRecordListen(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r)
		if user == nil {
			Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req struct {
			TrackID    string `json:"trackId"`
			DurationMs int    `json:"durationMs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		_, err := db.Writer.Exec(
			`INSERT INTO listening_history (id, user_id, track_id, duration_ms) VALUES (?, ?, ?, ?)`,
			database.NewID(), user.ID, req.TrackID, req.DurationMs,
		)
		if err != nil {
			Error(w, "failed to record listen", http.StatusInternalServerError)
			return
		}

		JSON(w, http.StatusCreated, map[string]string{"status": "ok"})
	}
}

func handleGetHistory(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r)
		if user == nil {
			Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		rows, err := db.Reader.Query(`
			SELECT h.id, h.track_id, h.listened_at, h.duration_ms,
			       t.title, t.duration_ms, t.format,
			       ar.name, al.title
			FROM listening_history h
			JOIN tracks t ON h.track_id = t.id
			JOIN artists ar ON t.artist_id = ar.id
			JOIN albums al ON t.album_id = al.id
			WHERE h.user_id = ?
			ORDER BY h.listened_at DESC
			LIMIT 50
		`, user.ID)
		if err != nil {
			Error(w, "failed to fetch history", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type HistoryEntry struct {
			ID         string `json:"id"`
			TrackID    string `json:"trackId"`
			ListenedAt string `json:"listenedAt"`
			DurationMs int    `json:"durationMs"`
			TrackTitle string `json:"trackTitle"`
			TrackDurMs int    `json:"trackDurationMs"`
			Format     string `json:"format"`
			ArtistName string `json:"artistName"`
			AlbumTitle string `json:"albumTitle"`
		}

		var history []HistoryEntry
		for rows.Next() {
			var h HistoryEntry
			rows.Scan(&h.ID, &h.TrackID, &h.ListenedAt, &h.DurationMs,
				&h.TrackTitle, &h.TrackDurMs, &h.Format,
				&h.ArtistName, &h.AlbumTitle)
			history = append(history, h)
		}
		if history == nil {
			history = []HistoryEntry{}
		}

		JSON(w, http.StatusOK, history)
	}
}

func handleWebSocket(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r)
	}
}

func handleInstance(inst *identity.Instance, db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := inst.Info()

		// Add library stats
		stats, _ := library.GetStats(db)

		JSON(w, http.StatusOK, map[string]any{
			"id":        info.ID,
			"name":      info.Name,
			"publicKey": info.PublicKey,
			"createdAt": info.CreatedAt,
			"stats":     stats,
			"version":   "0.1.0",
			"protocol":  "napstarr/1",
		})
	}
}

// suppress unused import warnings
var _ = time.Now
