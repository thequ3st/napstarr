package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/thequ3st/napstarr/internal/database"
	"github.com/thequ3st/napstarr/internal/identity"
	"github.com/thequ3st/napstarr/internal/library"
)

// Node manages federation with other Napstarr instances.
type Node struct {
	Instance *identity.Instance
	DB       *database.DB
	mu       sync.RWMutex
	peers    map[string]*PeerConn // instance_id -> connection
	client   *http.Client
}

// PeerInfo represents a known peer.
type PeerInfo struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
	PublicKey  string `json:"publicKey"`
	Address    string `json:"address"`
	LastSeen   string `json:"lastSeen,omitempty"`
	LastSynced string `json:"lastSynced,omitempty"`
	Status     string `json:"status"`
	Stats      *database.LibraryStats `json:"stats,omitempty"`
}

// PeerConn tracks an active peer connection.
type PeerConn struct {
	Info    PeerInfo
	cancel  context.CancelFunc
}

// RemoteLibrary is the library index a peer shares with us.
type RemoteLibrary struct {
	InstanceID string          `json:"instanceId"`
	Artists    []RemoteArtist  `json:"artists"`
}

type RemoteArtist struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	SortName   string        `json:"sortName"`
	Albums     []RemoteAlbum `json:"albums"`
}

type RemoteAlbum struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Year        *int          `json:"year,omitempty"`
	TrackCount  int           `json:"trackCount"`
	HasArtwork  bool          `json:"hasArtwork"`
	Tracks      []RemoteTrack `json:"tracks"`
}

type RemoteTrack struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	TrackNumber *int   `json:"trackNumber,omitempty"`
	DiscNumber  int    `json:"discNumber"`
	DurationMs  int    `json:"durationMs"`
	Format      string `json:"format"`
	ContentHash string `json:"contentHash"`
	FileSize    int64  `json:"fileSize"`
}

