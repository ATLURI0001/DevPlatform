package security

import "testing"

func TestBuildQueueRotary(t *testing.T) {
	r := NewBuildQueueRotary(2)
	r.Record(2)
	r.Record(4)
	r.Record(6)
	if got := r.Avg(); got != 5 {
		t.Fatalf("expected 5, got %v", got)
	}
}
