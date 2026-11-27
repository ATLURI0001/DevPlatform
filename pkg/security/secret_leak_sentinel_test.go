package security

import "testing"

func TestSecretLeakSentinel(t *testing.T) {
	s := NewSecretLeakSentinel([]string{"sk-", "ghp_"})
	if !s.Scan("normal-payload") {
		t.Fatal("expected normal payload to pass")
	}
	if s.Scan("sk-12345") {
		t.Fatal("expected secret prefix to be flagged")
	}
	if s.Hits() != 1 {
		t.Fatalf("expected 1 hit, got %d", s.Hits())
	}
}
