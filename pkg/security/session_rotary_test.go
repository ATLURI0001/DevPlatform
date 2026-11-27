package security

import "testing"

func TestSessionRotary(t *testing.T) {
	r := NewSessionRotary(2)
	r.Record("sid-1")
	r.Record("sid-2")
	r.Record("sid-3")
	if r.Len() != 2 {
		t.Fatalf("expected 2, got %d", r.Len())
	}
}
