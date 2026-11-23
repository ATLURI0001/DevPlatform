package models

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// BreakerState defines the active operational state of the rollout circuit breaker.
type BreakerState string

const (
	BreakerMonitoring BreakerState = "MONITORING"
	BreakerPaused     BreakerState = "PAUSED"
	BreakerTripped    BreakerState = "TRIPPED" // Rollback mandated
)

var (
	ErrBreakerAlreadyTripped = errors.New("circuit breaker is TRIPPED: automated rollback in progress")
	ErrInvalidBreakerConfig  = errors.New("invalid circuit breaker configuration parameters")
)

// PodFailureReason describes why a deployment pod failed.
type PodFailureReason string

const (
	ReasonCrashLoopBackOff PodFailureReason = "CrashLoopBackOff"
	ReasonOOMKilled        PodFailureReason = "OOMKilled"
	ReasonProbeTimeout     PodFailureReason = "ReadinessProbeTimeout"
	ReasonImagePullBackOff PodFailureReason = "ImagePullBackOff"
)

// PodEvent captures an observed event from the container runtime.
type PodEvent struct {
	PodName   string           `json:"pod_name"`
	IsError   bool             `json:"is_error"`
	Reason    PodFailureReason `json:"reason,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

// RolloutCircuitBreakerConfig defines threshold triggers for automatic rollback.
type RolloutCircuitBreakerConfig struct {
	MaxCrashLoops               int           // Consecutive crash loops to trip
	FailureRateThresholdPercent float64       // e.g. 25.0%
	ObservationWindowDuration   time.Duration // Time window to accumulate metrics
	MinimumPodEventsForTrip     int           // Minimum events before triggering rate trip
}

// DefaultRolloutCircuitBreakerConfig returns resilient production parameters.
func DefaultRolloutCircuitBreakerConfig() RolloutCircuitBreakerConfig {
	return RolloutCircuitBreakerConfig{
		MaxCrashLoops:               3,
		FailureRateThresholdPercent: 25.0,
		ObservationWindowDuration:   2 * time.Minute,
		MinimumPodEventsForTrip:     5,
	}
}

// BreakerStatusSnapshot provides a real-time status dump of the breaker.
type BreakerStatusSnapshot struct {
	State               BreakerState     `json:"state"`
	TripReason          string           `json:"trip_reason,omitempty"`
	ConsecutiveCrashLoops int            `json:"consecutive_crash_loops"`
	RecentFailureRate   float64          `json:"recent_failure_rate"`
	TotalEvents         int              `json:"total_events"`
	LastEventTime       time.Time        `json:"last_event_time"`
}

// RolloutCircuitBreaker prevents deployment thrashing and triggers automated rollback.
type RolloutCircuitBreaker struct {
	mu                    sync.RWMutex
	cfg                   RolloutCircuitBreakerConfig
	state                 BreakerState
	tripReason            string
	consecutiveCrashLoops int
	events                []PodEvent
}

// NewRolloutCircuitBreaker constructs a new breaker with validated configuration.
func NewRolloutCircuitBreaker(cfg RolloutCircuitBreakerConfig) (*RolloutCircuitBreaker, error) {
	if cfg.MaxCrashLoops <= 0 {
		cfg.MaxCrashLoops = 3
	}
	if cfg.FailureRateThresholdPercent <= 0 || cfg.FailureRateThresholdPercent > 100 {
		return nil, ErrInvalidBreakerConfig
	}
	if cfg.ObservationWindowDuration <= 0 {
		cfg.ObservationWindowDuration = 2 * time.Minute
	}
	if cfg.MinimumPodEventsForTrip <= 0 {
		cfg.MinimumPodEventsForTrip = 5
	}

	return &RolloutCircuitBreaker{
		cfg:    cfg,
		state:  BreakerMonitoring,
		events: make([]PodEvent, 0, 100),
	}, nil
}

// RecordPodEvent ingests a pod lifecycle observation and updates breaker state.
func (b *RolloutCircuitBreaker) RecordPodEvent(evt PodEvent) (BreakerState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == BreakerTripped {
		return BreakerTripped, ErrBreakerAlreadyTripped
	}

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}

	b.events = append(b.events, evt)
	b.evictExpiredEventsLocked(evt.Timestamp)

	// Check crash loop threshold
	if evt.IsError && (evt.Reason == ReasonCrashLoopBackOff || evt.Reason == ReasonOOMKilled) {
		b.consecutiveCrashLoops++
		if b.consecutiveCrashLoops >= b.cfg.MaxCrashLoops {
			b.state = BreakerTripped
			b.tripReason = fmt.Sprintf("tripped by %d consecutive pod crash events (last reason: %s)",
				b.consecutiveCrashLoops, evt.Reason)
			return BreakerTripped, nil
		}
	} else if !evt.IsError {
		// Successful pod readiness resets consecutive crash loop counter
		b.consecutiveCrashLoops = 0
	}

	// Check sliding window failure rate
	if len(b.events) >= b.cfg.MinimumPodEventsForTrip {
		errorCount := 0
		for _, e := range b.events {
			if e.IsError {
				errorCount++
			}
		}
		failRate := (float64(errorCount) / float64(len(b.events))) * 100.0
		if failRate >= b.cfg.FailureRateThresholdPercent {
			b.state = BreakerTripped
			b.tripReason = fmt.Sprintf("tripped by failure rate %.1f%% exceeding threshold %.1f%% over %d events",
				failRate, b.cfg.FailureRateThresholdPercent, len(b.events))
			return BreakerTripped, nil
		}
	}

	return b.state, nil
}

func (b *RolloutCircuitBreaker) evictExpiredEventsLocked(now time.Time) {
	cutoff := now.Add(-b.cfg.ObservationWindowDuration)
	validIdx := 0
	for i, e := range b.events {
		if e.Timestamp.After(cutoff) {
			validIdx = i
			break
		}
	}
	if validIdx > 0 {
		b.events = b.events[validIdx:]
	}
}

// Status returns a point-in-time telemetry snapshot of the breaker.
func (b *RolloutCircuitBreaker) Status() BreakerStatusSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var failRate float64
	var lastTime time.Time
	if len(b.events) > 0 {
		lastTime = b.events[len(b.events)-1].Timestamp
		errCount := 0
		for _, e := range b.events {
			if e.IsError {
				errCount++
			}
		}
		failRate = (float64(errCount) / float64(len(b.events))) * 100.0
	}

	return BreakerStatusSnapshot{
		State:                 b.state,
		TripReason:            b.tripReason,
		ConsecutiveCrashLoops: b.consecutiveCrashLoops,
		RecentFailureRate:     failRate,
		TotalEvents:           len(b.events),
		LastEventTime:         lastTime,
	}
}

// Reset resets the circuit breaker back to MONITORING after human intervention or rollback.
func (b *RolloutCircuitBreaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state = BreakerMonitoring
	b.tripReason = ""
	b.consecutiveCrashLoops = 0
	b.events = b.events[:0]
}
