package streaming

import (
	"net/http"
	"os"
	"strings"

	"github.com/thequ3st/napstarr/internal/database"
)

// StreamTrack serves an audio file with support for HTTP Range requests.
func StreamTrack(w http.ResponseWriter, r *http.Request, track *database.Track) {
	f, err := os.Open(track.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "track file not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to open track", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "failed to stat track", http.StatusInternalServerError)
		return
	}

	contentType := formatToContentType(track.Format)
	w.Header().Set("Content-Type", contentType)

	http.ServeContent(w, r, track.FilePath, info.ModTime(), f)
}

func formatToContentType(format string) string {
	switch strings.ToLower(format) {
	case "flac":
		return "audio/flac"
	case "mp3":
		return "audio/mpeg"
	case "ogg", "opus":
		return "audio/ogg"
	case "m4a":
		return "audio/mp4"
	default:
		return "application/octet-stream"
	}
}
