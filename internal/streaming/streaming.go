package streaming

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/thequ3st/napstarr/internal/database"
)

// StreamTrack serves an audio file with support for HTTP Range requests.
// If the file is on a FUSE mount and not cached, it attempts to warm the cache.
func StreamTrack(w http.ResponseWriter, r *http.Request, track *database.Track) {
	f, err := openWithWarmup(track.FilePath)
	if err != nil {
		http.Error(w, "track not available", http.StatusNotFound)
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

// openWithWarmup tries to open a file, and if it fails, retries a few times
// with short delays to allow FUSE/rclone to cache the file from debrid.
func openWithWarmup(path string) (*os.File, error) {
	// First try — instant
	f, err := os.Open(path)
	if err == nil {
		return f, nil
	}

	if !os.IsNotExist(err) {
		return nil, err
	}

	// File doesn't exist — might be a cold FUSE symlink.
	// Try to stat the parent directory to trigger rclone directory cache,
	// then retry opening the file.
	log.Printf("stream: warming FUSE cache for %s", path)

	// Touch the parent directory to trigger rclone readdir
	parentDir := path[:strings.LastIndex(path, "/")]
	entries, _ := os.ReadDir(parentDir)
	_ = entries // just trigger the readdir

	// Retry with backoff: 500ms, 1s, 2s, 4s
	for _, delay := range []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
	} {
		time.Sleep(delay)
		f, err = os.Open(path)
		if err == nil {
			log.Printf("stream: FUSE cache warmed for %s (after %v)", path, delay)
			return f, nil
		}
	}

	return nil, fmt.Errorf("file not accessible after cache warmup: %s", path)
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
