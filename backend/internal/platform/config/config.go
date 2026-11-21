// Package config loads runtime settings from environment variables.
package config

import (
	"fmt"
	"os"
)

// Config holds the runtime settings for the API server.
type Config struct {
	DBURL         string
	HTTPAddr      string
	LinkSecret    []byte
	UploadDir     string
	SecureCookies bool
}

// Load reads settings from the environment. NEXFLOW_DB_URL is required.
// NEXFLOW_HTTP_ADDR defaults to ":8080".
func Load() (Config, error) {
	cfg := Config{
		DBURL:         os.Getenv("NEXFLOW_DB_URL"),
		HTTPAddr:      os.Getenv("NEXFLOW_HTTP_ADDR"),
		LinkSecret:    []byte(os.Getenv("NEXFLOW_LINK_SECRET")),
		UploadDir:     os.Getenv("NEXFLOW_UPLOAD_DIR"),
		SecureCookies: os.Getenv("NEXFLOW_INSECURE_COOKIES") != "true",
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.DBURL == "" {
		return Config{}, fmt.Errorf("config: NEXFLOW_DB_URL is not set")
	}
	if len(cfg.LinkSecret) < 32 {
		return Config{}, fmt.Errorf("config: NEXFLOW_LINK_SECRET must be at least 32 characters")
	}
	if cfg.UploadDir == "" {
		cfg.UploadDir = "uploads"
	}
	return cfg, nil
}
