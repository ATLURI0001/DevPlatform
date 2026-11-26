package security

import (
	"errors"
	"testing"
	"time"
)

func TestRegistryPushCircuitBreaker(t *testing.T) {
	cb := NewRegistryPushCircuitBreaker(2, 5*time.Second)
	fail := func() error { return errors.New("registry 502") }
	_ = cb.Call(fail)
	_ = cb.Call(fail)
	if err := cb.Call(func() error { return nil }); err != ErrRegistryPushOpen {
		t.Fatalf("expected ErrRegistryPushOpen, got %v", err)
	}
}
