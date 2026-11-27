package security

import (
	"testing"
	"time"
)

func TestPolicyEvalProfiler(t *testing.T) {
	p := NewPolicyEvalProfiler()
	p.Record(3 * time.Millisecond)
	if p.Count() != 1 {
		t.Fatalf("expected 1 sample, got %d", p.Count())
	}
}