// NewNode creates a federation node.
func NewNode(inst *identity.Instance, db *database.DB) *Node {
	return &Node{
		Instance: inst,
		DB:       db,
		peers:    make(map[string]*PeerConn),
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// FollowPeer connects to a remote instance by address and stores it as a peer.
func (n *Node) FollowPeer(address string) (*PeerInfo, error) {
	// Fetch remote instance info
	resp, err := n.client.Get(address + "/api/instance")
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", address, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("peer returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var info struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		PublicKey string `json:"publicKey"`
		Version   string `json:"version"`
		Protocol  string `json:"protocol"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("parse instance info: %w", err)
	}

	if info.ID == "" || info.PublicKey == "" {
		return nil, fmt.Errorf("invalid instance info from %s", address)
	}

	if info.ID == n.Instance.ID {
		return nil, fmt.Errorf("cannot follow yourself")
	}

	// Check if already following
	var existingID string
	err = n.DB.Reader.QueryRow("SELECT id FROM peers WHERE instance_id = ?", info.ID).Scan(&existingID)
	if err == nil {
		// Update address and status
		n.DB.Writer.Exec("UPDATE peers SET address = ?, name = ?, status = 'active', last_seen = ? WHERE id = ?",
			address, info.Name, time.Now().UTC().Format(time.RFC3339), existingID)
		return &PeerInfo{
			ID: existingID, InstanceID: info.ID, Name: info.Name,
			PublicKey: info.PublicKey, Address: address, Status: "active",
		}, nil
	}

	// Insert new peer
	peerID := database.NewID()
	_, err = n.DB.Writer.Exec(
		`INSERT INTO peers (id, instance_id, name, public_key, address, last_seen, status)
		 VALUES (?, ?, ?, ?, ?, ?, 'active')`,
		peerID, info.ID, info.Name, info.PublicKey, address,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("store peer: %w", err)
	}

	log.Printf("federation: followed peer %s (%s) at %s", info.Name, info.ID, address)

	peer := &PeerInfo{
		ID: peerID, InstanceID: info.ID, Name: info.Name,
		PublicKey: info.PublicKey, Address: address, Status: "active",
	}

	// Sync library in background
	go n.SyncPeerLibrary(peer)

	return peer, nil
}

// UnfollowPeer removes a peer and all their remote data.
func (n *Node) UnfollowPeer(peerID string) error {
	tx, err := n.DB.Writer.Begin()
	if err != nil {
		return err
	}

	tx.Exec("DELETE FROM remote_tracks WHERE peer_id = ?", peerID)
	tx.Exec("DELETE FROM remote_albums WHERE peer_id = ?", peerID)
	tx.Exec("DELETE FROM remote_artists WHERE peer_id = ?", peerID)
	tx.Exec("DELETE FROM peer_activity WHERE peer_id = ?", peerID)
	tx.Exec("DELETE FROM peers WHERE id = ?", peerID)

	return tx.Commit()
}

// GetPeers returns all followed peers.
func (n *Node) GetPeers() ([]PeerInfo, error) {
	rows, err := n.DB.Reader.Query(
		`SELECT id, instance_id, name, public_key, address, last_seen, last_synced, status FROM peers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	peers := make([]PeerInfo, 0)
	for rows.Next() {
		var p PeerInfo
		var lastSeen, lastSynced *string
		rows.Scan(&p.ID, &p.InstanceID, &p.Name, &p.PublicKey, &p.Address, &lastSeen, &lastSynced, &p.Status)
		if lastSeen != nil {
			p.LastSeen = *lastSeen
		}
		if lastSynced != nil {
			p.LastSynced = *lastSynced
		}
		peers = append(peers, p)
	}
	return peers, nil
}

// SyncPeerLibrary fetches a peer's library index and stores it locally.
func (n *Node) SyncPeerLibrary(peer *PeerInfo) error {
	log.Printf("federation: syncing library from %s (%s)", peer.Name, peer.InstanceID)

	// Fetch their library export
	resp, err := n.client.Get(peer.Address + "/api/federation/library")
	if err != nil {
		return fmt.Errorf("fetch library from %s: %w", peer.Address, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("peer %s returned HTTP %d for library", peer.Name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024)) // 100MB max
	if err != nil {
		return fmt.Errorf("read library: %w", err)
	}

	var lib RemoteLibrary
	if err := json.Unmarshal(body, &lib); err != nil {
		return fmt.Errorf("parse library: %w", err)
	}

	// Store in DB
	tx, err := n.DB.Writer.Begin()
	if err != nil {
		return err
	}

	// Clear old data for this peer
	tx.Exec("DELETE FROM remote_tracks WHERE peer_id = ?", peer.ID)
	tx.Exec("DELETE FROM remote_albums WHERE peer_id = ?", peer.ID)
	tx.Exec("DELETE FROM remote_artists WHERE peer_id = ?", peer.ID)

	trackCount := 0
	for _, artist := range lib.Artists {
		artistID := database.NewID()
		tx.Exec(`INSERT INTO remote_artists (id, peer_id, name, sort_name, album_count, track_count)
			VALUES (?, ?, ?, ?, ?, ?)`,
			artistID, peer.ID, artist.Name, artist.SortName,
			len(artist.Albums), countTracks(artist))

		for _, album := range artist.Albums {
			albumID := database.NewID()
			tx.Exec(`INSERT INTO remote_albums (id, peer_id, artist_id, artist_name, title, year, track_count, has_artwork)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				albumID, peer.ID, artistID, artist.Name, album.Title, album.Year,
				len(album.Tracks), album.HasArtwork)

			for _, track := range album.Tracks {
				// Use the original remote track ID so we can reference it for streaming
				remoteTrackID := track.ID
				if remoteTrackID == "" {
					remoteTrackID = database.NewID()
				}
				tx.Exec(`INSERT INTO remote_tracks (id, peer_id, album_id, artist_name, album_title, title,
					track_number, disc_number, duration_ms, format, content_hash, file_size)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					remoteTrackID, peer.ID, albumID, artist.Name, album.Title,
					track.Title, track.TrackNumber, track.DiscNumber, track.DurationMs,
					track.Format, track.ContentHash, track.FileSize)
				trackCount++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit library sync: %w", err)
	}

	// Update last_synced
	n.DB.Writer.Exec("UPDATE peers SET last_synced = ?, last_seen = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
		peer.ID)

	log.Printf("federation: synced %d artists, %d tracks from %s",
		len(lib.Artists), trackCount, peer.Name)

	return nil
}

// ExportLibrary creates a library index for sharing with peers.
func (n *Node) ExportLibrary() (*RemoteLibrary, error) {
	artists, err := library.GetArtists(n.DB)
	if err != nil {
		return nil, err
	}

	lib := &RemoteLibrary{
		InstanceID: n.Instance.ID,
		Artists:    make([]RemoteArtist, 0, len(artists)),
	}

	for _, a := range artists {
		albums, err := library.GetArtistAlbums(n.DB, a.ID)
		if err != nil {
			continue
		}

		ra := RemoteArtist{
			ID:       a.ID,
			Name:     a.Name,
			SortName: a.SortName,
			Albums:   make([]RemoteAlbum, 0, len(albums)),
		}

		for _, al := range albums {
			full, err := library.GetAlbum(n.DB, al.ID)
			if err != nil {
				continue
			}

			rAlbum := RemoteAlbum{
				ID:         al.ID,
				Title:      al.Title,
				Year:       al.Year,
				TrackCount: len(full.Tracks),
				HasArtwork: al.HasArtwork,
				Tracks:     make([]RemoteTrack, 0, len(full.Tracks)),
			}

			for _, t := range full.Tracks {
				rAlbum.Tracks = append(rAlbum.Tracks, RemoteTrack{
					ID:          t.ID,
					Title:       t.Title,
					TrackNumber: t.TrackNumber,
					DiscNumber:  t.DiscNumber,
					DurationMs:  t.DurationMs,
					Format:      t.Format,
					ContentHash: t.ID, // TODO: real content hash
					FileSize:    t.FileSize,
				})
			}

			ra.Albums = append(ra.Albums, rAlbum)
		}

		lib.Artists = append(lib.Artists, ra)
	}

	return lib, nil
}

func countTracks(a RemoteArtist) int {
	n := 0
	for _, al := range a.Albums {
		n += len(al.Tracks)
	}
	return n
}
