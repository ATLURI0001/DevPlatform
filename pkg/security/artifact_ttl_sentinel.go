package security

import (
	"fmt"
	"time"
)

// ArtifactTTLSentinel monitors artifact expiry TTL violations.
type ArtifactTTLSentinel struct {
	MaxTTL time.Duration
}

// ArtifactRecord represents a build artifact with its creation time and configured TTL.
type ArtifactRecord struct {
	ID        string
	CreatedAt time.Time
	TTL       time.Duration
}

// Validate returns an error if the artifact's TTL exceeds MaxTTL or if it has already expired.
func (s *ArtifactTTLSentinel) Validate(a ArtifactRecord) error {
	if a.TTL > s.MaxTTL {
		return fmt.Errorf("sentinel violation: artifact %s TTL %v exceeds max %v", a.ID, a.TTL, s.MaxTTL)
	}
	if time.Since(a.CreatedAt) > a.TTL {
		return fmt.Errorf("sentinel violation: artifact %s has expired (age %v > TTL %v)", a.ID, time.Since(a.CreatedAt).Truncate(time.Second), a.TTL)
	}
	return nil
}
