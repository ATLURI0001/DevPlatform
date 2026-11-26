package security

import "testing"

func TestDeployDurationRotary(t *testing.T) {
	r := NewDeployDurationRotary(2)
	r.Record(10)
	r.Record(20)
	r.Record(40)
	if got := r.Avg(); got != 30 {
		t.Fatalf("expected 30, got %v", got)
	}
}
