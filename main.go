package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"strings"

	"github.com/thequ3st/napstarr/internal/api"
	"github.com/thequ3st/napstarr/internal/auth"
	"github.com/thequ3st/napstarr/internal/config"
	"github.com/thequ3st/napstarr/internal/database"
	"github.com/thequ3st/napstarr/internal/federation"
	"github.com/thequ3st/napstarr/internal/identity"
	"github.com/thequ3st/napstarr/internal/p2p"
	"github.com/thequ3st/napstarr/internal/ws"
)

func main() {
	cfg := config.Parse()

	log.Printf("Napstarr starting...")
	log.Printf("  Music dir: %s", cfg.MusicDir)
	log.Printf("  Data dir:  %s", cfg.DataDir)
	log.Printf("  Listen:    %s", cfg.ListenAddr)

	// Load or generate instance identity
	inst, err := identity.LoadOrCreate(cfg.DataDir, cfg.InstanceName)
	if err != nil {
		log.Fatalf("Failed to initialize identity: %v", err)
	}
	log.Printf("  Instance:  %s (%s)", inst.ID, inst.Name)
	log.Printf("  Public key: %x", inst.PublicKey[:8])

	db, err := database.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := auth.EnsureAdminUser(db, cfg.AdminUser, cfg.AdminPass); err != nil {
		log.Fatalf("Failed to ensure admin user: %v", err)
	}

	// Initialize federation node
	node := federation.NewNode(inst, db)
	cfg.Federation = node

	// Start libp2p host
	var bootstrapPeers []string
	if cfg.P2PBootstrap != "" {
		bootstrapPeers = strings.Split(cfg.P2PBootstrap, ",")
	}
	p2pHost, err := p2p.NewHost(inst, p2p.Config{
		ListenPort: cfg.P2PPort,
		Bootstrap:  bootstrapPeers,
	})
	if err != nil {
		log.Fatalf("Failed to start P2P host: %v", err)
	}
	defer p2pHost.Close()
	cfg.P2PHost = p2pHost

	// Start transfer and stream services
	_ = p2p.NewTransferService(p2pHost, db, cfg.MusicDir, cfg.DataDir)
	_ = p2p.NewStreamService(p2pHost, db)

	// Start gossip service
	gossip := p2p.NewGossipService(p2pHost, inst.ID)
	gossip.SetMessageHandler(func(msg p2p.GossipMessage) {
		log.Printf("gossip: received %s from %s", msg.Type, msg.InstanceID)
	})

	hub := ws.NewHub()
	go hub.Run()

	router := api.NewRouter(db, cfg, hub, inst)

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // disabled for audio streaming
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("Napstarr listening on %s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
