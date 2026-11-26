package models

import (
	"errors"
	"testing"
	"time"
)

func TestWebhookCircuitBreaker_TripAndRecover(t *testing.T) {
	cb := NewWebhookCircuitBreaker(2, 100*time.Millisecond)
	now := time.Now()
	ep := "https://hooks.example.com/ci"

	cb.RecordOutcome(ep, false, now)
	cb.RecordOutcome(ep, false, now)

	if err := cb.AllowDispatch(ep, now.Add(20*time.Millisecond)); !errors.Is(err, ErrWebhookCircuitOpen) {
		t.Fatalf("expected ErrWebhookCircuitOpen, got %v", err)
	}

	if err := cb.AllowDispatch(ep, now.Add(150*time.Millisecond)); err != nil {
		t.Fatalf("expected HALF_OPEN allow after cooldown, got %v", err)
	}
	cb.RecordOutcome(ep, true, now.Add(160*time.Millisecond))
	if err := cb.AllowDispatch(ep, now.Add(170*time.Millisecond)); err != nil {
		t.Fatalf("expected CLOSED allow after recovery, got %v", err)
	}
}
