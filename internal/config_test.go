package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("ADMIN_CLIENT_ID", "9")
	t.Setenv("OAUTH_ISSUER", "https://auth.example.invalid/")
	t.Setenv("CONFIG_TEST_SECRET", "fixture-secret")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("cloudflare:\n  turnstile:\n    key: fixture\n    secret: ${CONFIG_TEST_SECRET}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	original := AuthServerConfig
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.ClientID != 9 || cfg.Issuer != "https://auth.example.invalid" || cfg.Port != 8080 || cfg.Redis.Port != "6379" || len(cfg.JWT) != 1 || cfg.JWT[0].Alg != "RS256" || cfg.Cloudflare.Turnstile.Secret != "fixture-secret" {
		t.Fatalf("bad defaults/overrides: client=%d issuer=%q", cfg.Admin.ClientID, cfg.Issuer)
	}
	if AuthServerConfig.Port != original.Port || AuthServerConfig.Admin.ClientID != original.Admin.ClientID {
		t.Fatal("LoadConfig changed global config")
	}
}

func TestLoadConfigReturnsErrors(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing config accepted")
	}
	for _, tc := range []struct{ name, source, expected string }{
		{"yaml", "host: [", "parse config"},
		{"env", "db: ${AUTH_REGRESSION_UNSET_VARIABLE}\n", "AUTH_REGRESSION_UNSET_VARIABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), tc.expected) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
