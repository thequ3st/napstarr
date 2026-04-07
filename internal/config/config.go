package config

import (
	"flag"
	"os"
)

type Config struct {
	MusicDir     string
	DataDir      string
	ListenAddr   string
	AdminUser    string
	AdminPass    string
	InstanceName string

	// Runtime — set after init, not from flags
	Federation any // *federation.Node, stored as any to avoid import cycle
}

func Parse() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.MusicDir, "music-dir", envOr("NAPSTARR_MUSIC_DIR", "/music"), "path to music library")
	flag.StringVar(&cfg.DataDir, "data-dir", envOr("NAPSTARR_DATA_DIR", "/data"), "path for database and artwork cache")
	flag.StringVar(&cfg.ListenAddr, "listen", envOr("NAPSTARR_LISTEN", ":8484"), "listen address")
	flag.StringVar(&cfg.AdminUser, "admin-user", envOr("NAPSTARR_ADMIN_USER", "admin"), "admin username")
	flag.StringVar(&cfg.AdminPass, "admin-pass", envOr("NAPSTARR_ADMIN_PASS", ""), "admin password (required on first run)")
	flag.StringVar(&cfg.InstanceName, "instance-name", envOr("NAPSTARR_INSTANCE_NAME", ""), "instance display name on the network")
	flag.Parse()

	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
