package config

import "testing"

func TestLoadRefusesShortLinkSecret(t *testing.T) {
	t.Setenv("NEXFLOW_DB_URL", "postgres://x")
	t.Setenv("NEXFLOW_LINK_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a link secret shorter than 32 characters")
	}
}

func TestLoadDefaultsSecureCookies(t *testing.T) {
	t.Setenv("NEXFLOW_DB_URL", "postgres://x")
	t.Setenv("NEXFLOW_LINK_SECRET", "0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SecureCookies {
		t.Fatal("cookies are not secure by default")
	}
}
