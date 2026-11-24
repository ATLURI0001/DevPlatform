package models

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeployCircuitBreaker_NominalExecution(t *testing.T) {
	cfg := DefaultDeployCircuitBreakerConfig()
	var fallbackInvoked bool
	fallback := func(ctx context.Context, action string, lastErr error) error {
		fallbackInvoked = true
		return nil
	}

	cb := NewDeployCircuitBreaker(cfg, fallback)
	if cb.State() != DeployStateClosed {
		t.Fatalf("expected state CLOSED, got %s", cb.State())
	}

	err := cb.ExecuteWithRetry(context.Background(), "apply_k8s_manifest", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fallbackInvoked {
		t.Fatalf("fallback should not be invoked on success")
	}

	metrics := cb.GetMetrics()
	if metrics.TotalExecutions != 1 || metrics.ConsecutiveFailures != 0 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestDeployCircuitBreaker_RetriesAndThresholdTrip(t *testing.T) {
	cfg := DeployCircuitBreakerConfig{
		MaxConsecutiveFailures: 2,
		RecoveryTimeout:        50 * time.Millisecond,
		BaseBackoff:            2 * time.Millisecond,
		MaxBackoff:             10 * time.Millisecond,
		MaxRetries:             2,
	}

	var fallbackCount int
	fallback := func(ctx context.Context, action string, lastErr error) error {
		fallbackCount++
		return errors.New("rollback initiated")
	}

	cb := NewDeployCircuitBreaker(cfg, fallback)

	failingTask := func(ctx context.Context) error {
		return errors.New("container registry 504 gateway timeout")
	}

	// Execution 1: fails after 2 retries (total 3 attempts). consecutive = 1. CLOSED.
	err := cb.ExecuteWithRetry(context.Background(), "pull_image", failingTask)
	if err == nil {
		t.Fatalf("expected error from failingTask")
	}
	if cb.State() != DeployStateClosed {
		t.Fatalf("expected CLOSED after 1 failure, got %s", cb.State())
	}
	if fallbackCount != 1 {
		t.Fatalf("expected fallbackCount 1, got %d", fallbackCount)
	}

	// Execution 2: fails again, hits threshold 2 -> TRIPS to OPEN!
	err = cb.ExecuteWithRetry(context.Background(), "pull_image", failingTask)
	if err == nil {
		t.Fatalf("expected error on trip")
	}
	if cb.State() != DeployStateOpen {
		t.Fatalf("expected OPEN state, got %s", cb.State())
	}

	// Execution 3: in OPEN state, fails fast without running task, immediately invoking fallback
	var taskRanWhileOpen bool
	err = cb.ExecuteWithRetry(context.Background(), "pull_image", func(ctx context.Context) error {
		taskRanWhileOpen = true
		return nil
	})
	if taskRanWhileOpen {
		t.Fatalf("task must not run while circuit is OPEN")
	}

	// 4. Recovery timeout elapses -> transitions to HALF_OPEN
	time.Sleep(60 * time.Millisecond)
	if cb.State() != DeployStateHalfOpen {
		t.Fatalf("expected state HALF_OPEN, got %s", cb.State())
	}

	// 5. Successful probe recovers state to CLOSED
	err = cb.ExecuteWithRetry(context.Background(), "pull_image", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected probe error: %v", err)
	}
	if cb.State() != DeployStateClosed {
		t.Fatalf("expected state CLOSED after successful probe, got %s", cb.State())
	}
}

func TestDeployCircuitBreaker_CalculateBackoffBounds(t *testing.T) {
	cfg := DeployCircuitBreakerConfig{
		BaseBackoff: 10 * time.Millisecond,
		MaxBackoff:  100 * time.Millisecond,
	}
	cb := NewDeployCircuitBreaker(cfg, nil)

	for attempt := 0; attempt < 10; attempt++ {
		backoff := cb.CalculateBackoff(attempt)
		if backoff < 0 {
			t.Fatalf("backoff must be non-negative: %v", backoff)
		}
		if backoff > cfg.MaxBackoff {
			t.Fatalf("backoff exceeds max backoff: %v > %v", backoff, cfg.MaxBackoff)
		}
	}
}
