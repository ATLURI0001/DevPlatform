package security

import (
	"strings"
	"time"
)

// BenchmarkConfigScan measures key=value config line scanning throughput.
func BenchmarkConfigScan(lines []string, iterations int) (count int, elapsed time.Duration) {
	start := time.Now()
	for i := 0; i < iterations; i++ {
		for _, l := range lines {
			if strings.Contains(l, "=") {
				count++
			}
		}
	}
	return count, time.Since(start)
}
