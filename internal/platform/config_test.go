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
	cfg.Telemetry.SampleRate = 0.1

	cfg.Server.Addr = "localhost"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on server addr without colon")
	}
	cfg.Server.Addr = ":8080"

	cfg.Redis.URL = "http://localhost:6379"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on invalid redis scheme")
	}
	cfg.Redis.URL = "redis://localhost:6379"

	cfg.NATS.URL = "invalid-url"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on invalid nats url")
	}
	cfg.NATS.URL = "nats://localhost:4222"

	cfg.AI.BaseURL = "ftp://api.openai.com/v1"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on unsupported ai base url scheme")
	}
	cfg.AI.BaseURL = "https://api.openai.com/v1"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error on valid https ai base url: %v", err)
	}
	cfg.AI.BaseURL = ""

	cfg.Telemetry.OTLPEndpoint = "   "
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on whitespace-only telemetry otlp endpoint")
	}
	cfg.Telemetry.OTLPEndpoint = "ftp://collector:4317"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on unsupported telemetry otlp scheme")
	}
	cfg.Telemetry.OTLPEndpoint = "grpc://collector:4317"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error on valid grpc otlp endpoint: %v", err)
	}
	cfg.Telemetry.OTLPEndpoint = ""

	cfg.Environment = "production"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error in production when jwt secret is default dev secret")
	}
	cfg.Auth.JWTSecret = "short"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error in production when jwt secret is under 16 characters")
	}
	cfg.Auth.JWTSecret = "super-secret-production-token-signing-key-12345"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error with strong production jwt secret: %v", err)
	}
	cfg.Environment = "development"
	cfg.Auth.JWTSecret = "dev-secret-change-in-production"
}
