package security

import (
	"errors"
	"time"
)

var ErrRegistryPushOpen = errors.New("registry push circuit breaker open")

// RegistryPushCircuitBreaker protects container registry pushes against outages.
type RegistryPushCircuitBreaker struct {
	threshold int
	cooldown  time.Duration
	failures  int
	openUntil time.Time
}

func NewRegistryPushCircuitBreaker(threshold int, cooldown time.Duration) *RegistryPushCircuitBreaker {
	return &RegistryPushCircuitBreaker{threshold: threshold, cooldown: cooldown}
}

func (cb *RegistryPushCircuitBreaker) Call(fn func() error) error {
	if time.Now().Before(cb.openUntil) {
		return ErrRegistryPushOpen
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
