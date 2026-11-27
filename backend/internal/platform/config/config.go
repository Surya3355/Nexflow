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
	// TrustProxy makes the login limit read the client address from the
	// X-Real-IP header nginx sets. Only when the API is reachable through nginx alone.
	TrustProxy bool
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
		TrustProxy:    os.Getenv("NEXFLOW_TRUST_PROXY") == "true",
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
	if weakSecret(cfg.LinkSecret) {
		return Config{}, fmt.Errorf("config: NEXFLOW_LINK_SECRET is the example value or too simple; make one with: openssl rand -hex 32")
	}
	if cfg.UploadDir == "" {
		cfg.UploadDir = "uploads"
	}
	return cfg, nil
}

// exampleSecret is the placeholder in .env.example. It is public, so a server
// using it would let anyone sign download links.
const exampleSecret = "change-me-to-32-or-more-random-characters"

// minSecretVariety is the fewest different characters a link secret may have.
// Random hex (16 possible digits) clears it easily; repeated text does not.
const minSecretVariety = 10

// weakSecret reports whether s is the public example value or has too little variety.
func weakSecret(s []byte) bool {
	if string(s) == exampleSecret {
		return true
	}
	seen := map[byte]bool{}
	for _, b := range s {
		seen[b] = true
	}
	return len(seen) < minSecretVariety
}
