package p2p

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/thequ3st/napstarr/internal/database"
)

const (
	ProtocolStream = protocol.ID("/napstarr/stream/1.0.0")
)

// StreamRequest is sent by the requesting peer to stream audio.
type StreamRequest struct {
	TrackID    string `json:"trackId"`
	RangeStart int64  `json:"rangeStart,omitempty"` // byte offset for seeking
	RangeEnd   int64  `json:"rangeEnd,omitempty"`
}

// StreamResponse header sent before audio data.
type StreamResponse struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	ContentType string `json:"contentType"`
	FileSize    int64  `json:"fileSize"`
	RangeStart  int64  `json:"rangeStart"`
	Format      string `json:"format"`
}

// StreamService handles audio streaming between peers.
type StreamService struct {
	host *Host
	db   *database.DB
}

// NewStreamService creates the stream service and registers the handler.
func NewStreamService(h *Host, db *database.DB) *StreamService {
	ss := &StreamService{host: h, db: db}
	h.host.SetStreamHandler(ProtocolStream, ss.handleIncoming)
	return ss
}

// handleIncoming serves audio to a requesting peer — streams from local disk.
func (ss *StreamService) handleIncoming(s network.Stream) {
	defer s.Close()

	var req StreamRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		writeStreamError(s, "invalid request")
		return
	}

	log.Printf("stream: peer %s wants to stream track %s", s.Conn().RemotePeer().String()[:16], req.TrackID)

	var filePath, format string
	var fileSize int64
	err := ss.db.Reader.QueryRow(
		"SELECT file_path, format, file_size FROM tracks WHERE id = ?", req.TrackID,
	).Scan(&filePath, &format, &fileSize)
	if err != nil {
		writeStreamError(s, "track not found")
		return
	}

	f, err := openFileForStream(filePath)
	if err != nil {
		writeStreamError(s, "file not accessible")
		return
	}
	defer f.Close()

	stat, _ := f.Stat()
	fileSize = stat.Size()

	// Handle range request (seeking)
	if req.RangeStart > 0 {
		f.Seek(req.RangeStart, io.SeekStart)
	}

	contentType := formatToContentType(format)

	// Send response header as a single JSON line
	resp := StreamResponse{
		OK:          true,
		ContentType: contentType,
		FileSize:    fileSize,
		RangeStart:  req.RangeStart,
		Format:      format,
	}
	respBytes, _ := json.Marshal(resp)
	s.Write(respBytes)
	s.Write([]byte("\n"))

	// Stream audio data
	var written int64
	if req.RangeEnd > 0 && req.RangeEnd > req.RangeStart {
		written, _ = io.CopyN(s, f, req.RangeEnd-req.RangeStart)
	} else {
		written, _ = io.Copy(s, f)
	}

	log.Printf("stream: streamed %d bytes of %s to %s", written, req.TrackID, s.Conn().RemotePeer().String()[:16])
}

// ProxyStream handles an HTTP request by streaming audio from a remote peer.
// This is called when the local web UI wants to play a remote track.
func (ss *StreamService) ProxyStream(w http.ResponseWriter, r *http.Request, peerID peer.ID, trackID string) {
	ctx := r.Context()
	s, err := ss.host.host.NewStream(ctx, peerID, ProtocolStream)
	if err != nil {
		http.Error(w, "peer not reachable", http.StatusBadGateway)
		return
	}
	defer s.Close()

	// Parse range header from browser
	var rangeStart, rangeEnd int64
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		// Parse "bytes=START-END"
		fmt.Sscanf(rangeHeader, "bytes=%d-%d", &rangeStart, &rangeEnd)
	}

	// Send stream request
	req := StreamRequest{
		TrackID:    trackID,
		RangeStart: rangeStart,
		RangeEnd:   rangeEnd,
	}
	reqBytes, _ := json.Marshal(req)
	s.Write(reqBytes)
	s.Write([]byte("\n"))

	// Read response header (single JSON line)
	buf := make([]byte, 0, 4096)
	oneByte := make([]byte, 1)
	for {
		_, err := s.Read(oneByte)
		if err != nil {
			http.Error(w, "stream error", http.StatusBadGateway)
			return
		}
		if oneByte[0] == '\n' {
			break
		}
		buf = append(buf, oneByte[0])
	}

	var resp StreamResponse
	if err := json.Unmarshal(buf, &resp); err != nil {
		http.Error(w, "invalid stream response", http.StatusBadGateway)
		return
	}

	if !resp.OK {
		http.Error(w, resp.Error, http.StatusNotFound)
		return
	}

	// Set HTTP headers for the browser's audio player
	w.Header().Set("Content-Type", resp.ContentType)
	w.Header().Set("Accept-Ranges", "bytes")

	if rangeStart > 0 {
		remaining := resp.FileSize - rangeStart
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rangeStart, resp.FileSize-1, resp.FileSize))
		w.Header().Set("Content-Length", strconv.FormatInt(remaining, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.FileSize, 10))
		w.WriteHeader(http.StatusOK)
	}

	// Pipe audio from libp2p stream to HTTP response
	io.Copy(w, s)
}

func openFileForStream(path string) (*fileHandle, error) {
	return openFile(path)
}

func writeStreamError(s network.Stream, msg string) {
	resp := StreamResponse{OK: false, Error: msg}
	respBytes, _ := json.Marshal(resp)
	s.Write(respBytes)
	s.Write([]byte("\n"))
}

func formatToContentType(format string) string {
	switch format {
	case "flac":
		return "audio/flac"
	case "mp3":
		return "audio/mpeg"
	case "ogg", "opus":
		return "audio/ogg"
	case "m4a", "aac":
		return "audio/mp4"
	default:
		return "application/octet-stream"
	}
}
