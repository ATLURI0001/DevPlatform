package security

import "testing"

func TestBenchmarkEnvLookup(t *testing.T) {
	hits, _ := BenchmarkEnvLookup(map[string]string{"ENV": "prod"}, []string{"ENV", "MISSING"}, 50)
	if hits != 50 {
		t.Fatalf("expected 50 hits, got %d", hits)
	}
}
