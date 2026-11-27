package security

import "testing"

func TestAssertSafeEnvValue(t *testing.T) {
	if err := AssertSafeEnvValue("production"); err != nil {
		t.Fatalf("expected clean value, got %v", err)
	}
	if err := AssertSafeEnvValue("prod\nEVIL=1"); err == nil {
		t.Fatal("expected env injection sentinel violation")
	}
}
