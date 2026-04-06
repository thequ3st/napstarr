package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thequ3st/napstarr/internal/api"
	"github.com/thequ3st/napstarr/internal/auth"
	"github.com/thequ3st/napstarr/internal/config"
	"github.com/thequ3st/napstarr/internal/database"
	"github.com/thequ3st/napstarr/internal/ws"
)

func main() {
	cfg := config.Parse()

	log.Printf("Napstarr starting...")
	log.Printf("  Music dir: %s", cfg.MusicDir)
	log.Printf("  Data dir:  %s", cfg.DataDir)
	log.Printf("  Listen:    %s", cfg.ListenAddr)

	db, err := database.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := auth.EnsureAdminUser(db, cfg.AdminUser, cfg.AdminPass); err != nil {
		log.Fatalf("Failed to ensure admin user: %v", err)
	}

	hub := ws.NewHub()
	go hub.Run()

	router := api.NewRouter(db, cfg, hub)

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
