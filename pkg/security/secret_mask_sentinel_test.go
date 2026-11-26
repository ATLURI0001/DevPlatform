package security

import "testing"

func TestAssertNoRawSecret(t *testing.T) {
	if err := AssertNoRawSecret("build succeeded for commit abc"); err != nil {
		t.Fatalf("expected clean log, got %v", err)
	}
	if err := AssertNoRawSecret("token=ghp_12345"); err == nil {
		t.Fatal("expected unmasked secret sentinel violation")
	}
}
