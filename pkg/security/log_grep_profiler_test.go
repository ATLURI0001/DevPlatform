package security

import "testing"

func TestBenchmarkLogGrep(t *testing.T) {
	hits, d := BenchmarkLogGrep([]string{"INFO ok", "ERROR fail"}, "ERROR", 100)
	if hits != 100 || d < 0 {
		t.Fatalf("unexpected grep result: %d, %v", hits, d)
	}
}
