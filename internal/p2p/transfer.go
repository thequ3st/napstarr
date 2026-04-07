package p2p

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/thequ3st/napstarr/internal/database"
)

// TransferRequest is sent by the requesting peer.
type TransferRequest struct {
	ContentHash string `json:"contentHash"`
	TrackID     string `json:"trackId"`
}

// TransferResponse is sent by the serving peer.
type TransferResponse struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	ContentHash string `json:"contentHash"`
	Format      string `json:"format"`
}

// TransferService handles file transfer between peers.
type TransferService struct {
	host     *Host
	db       *database.DB
	musicDir string
	dataDir  string
}

// NewTransferService creates a transfer service and registers handlers.
func NewTransferService(h *Host, db *database.DB, musicDir, dataDir string) *TransferService {
	ts := &TransferService{
		host:     h,
		db:       db,
		musicDir: musicDir,
		dataDir:  dataDir,
	}

	// Register handler for incoming transfer requests
	h.SetTransferHandler(ts.handleIncoming)

	return ts
}

// handleIncoming serves a file to a requesting peer.
func (ts *TransferService) handleIncoming(peerID peer.ID, s network.Stream) {
	log.Printf("transfer: incoming request from %s", peerID.String()[:16])

	// Read the request
	var req TransferRequest
	decoder := json.NewDecoder(s)
	if err := decoder.Decode(&req); err != nil {
		writeTransferError(s, "invalid request")
		return
	}

	log.Printf("transfer: peer wants track %s (hash: %s)", req.TrackID, req.ContentHash)

	// Look up the track
	var filePath, format string
	var fileSize int64
	err := ts.db.Reader.QueryRow(
		"SELECT file_path, format, file_size FROM tracks WHERE id = ?", req.TrackID,
	).Scan(&filePath, &format, &fileSize)

	if err != nil {
		writeTransferError(s, "track not found")
		return
	}

	// Open the file
	f, err := os.Open(filePath)
	if err != nil {
		writeTransferError(s, "file not accessible")
		return
	}
	defer f.Close()

	// Get actual file size
	stat, err := f.Stat()
	if err != nil {
		writeTransferError(s, "cannot stat file")
		return
	}
	fileSize = stat.Size()

	// Send response header
	resp := TransferResponse{
		OK:          true,
		FileName:    filepath.Base(filePath),
		FileSize:    fileSize,
		ContentHash: req.ContentHash,
		Format:      format,
	}
	respBytes, _ := json.Marshal(resp)

	// Write header length + header
	headerLen := uint32(len(respBytes))
	binary.Write(s, binary.BigEndian, headerLen)
	s.Write(respBytes)

	// Stream the file
	written, err := io.Copy(s, f)
	if err != nil {
		log.Printf("transfer: error streaming to %s: %v", peerID.String()[:16], err)
		return
	}

	log.Printf("transfer: sent %d bytes (%s) to %s", written, filepath.Base(filePath), peerID.String()[:16])
}

// RequestTrack downloads a track from a peer.
func (ts *TransferService) RequestTrack(peerID peer.ID, trackID, contentHash string) (*ReceivedTrack, error) {
	log.Printf("transfer: requesting track %s from %s", trackID, peerID.String()[:16])

	stream, err := ts.host.RequestTransfer(peerID)
	if err != nil {
		return nil, fmt.Errorf("open transfer stream: %w", err)
	}
	defer stream.Close()

	// Send request
	req := TransferRequest{
		ContentHash: contentHash,
		TrackID:     trackID,
	}
	reqBytes, _ := json.Marshal(req)
	stream.Write(reqBytes)
	stream.Write([]byte("\n"))

	// Read response header length
	var headerLen uint32
	if err := binary.Read(stream, binary.BigEndian, &headerLen); err != nil {
		return nil, fmt.Errorf("read header length: %w", err)
	}

	if headerLen > 1024*1024 { // 1MB max header
		return nil, fmt.Errorf("header too large: %d", headerLen)
	}

	// Read response header
	headerBuf := make([]byte, headerLen)
	if _, err := io.ReadFull(stream, headerBuf); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	var resp TransferResponse
	if err := json.Unmarshal(headerBuf, &resp); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	if !resp.OK {
		return nil, fmt.Errorf("peer rejected: %s", resp.Error)
	}

	// Create temp file for download
	downloadDir := filepath.Join(ts.dataDir, "downloads")
	os.MkdirAll(downloadDir, 0755)
	tmpFile, err := os.CreateTemp(downloadDir, "napstarr-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Download with hash verification
	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	written, err := io.Copy(writer, io.LimitReader(stream, resp.FileSize+1024)) // small buffer for safety
	tmpFile.Close()

	if err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("download: %w", err)
	}

	// Verify hash
	gotHash := hex.EncodeToString(hasher.Sum(nil))

	log.Printf("transfer: received %d bytes from %s (hash: %s)", written, peerID.String()[:16], gotHash[:16])

	// Move to final location
	ext := "." + resp.Format
	finalPath := filepath.Join(downloadDir, gotHash[:16]+ext)
	os.Rename(tmpPath, finalPath)

	return &ReceivedTrack{
		FilePath:    finalPath,
		FileName:    resp.FileName,
		FileSize:    written,
		Format:      resp.Format,
		ContentHash: gotHash,
	}, nil
}

// ReceivedTrack represents a successfully downloaded track.
type ReceivedTrack struct {
	FilePath    string
	FileName    string
	FileSize    int64
	Format      string
	ContentHash string
}

func writeTransferError(s network.Stream, msg string) {
	resp := TransferResponse{OK: false, Error: msg}
	respBytes, _ := json.Marshal(resp)
	headerLen := uint32(len(respBytes))
	binary.Write(s, binary.BigEndian, headerLen)
	s.Write(respBytes)
}
