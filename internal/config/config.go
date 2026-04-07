package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	MusicDir     string
	DataDir      string
	ListenAddr   string
	AdminUser    string
	AdminPass    string
	InstanceName string
	P2PPort      int
	P2PBootstrap string

	// Runtime — set after init, not from flags
	Federation any // *federation.Node, stored as any to avoid import cycle
	P2PHost    any // *p2p.Host, stored as any to avoid import cycle
}

func Parse() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.MusicDir, "music-dir", envOr("NAPSTARR_MUSIC_DIR", "/music"), "path to music library")
	flag.StringVar(&cfg.DataDir, "data-dir", envOr("NAPSTARR_DATA_DIR", "/data"), "path for database and artwork cache")
	flag.StringVar(&cfg.ListenAddr, "listen", envOr("NAPSTARR_LISTEN", ":8484"), "listen address")
	flag.StringVar(&cfg.AdminUser, "admin-user", envOr("NAPSTARR_ADMIN_USER", "admin"), "admin username")
	flag.StringVar(&cfg.AdminPass, "admin-pass", envOr("NAPSTARR_ADMIN_PASS", ""), "admin password (required on first run)")
	flag.StringVar(&cfg.InstanceName, "instance-name", envOr("NAPSTARR_INSTANCE_NAME", ""), "instance display name on the network")
	flag.IntVar(&cfg.P2PPort, "p2p-port", envOrInt("NAPSTARR_P2P_PORT", 4001), "libp2p listen port")
	flag.StringVar(&cfg.P2PBootstrap, "p2p-bootstrap", envOr("NAPSTARR_P2P_BOOTSTRAP", ""), "bootstrap peer multiaddr (comma-separated)")
	flag.Parse()

	return cfg
}

func envOrInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n := 0
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			return n
		}
	}
	return fallback
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
