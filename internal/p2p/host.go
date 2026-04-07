package p2p

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/websocket"

	"github.com/thequ3st/napstarr/internal/identity"
)

const (
	// Protocol IDs
	ProtocolLibrary  = protocol.ID("/napstarr/library/1.0.0")
	ProtocolTransfer = protocol.ID("/napstarr/transfer/1.0.0")

	// DHT namespace for Napstarr peer discovery
	RendezvousString = "napstarr/network/v1"
)

// Host wraps a libp2p host with Napstarr-specific functionality.
type Host struct {
	host      host.Host
	dht       *dht.IpfsDHT
	pubsub    *pubsub.PubSub
	discovery *drouting.RoutingDiscovery
	ctx       context.Context
	cancel    context.CancelFunc
	instance  *identity.Instance

	// Subscriptions
	mu     sync.RWMutex
	topics map[string]*pubsub.Topic
	subs   map[string]*pubsub.Subscription

	// Callbacks
	onLibraryRequest func(peerID peer.ID, stream network.Stream)
	onTransferRequest func(peerID peer.ID, stream network.Stream)
}

// Config for the P2P host.
type Config struct {
	ListenPort int    // Port to listen on (0 = random)
	Bootstrap  []string // Bootstrap peer addresses (multiaddrs)
}

// NewHost creates and starts a libp2p host with DHT and GossipSub.
func NewHost(inst *identity.Instance, cfg Config) (*Host, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Convert Ed25519 private key to libp2p format
	privKey, err := crypto.UnmarshalEd25519PrivateKey(inst.PrivateKey)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("convert private key: %w", err)
	}

	listenAddr := fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.ListenPort)
	listenAddrQuic := fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", cfg.ListenPort)

	// Create libp2p host
	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(
			listenAddr,
			listenAddrQuic,
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d/ws", cfg.ListenPort+1),
		),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(websocket.New),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	log.Printf("p2p: libp2p host started")
	log.Printf("p2p: peer ID: %s", h.ID())
	for _, addr := range h.Addrs() {
		log.Printf("p2p: listening on %s/p2p/%s", addr, h.ID())
	}

	// Create DHT
	kdht, err := dht.New(ctx, h, dht.Mode(dht.ModeAutoServer))
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("create DHT: %w", err)
	}

	if err := kdht.Bootstrap(ctx); err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("bootstrap DHT: %w", err)
	}

	// Connect to bootstrap peers
	for _, addrStr := range cfg.Bootstrap {
		if addrStr == "" {
			continue
		}
		peerInfo, err := peer.AddrInfoFromString(addrStr)
		if err != nil {
			log.Printf("p2p: invalid bootstrap addr %s: %v", addrStr, err)
			continue
		}
		go func(pi peer.AddrInfo) {
			if err := h.Connect(ctx, pi); err != nil {
				log.Printf("p2p: bootstrap connect to %s failed: %v", pi.ID.String()[:16], err)
			} else {
				log.Printf("p2p: connected to bootstrap peer %s", pi.ID.String()[:16])
			}
		}(*peerInfo)
	}

	// Create GossipSub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("create GossipSub: %w", err)
	}

	// Create routing discovery
	disc := drouting.NewRoutingDiscovery(kdht)

	p2pHost := &Host{
		host:      h,
		dht:       kdht,
		pubsub:    ps,
		discovery: disc,
		ctx:       ctx,
		cancel:    cancel,
		instance:  inst,
		topics:    make(map[string]*pubsub.Topic),
		subs:      make(map[string]*pubsub.Subscription),
	}

	// Register stream handlers
	h.SetStreamHandler(ProtocolLibrary, p2pHost.handleLibraryStream)
	h.SetStreamHandler(ProtocolTransfer, p2pHost.handleTransferStream)

	// Announce ourselves on the DHT
	go p2pHost.announce()

	// Start peer discovery
	go p2pHost.discoverPeers()

	return p2pHost, nil
}

// announce advertises our presence on the DHT.
func (h *Host) announce() {
	for {
		_, err := h.discovery.Advertise(h.ctx, RendezvousString)
		if err != nil {
			log.Printf("p2p: DHT advertise error: %v", err)
		} else {
			log.Printf("p2p: announced on DHT")
		}

		select {
		case <-h.ctx.Done():
			return
		case <-time.After(10 * time.Minute):
		}
	}
}

