package models

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	ErrDeployCircuitOpen = errors.New("deploy worker circuit breaker is OPEN: fast-failing task execution")
	ErrMaxRetriesExceeded = errors.New("maximum task retry attempts exceeded")
)

type DeployCircuitState string

const (
	DeployStateClosed   DeployCircuitState = "CLOSED"
	DeployStateOpen     DeployCircuitState = "OPEN"
	DeployStateHalfOpen DeployCircuitState = "HALF_OPEN"
)

type DeployTaskFunc func(ctx context.Context) error
type DeployFallbackFunc func(ctx context.Context, action string, lastErr error) error

type DeployCircuitBreakerConfig struct {
	MaxConsecutiveFailures int           `json:"max_consecutive_failures"`
	RecoveryTimeout        time.Duration `json:"recovery_timeout"`
	BaseBackoff            time.Duration `json:"base_backoff"`
	MaxBackoff             time.Duration `json:"max_backoff"`
	MaxRetries             int           `json:"max_retries"`
}

func DefaultDeployCircuitBreakerConfig() DeployCircuitBreakerConfig {
	return DeployCircuitBreakerConfig{
		MaxConsecutiveFailures: 3,
		RecoveryTimeout:        10 * time.Second,
		BaseBackoff:            50 * time.Millisecond,
		MaxBackoff:             2 * time.Second,
		MaxRetries:             3,
	}
}

type DeployBreakerMetrics struct {
	State               DeployCircuitState `json:"state"`
	ConsecutiveFailures int                `json:"consecutive_failures"`
	TotalExecutions     uint64             `json:"total_executions"`
	FallbackInvocations uint64             `json:"fallback_invocations"`
	RetriesAttempted    uint64             `json:"retries_attempted"`
	StateTransitions    uint64             `json:"state_transitions"`
	LastFailureReason   string             `json:"last_failure_reason,omitempty"`
}

type DeployCircuitBreaker struct {
	mu                  sync.RWMutex
	config              DeployCircuitBreakerConfig
	fallbackHandler     DeployFallbackFunc
	state               DeployCircuitState
	consecutiveFailures int
	openedAt            time.Time
	lastFailureReason   string

	// Metrics
	totalExecutions     uint64
	fallbackInvocations uint64
	retriesAttempted    uint64
	stateTransitions    uint64
}

func NewDeployCircuitBreaker(cfg DeployCircuitBreakerConfig, fallback DeployFallbackFunc) *DeployCircuitBreaker {
	if cfg.MaxConsecutiveFailures <= 0 {
		cfg.MaxConsecutiveFailures = 3
	}
	if cfg.RecoveryTimeout <= 0 {
		cfg.RecoveryTimeout = 10 * time.Second
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 50 * time.Millisecond
	}
	if cfg.MaxBackoff < cfg.BaseBackoff {
		cfg.MaxBackoff = cfg.BaseBackoff * 10
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 3
	}

	return &DeployCircuitBreaker{
		config:          cfg,
		fallbackHandler: fallback,
		state:           DeployStateClosed,
	}
}

func (cb *DeployCircuitBreaker) State() DeployCircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.evalStateLocked()
	return cb.state
}

func (cb *DeployCircuitBreaker) evalStateLocked() {
	if cb.state == DeployStateOpen && time.Since(cb.openedAt) >= cb.config.RecoveryTimeout {
		cb.state = DeployStateHalfOpen
		cb.stateTransitions++
	}
}

func (cb *DeployCircuitBreaker) tripLocked(reason string) {
	if cb.state != DeployStateOpen {
		cb.state = DeployStateOpen
		cb.openedAt = time.Now()
		cb.lastFailureReason = reason
		cb.stateTransitions++
	}
}

func (cb *DeployCircuitBreaker) resetLocked() {
	if cb.state != DeployStateClosed {
		cb.state = DeployStateClosed
		cb.consecutiveFailures = 0
		cb.stateTransitions++
	}
}

// CalculateBackoff computes exponential backoff with full jitter.
func (cb *DeployCircuitBreaker) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return cb.config.BaseBackoff
	}
	multiplier := math.Pow(2, float64(attempt))
	maxDelay := float64(cb.config.BaseBackoff) * multiplier
	if maxDelay > float64(cb.config.MaxBackoff) {
		maxDelay = float64(cb.config.MaxBackoff)
	}
	// Full jitter: uniformly random between 0 and maxDelay
	jitter := rand.Float64() * maxDelay
	return time.Duration(jitter)
}

func (cb *DeployCircuitBreaker) ExecuteWithRetry(ctx context.Context, action string, fn DeployTaskFunc) error {
	cb.mu.Lock()
	cb.totalExecutions++
	cb.evalStateLocked()

	if cb.state == DeployStateOpen {
		cb.fallbackInvocations++
		lastReason := cb.lastFailureReason
		fallback := cb.fallbackHandler
		cb.mu.Unlock()

		if fallback != nil {
			return fallback(ctx, action, fmt.Errorf("%w: %s", ErrDeployCircuitOpen, lastReason))
		}
		return fmt.Errorf("%w: %s", ErrDeployCircuitOpen, lastReason)
	}
	isHalfOpen := cb.state == DeployStateHalfOpen
	cb.mu.Unlock()

	var lastErr error
	maxAttempts := cb.config.MaxRetries
	if isHalfOpen {
		maxAttempts = 0 // In half-open, only 1 probe trial
	}

	for attempt := 0; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			// Success
			cb.mu.Lock()
			if cb.state == DeployStateHalfOpen {
				cb.resetLocked()
			} else if cb.state == DeployStateClosed {
				cb.consecutiveFailures = 0
			}
			cb.mu.Unlock()
			return nil
		}

		// Failed attempt
		if attempt < maxAttempts {
			cb.mu.Lock()
			cb.retriesAttempted++
			cb.mu.Unlock()

			backoff := cb.CalculateBackoff(attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// All attempts exhausted
	cb.mu.Lock()
	cb.consecutiveFailures++
	cb.lastFailureReason = lastErr.Error()

	if isHalfOpen || cb.consecutiveFailures >= cb.config.MaxConsecutiveFailures {
		cb.tripLocked(lastErr.Error())
	}

	fallback := cb.fallbackHandler
	cb.mu.Unlock()

	if fallback != nil {
		cb.mu.Lock()
		cb.fallbackInvocations++
		cb.mu.Unlock()
		return fallback(ctx, action, lastErr)
	}

	return lastErr
}

func (cb *DeployCircuitBreaker) GetMetrics() DeployBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return DeployBreakerMetrics{
		State:               cb.state,
		ConsecutiveFailures: cb.consecutiveFailures,
		TotalExecutions:     cb.totalExecutions,
		FallbackInvocations: cb.fallbackInvocations,
		RetriesAttempted:    cb.retriesAttempted,
		StateTransitions:    cb.stateTransitions,
		LastFailureReason:   cb.lastFailureReason,
	}
}
