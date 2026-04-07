package p2p

import (
	"context"
	"encoding/json"
	"log"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const (
	TopicNowPlaying    = "napstarr/now-playing"
	TopicLibraryUpdate = "napstarr/library-update"
	TopicPeerAnnounce  = "napstarr/peer-announce"
)

// GossipMessage is the envelope for all gossip messages.
type GossipMessage struct {
	Type       string          `json:"type"`
	InstanceID string          `json:"instanceId"`
	PeerID     string          `json:"peerId"`
	Data       json.RawMessage `json:"data"`
}

// NowPlayingEvent is broadcast when a user starts playing a track.
type NowPlayingEvent struct {
	TrackTitle  string `json:"trackTitle"`
	ArtistName  string `json:"artistName"`
	AlbumTitle  string `json:"albumTitle"`
	ContentHash string `json:"contentHash,omitempty"`
}

// LibraryUpdateEvent is broadcast when new music is added.
type LibraryUpdateEvent struct {
	Action     string `json:"action"` // "added", "removed"
	ArtistName string `json:"artistName"`
	AlbumTitle string `json:"albumTitle"`
	TrackCount int    `json:"trackCount"`
}

// PeerAnnounceEvent is broadcast periodically so peers know we're alive.
type PeerAnnounceEvent struct {
	Name        string `json:"name"`
	ArtistCount int    `json:"artistCount"`
	AlbumCount  int    `json:"albumCount"`
	TrackCount  int    `json:"trackCount"`
}

// GossipService manages pub/sub messaging.
type GossipService struct {
	host       *Host
	instanceID string
	onMessage  func(GossipMessage)
}

// NewGossipService creates the gossip service and subscribes to all topics.
func NewGossipService(h *Host, instanceID string) *GossipService {
	gs := &GossipService{
		host:       h,
		instanceID: instanceID,
	}

	// Subscribe to all topics
	topics := []string{TopicNowPlaying, TopicLibraryUpdate, TopicPeerAnnounce}
	for _, t := range topics {
		sub, err := h.Subscribe(t)
		if err != nil {
			log.Printf("gossip: failed to subscribe to %s: %v", t, err)
			continue
		}
		go gs.readMessages(t, sub)
	}

	return gs
}

// SetMessageHandler sets the callback for incoming gossip messages.
func (gs *GossipService) SetMessageHandler(fn func(GossipMessage)) {
	gs.onMessage = fn
}

// PublishNowPlaying broadcasts what we're currently playing.
func (gs *GossipService) PublishNowPlaying(event NowPlayingEvent) error {
	return gs.publish(TopicNowPlaying, "now_playing", event)
}

// PublishLibraryUpdate broadcasts a library change.
func (gs *GossipService) PublishLibraryUpdate(event LibraryUpdateEvent) error {
	return gs.publish(TopicLibraryUpdate, "library_update", event)
}

// PublishPeerAnnounce broadcasts our presence and stats.
func (gs *GossipService) PublishPeerAnnounce(event PeerAnnounceEvent) error {
	return gs.publish(TopicPeerAnnounce, "peer_announce", event)
}

func (gs *GossipService) publish(topic, msgType string, data any) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	msg := GossipMessage{
		Type:       msgType,
		InstanceID: gs.instanceID,
		PeerID:     gs.host.PeerID().String(),
		Data:       dataBytes,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return gs.host.Publish(topic, msgBytes)
}

func (gs *GossipService) readMessages(topic string, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(context.Background())
		if err != nil {
			log.Printf("gossip: error reading %s: %v", topic, err)
			return
		}

		// Skip our own messages
		if msg.ReceivedFrom == gs.host.PeerID() {
			continue
		}

		var gm GossipMessage
		if err := json.Unmarshal(msg.Data, &gm); err != nil {
			continue
		}

		log.Printf("gossip: [%s] from %s: %s", topic, gm.InstanceID[:8], gm.Type)

		if gs.onMessage != nil {
			gs.onMessage(gm)
		}
	}
}
