package security

import "testing"

func TestBenchmarkConfigScan(t *testing.T) {
	c, d := BenchmarkConfigScan([]string{"PORT=8080", "HOST=localhost"}, 100)
	if c != 200 || d < 0 {
		t.Fatalf("unexpected result: %d, %v", c, d)
	}
}
