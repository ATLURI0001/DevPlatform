package models

import (
	"testing"
	"time"
)

func TestRolloutCircuitBreaker_ConsecutiveCrashLoopTrip(t *testing.T) {
	cfg := DefaultRolloutCircuitBreakerConfig()
	cfg.MaxCrashLoops = 3
	breaker, err := NewRolloutCircuitBreaker(cfg)
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	now := time.Now()

	// Pod 1 crash
	st1, err := breaker.RecordPodEvent(PodEvent{
		PodName:   "app-canary-1",
		IsError:   true,
		Reason:    ReasonCrashLoopBackOff,
		Timestamp: now,
	})
	if err != nil || st1 != BreakerMonitoring {
		t.Fatalf("expected MONITORING after 1 crash, got %s: %v", st1, err)
	}

	// Pod 2 crash
	st2, err := breaker.RecordPodEvent(PodEvent{
		PodName:   "app-canary-2",
		IsError:   true,
		Reason:    ReasonOOMKilled,
		Timestamp: now.Add(time.Second),
	})
	if err != nil || st2 != BreakerMonitoring {
		t.Fatalf("expected MONITORING after 2 crashes, got %s: %v", st2, err)
	}

	// Pod 3 crash -> should TRIP
	st3, err := breaker.RecordPodEvent(PodEvent{
		PodName:   "app-canary-3",
		IsError:   true,
		Reason:    ReasonCrashLoopBackOff,
		Timestamp: now.Add(2 * time.Second),
	})
	if err != nil || st3 != BreakerTripped {
		t.Fatalf("expected TRIPPED after 3 crashes, got %s: %v", st3, err)
	}

	snap := breaker.Status()
	if snap.State != BreakerTripped || snap.ConsecutiveCrashLoops != 3 {
		t.Errorf("unexpected status: %+v", snap)
	}

	// Further event should return ErrBreakerAlreadyTripped
	_, err = breaker.RecordPodEvent(PodEvent{PodName: "app-canary-4", Timestamp: now.Add(3 * time.Second)})
	if err != ErrBreakerAlreadyTripped {
		t.Errorf("expected ErrBreakerAlreadyTripped, got: %v", err)
	}

	// Reset should restore MONITORING
	breaker.Reset()
	snapAfter := breaker.Status()
	if snapAfter.State != BreakerMonitoring || snapAfter.ConsecutiveCrashLoops != 0 {
		t.Errorf("expected clean state after reset, got: %+v", snapAfter)
	}
}

func TestRolloutCircuitBreaker_FailureRateThresholdTrip(t *testing.T) {
	cfg := DefaultRolloutCircuitBreakerConfig()
	cfg.MaxCrashLoops = 10 // High so crash loop doesn't trip first
	cfg.FailureRateThresholdPercent = 30.0
	cfg.MinimumPodEventsForTrip = 5

	breaker, err := NewRolloutCircuitBreaker(cfg)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}

	now := time.Now()

	// 3 successes, 2 errors (40% error rate >= 30%)
	breaker.RecordPodEvent(PodEvent{PodName: "pod-1", IsError: false, Timestamp: now})
	breaker.RecordPodEvent(PodEvent{PodName: "pod-2", IsError: false, Timestamp: now.Add(time.Second)})
	breaker.RecordPodEvent(PodEvent{PodName: "pod-3", IsError: false, Timestamp: now.Add(2 * time.Second)})
	breaker.RecordPodEvent(PodEvent{PodName: "pod-4", IsError: true, Reason: ReasonProbeTimeout, Timestamp: now.Add(3 * time.Second)})
	st, err := breaker.RecordPodEvent(PodEvent{PodName: "pod-5", IsError: true, Reason: ReasonProbeTimeout, Timestamp: now.Add(4 * time.Second)})

	if err != nil || st != BreakerTripped {
		t.Fatalf("expected TRIPPED from 40%% failure rate, got %s: %v", st, err)
	}
}

func TestRolloutCircuitBreaker_SuccessResetsCrashLoops(t *testing.T) {
	cfg := DefaultRolloutCircuitBreakerConfig()
	cfg.MaxCrashLoops = 3
	breaker, _ := NewRolloutCircuitBreaker(cfg)

	now := time.Now()
	breaker.RecordPodEvent(PodEvent{PodName: "p1", IsError: true, Reason: ReasonCrashLoopBackOff, Timestamp: now})
	breaker.RecordPodEvent(PodEvent{PodName: "p2", IsError: true, Reason: ReasonCrashLoopBackOff, Timestamp: now.Add(time.Second)})

	// Healthy event arrives -> should reset consecutive crash loops
	breaker.RecordPodEvent(PodEvent{PodName: "p3", IsError: false, Timestamp: now.Add(2 * time.Second)})

	snap := breaker.Status()
	if snap.ConsecutiveCrashLoops != 0 {
		t.Errorf("expected consecutive crash loops reset to 0, got %d", snap.ConsecutiveCrashLoops)
	}
}
