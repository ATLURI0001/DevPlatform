package security

import (
	"strings"
	"time"
)

// BenchmarkLogGrep measures substring log filtering throughput.
func BenchmarkLogGrep(lines []string, needle string, iterations int) (hits int, elapsed time.Duration) {
	start := time.Now()
	for i := 0; i < iterations; i++ {
		for _, l := range lines {
			if strings.Contains(l, needle) {
				hits++
			}
		}
	}
	return hits, time.Since(start)
}
