package security

import "testing"

func TestValidateAPIKey(t *testing.T) {
	if err := ValidateAPIKey("abcdefghijklmnop"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateAPIKey("short"); err == nil {
		t.Fatal("expected error for short key")
	}
}
