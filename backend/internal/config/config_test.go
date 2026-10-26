package config

import (
	"net/url"
	"testing"
)

func TestLoadCapitalLimit(t *testing.T) {
	t.Setenv("MAX_CAPITAL_CHF", "5001")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted capital above the compiled ceiling")
	}
}

func TestLoadAcceptsLowerCapital(t *testing.T) {
	t.Setenv("MAX_CAPITAL_CHF", "2500")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxCapitalCHF != 2500 {
		t.Fatalf("MaxCapitalCHF = %d, want 2500", cfg.MaxCapitalCHF)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("API_PORT", "not-a-port")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a non-integer API_PORT")
	}
}

func TestLoadRejectsPublicAPIHost(t *testing.T) {
	t.Setenv("API_HOST", "0.0.0.0")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a non-loopback API_HOST")
	}
}

func TestLoadValidatesAlertScoreThreshold(t *testing.T) {
	t.Setenv("ALERT_SCORE_THRESHOLD", "82.5")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AlertScoreThreshold != 82.5 {
		t.Fatalf("AlertScoreThreshold = %v, want 82.5", cfg.AlertScoreThreshold)
	}
	t.Setenv("ALERT_SCORE_THRESHOLD", "NaN")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a NaN alert threshold")
	}
}

func TestDatabaseURLEncodesCredentials(t *testing.T) {
	t.Setenv("DB_USER", "user@example")
	t.Setenv("DB_PASSWORD", "p@ss:/?")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	parsed, err := url.Parse(cfg.DatabaseURL())
	if err != nil {
		t.Fatalf("DatabaseURL() returned invalid URL: %v", err)
	}
	password, ok := parsed.User.Password()
	if !ok || password != "p@ss:/?" || parsed.User.Username() != "user@example" {
		t.Fatalf("DatabaseURL() did not preserve encoded credentials: %q", parsed)
	}
}
