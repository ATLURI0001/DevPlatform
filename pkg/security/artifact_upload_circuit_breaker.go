package security

import (
	"errors"
	"time"
)

var ErrArtifactUploadOpen = errors.New("artifact upload circuit breaker open")

// ArtifactUploadCircuitBreaker guards build artifact uploads against storage outages.
type ArtifactUploadCircuitBreaker struct {
	threshold int
	cooldown  time.Duration
	failures  int
	openUntil time.Time
}

func NewArtifactUploadCircuitBreaker(threshold int, cooldown time.Duration) *ArtifactUploadCircuitBreaker {
	return &ArtifactUploadCircuitBreaker{threshold: threshold, cooldown: cooldown}
}

func (cb *ArtifactUploadCircuitBreaker) Call(fn func() error) error {
	if time.Now().Before(cb.openUntil) {
		return ErrArtifactUploadOpen
	}
	if err := fn(); err != nil {
		cb.failures++
		if cb.failures >= cb.threshold {
			cb.openUntil = time.Now().Add(cb.cooldown)
			cb.failures = 0
		}
		return err
	}
	cb.failures = 0
	return nil
}
