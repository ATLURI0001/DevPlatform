package security

// SecretLeakSentinel detects forbidden secret prefixes in payloads.
type SecretLeakSentinel struct {
	prefixes []string
	hits     int
}

func NewSecretLeakSentinel(prefixes []string) *SecretLeakSentinel {
	return &SecretLeakSentinel{prefixes: prefixes}
}

func (s *SecretLeakSentinel) Scan(payload string) bool {
	for _, p := range s.prefixes {
		if len(payload) >= len(p) && payload[:len(p)] == p {
			s.hits++
			return false
		}
	}
	return true
}

func (s *SecretLeakSentinel) Hits() int {
	return s.hits
}
