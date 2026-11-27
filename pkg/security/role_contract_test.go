package security

import "testing"

func TestValidateRole(t *testing.T) {
	if err := ValidateRole("admin:write"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateRole("INVALID"); err == nil {
		t.Fatal("expected error for uppercase role")
	}
}
