package security

import "time"

// BenchmarkEnvLookup measures map lookup throughput for environment config keys.
func BenchmarkEnvLookup(env map[string]string, keys []string, iterations int) (hits int, elapsed time.Duration) {
	start := time.Now()
	for i := 0; i < iterations; i++ {
		for _, k := range keys {
			if _, ok := env[k]; ok {
				hits++
			}
		}
	}
	return hits, time.Since(start)
}
