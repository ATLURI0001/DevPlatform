package security

import "testing"

func TestSecurityEventRotary(t *testing.T) {
	r := NewSecurityEventRotary(2)
	r.Push("login")
	r.Push("logout")
	r.Push("token_refresh")
	if r.Count() != 2 {
		t.Fatalf("expected 2, got %d", r.Count())
	}
}