// discoverPeers finds other Napstarr nodes on the DHT.
func (h *Host) discoverPeers() {
	// Wait for DHT to warm up
	time.Sleep(5 * time.Second)

	for {
		peerChan, err := h.discovery.FindPeers(h.ctx, RendezvousString)
		if err != nil {
			log.Printf("p2p: peer discovery error: %v", err)
		} else {
			for p := range peerChan {
				if p.ID == h.host.ID() || len(p.Addrs) == 0 {
					continue
				}
				// Try to connect
				if h.host.Network().Connectedness(p.ID) != network.Connected {
					go func(pi peer.AddrInfo) {
						ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
						defer cancel()
						if err := h.host.Connect(ctx, pi); err == nil {
							log.Printf("p2p: discovered and connected to %s", pi.ID.String()[:16])
						}
					}(p)
				}
			}
		}

		select {
		case <-h.ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

// handleLibraryStream handles incoming library index requests from peers.
func (h *Host) handleLibraryStream(s network.Stream) {
	defer s.Close()
	if h.onLibraryRequest != nil {
		h.onLibraryRequest(s.Conn().RemotePeer(), s)
	}
}

// handleTransferStream handles incoming file transfer requests from peers.
func (h *Host) handleTransferStream(s network.Stream) {
	defer s.Close()
	if h.onTransferRequest != nil {
		h.onTransferRequest(s.Conn().RemotePeer(), s)
	}
}

// SetLibraryHandler sets the callback for library requests.
func (h *Host) SetLibraryHandler(fn func(peer.ID, network.Stream)) {
	h.onLibraryRequest = fn
}

// SetTransferHandler sets the callback for transfer requests.
func (h *Host) SetTransferHandler(fn func(peer.ID, network.Stream)) {
	h.onTransferRequest = fn
}

// PeerID returns our libp2p peer ID.
func (h *Host) PeerID() peer.ID {
	return h.host.ID()
}

// Addrs returns our listen addresses.
func (h *Host) Addrs() []string {
	addrs := make([]string, 0)
	for _, addr := range h.host.Addrs() {
		addrs = append(addrs, fmt.Sprintf("%s/p2p/%s", addr, h.host.ID()))
	}
	return addrs
}

// ConnectedPeers returns currently connected peer IDs.
func (h *Host) ConnectedPeers() []peer.ID {
	return h.host.Network().Peers()
}

// Connect to a specific peer by multiaddr.
func (h *Host) Connect(addr string) error {
	peerInfo, err := peer.AddrInfoFromString(addr)
	if err != nil {
		return fmt.Errorf("parse multiaddr: %w", err)
	}

	ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
	defer cancel()

	return h.host.Connect(ctx, *peerInfo)
}

// RequestLibrary opens a stream to a peer and requests their library index.
func (h *Host) RequestLibrary(peerID peer.ID) (network.Stream, error) {
	ctx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer cancel()
	return h.host.NewStream(ctx, peerID, ProtocolLibrary)
}

// RequestTransfer opens a stream to a peer for file transfer.
func (h *Host) RequestTransfer(peerID peer.ID) (network.Stream, error) {
	ctx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer cancel()
	return h.host.NewStream(ctx, peerID, ProtocolTransfer)
}

// Subscribe joins a GossipSub topic and returns the subscription.
func (h *Host) Subscribe(topicName string) (*pubsub.Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if sub, ok := h.subs[topicName]; ok {
		return sub, nil
	}

	topic, err := h.pubsub.Join(topicName)
	if err != nil {
		return nil, fmt.Errorf("join topic %s: %w", topicName, err)
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("subscribe to %s: %w", topicName, err)
	}

	h.topics[topicName] = topic
	h.subs[topicName] = sub
	return sub, nil
}

// Publish sends a message to a GossipSub topic.
func (h *Host) Publish(topicName string, data []byte) error {
	h.mu.RLock()
	topic, ok := h.topics[topicName]
	h.mu.RUnlock()

	if !ok {
		// Auto-join if not yet subscribed
		var err error
		topic, err = h.pubsub.Join(topicName)
		if err != nil {
			return fmt.Errorf("join topic %s: %w", topicName, err)
		}
		h.mu.Lock()
		h.topics[topicName] = topic
		h.mu.Unlock()
	}

	return topic.Publish(h.ctx, data)
}

// Close shuts down the host.
func (h *Host) Close() error {
	h.cancel()
	h.dht.Close()
	return h.host.Close()
}

// suppress unused import
var _ = ed25519.PublicKeySize
