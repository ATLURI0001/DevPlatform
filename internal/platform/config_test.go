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
