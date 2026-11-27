package security

import "testing"

func TestValidateServicePort(t *testing.T) {
	if err := ValidateServicePort(8080); err != nil {
		t.Fatalf("expected valid port, got %v", err)
	}
	if err := ValidateServicePort(70000); err == nil {
		t.Fatal("expected port contract violation")
	}
}
