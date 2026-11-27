package config

import (
	"strings"
	"testing"
)

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

func loadWithSecret(t *testing.T, secret string) error {
	t.Helper()
	t.Setenv("NEXFLOW_DB_URL", "postgres://x")
	t.Setenv("NEXFLOW_LINK_SECRET", secret)
	_, err := Load()
	return err
}

func TestLoadRejectsExampleSecret(t *testing.T) {
	err := loadWithSecret(t, "change-me-to-32-or-more-random-characters")
	if err == nil || !strings.Contains(err.Error(), "NEXFLOW_LINK_SECRET") {
		t.Fatalf("example secret: got %v, want an error naming NEXFLOW_LINK_SECRET", err)
	}
}

func TestLoadRejectsLowVarietySecret(t *testing.T) {
	if err := loadWithSecret(t, "aaaaaaaaaaaaaaaaaaaabbbbbbbbbbbbbbbb"); err == nil {
		t.Fatal("accepted a secret with only 2 different characters")
	}
}

func TestLoadAcceptsRandomSecret(t *testing.T) {
	if err := loadWithSecret(t, "9f2c4e71b0a85d36e1f47c29ab03d58e6c17f2940be5a3d8c61f07e2b94d5a3c"); err != nil {
		t.Fatalf("random secret refused: %v", err)
	}
}

// A hex secret missing a few of its 16 possible digits is still random.
func TestLoadAcceptsHexSecretMissingDigits(t *testing.T) {
	if err := loadWithSecret(t, "0123456789ab0123456789ab0123456789ab0123456789ab0123456789ab0123"); err != nil {
		t.Fatalf("hex secret with 12 different digits refused: %v", err)
	}
}
