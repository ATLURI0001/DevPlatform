package platform

import "testing"

func TestDatabaseConfigDSN(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "db.internal",
		Port:     5432,
		User:     "app",
		Password: "secret:p@ss",
		Name:     "devplatform",
		SSLMode:  "require",
	}
	want := "postgres://app:secret%3Ap%40ss@db.internal:5432/devplatform?sslmode=require"
	if got := cfg.DSN(); got != want {
		t.Fatalf("DSN() mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestDatabaseConfigDSN_IPv6AndLeadingSlash(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "::1",
		Port:     0,
		User:     "app",
		Password: "pw",
		Name:     "/devplatform",
	}
	want := "postgres://app:pw@[::1]/devplatform"
	if got := cfg.DSN(); got != want {
		t.Fatalf("DSN() mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestConfigValidate(t *testing.T) {
	var nilCfg *Config
	if err := nilCfg.Validate(); err == nil {
		t.Fatalf("expected error on nil config")
	}

	cfg := LoadConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid default config, got %v", err)
	}

	cfg.Server.ReadTimeout = -1
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on negative ReadTimeout")
	}
	cfg.Server.ReadTimeout = 15

	cfg.Database.Port = 70000
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on invalid port 70000")
	}
	cfg.Database.Port = 5432

	cfg.AI.Temperature = 2.5
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on invalid AI temperature 2.5")
	}
	cfg.AI.Temperature = 0.7

	cfg.Telemetry.SampleRate = 1.5
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on sample rate > 1.0")
	}
}
