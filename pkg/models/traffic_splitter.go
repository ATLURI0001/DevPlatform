package models

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidWeight        = errors.New("canary weight must be between 0 and 100")
	ErrCannotAdvanceSplit   = errors.New("cannot advance weight while in rolled back state without reset")
	ErrInvalidConfiguration = errors.New("deployment ID and salt cannot be empty")
)

// SplitPhase defines the lifecycle state of a canary traffic deployment.
type SplitPhase string

const (
	SplitIdle       SplitPhase = "SPLIT_IDLE"
	SplitActive     SplitPhase = "SPLIT_ACTIVE"
	SplitRolledBack SplitPhase = "SPLIT_ROLLED_BACK"
	SplitCompleted  SplitPhase = "SPLIT_COMPLETED"
)

// TargetRoute specifies the resolved upstream routing target.
type TargetRoute string

const (
	RouteStable TargetRoute = "STABLE"
	RouteCanary TargetRoute = "CANARY"
)

// CanarySplitConfig specifies the parameters for traffic splitting and auto-rollback.
type CanarySplitConfig struct {
	DeploymentID            string  `json:"deployment_id"`
	Salt                    string  `json:"salt"`
	CanaryWeight            int     `json:"canary_weight"`
	MaxErrorRate            float64 `json:"max_error_rate"`             // e.g. 0.05 = 5%
	MinEvaluationRequests   int     `json:"min_evaluation_requests"`    // minimum samples before rollback eval
}

// RouteMetrics tracks operational telemetry for a given route.
type RouteMetrics struct {
	TotalRequests uint64  `json:"total_requests"`
	Errors        uint64  `json:"errors"`
	ErrorRate     float64 `json:"error_rate"`
}

// SplitterStats captures a point-in-time diagnostic summary.
type SplitterStats struct {
	Phase          SplitPhase   `json:"phase"`
	CanaryWeight   int          `json:"canary_weight"`
	StableWeight   int          `json:"stable_weight"`
	RollbackReason string       `json:"rollback_reason,omitempty"`
	StableMetrics  RouteMetrics `json:"stable_metrics"`
	CanaryMetrics  RouteMetrics `json:"canary_metrics"`
}

// TrafficSplitter governs dynamic canary distribution, deterministic hashing, and auto-rollback.
type TrafficSplitter struct {
	mu             sync.RWMutex
	deploymentID   string
	salt           string
	phase          SplitPhase
	canaryWeight   int
	maxErrorRate   float64
	minEvalReqs    int
	rollbackReason string

	stableRequests uint64
	stableErrors   uint64
	canaryRequests uint64
	canaryErrors   uint64
}

// NewTrafficSplitter constructs a validated canary traffic splitter.
func NewTrafficSplitter(cfg CanarySplitConfig) (*TrafficSplitter, error) {
	if cfg.DeploymentID == "" || cfg.Salt == "" {
		return nil, ErrInvalidConfiguration
	}
	if cfg.CanaryWeight < 0 || cfg.CanaryWeight > 100 {
		return nil, ErrInvalidWeight
	}

	maxErr := cfg.MaxErrorRate
	if maxErr <= 0.0 || maxErr > 1.0 {
		maxErr = 0.05
	}

	minReqs := cfg.MinEvaluationRequests
	if minReqs <= 0 {
		minReqs = 20
	}

	phase := SplitIdle
	if cfg.CanaryWeight == 100 {
		phase = SplitCompleted
	} else if cfg.CanaryWeight > 0 {
		phase = SplitActive
	}

	return &TrafficSplitter{
		deploymentID: cfg.DeploymentID,
		salt:         cfg.Salt,
		phase:        phase,
		canaryWeight: cfg.CanaryWeight,
		maxErrorRate: maxErr,
		minEvalReqs:  minReqs,
	}, nil
}

// RouteKey deterministically routes a given key (user ID, session ID) to STABLE or CANARY.
func (s *TrafficSplitter) RouteKey(routingKey string) TargetRoute {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.phase == SplitRolledBack || s.phase == SplitIdle || s.canaryWeight <= 0 {
		return RouteStable
	}
	if s.phase == SplitCompleted || s.canaryWeight >= 100 {
		return RouteCanary
	}

	// Deterministic SHA-256 hash distribution in [0, 99]
	hasher := sha256.New()
	hasher.Write([]byte(s.deploymentID))
	hasher.Write([]byte(":"))
	hasher.Write([]byte(s.salt))
	hasher.Write([]byte(":"))
	hasher.Write([]byte(routingKey))
	digest := hasher.Sum(nil)

	bucket := int(binary.BigEndian.Uint32(digest[:4]) % 100)

	if bucket < s.canaryWeight {
		return RouteCanary
	}
	return RouteStable
}

// RecordOutcome logs a request result and evaluates canary health for automatic rollback.
func (s *TrafficSplitter) RecordOutcome(route TargetRoute, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if route == RouteStable {
		s.stableRequests++
		if !success {
			s.stableErrors++
		}
		return
	}

	// Canary route
	s.canaryRequests++
	if !success {
		s.canaryErrors++
	}

	// Check for auto-rollback condition
	if s.phase == SplitActive && s.canaryRequests >= uint64(s.minEvalReqs) {
		errRate := float64(s.canaryErrors) / float64(s.canaryRequests)
		if errRate > s.maxErrorRate {
			s.phase = SplitRolledBack
			s.canaryWeight = 0
			s.rollbackReason = fmt.Sprintf("automatic rollback: canary error rate %.2f%% exceeded threshold %.2f%% (%d errors / %d requests)",
				errRate*100, s.maxErrorRate*100, s.canaryErrors, s.canaryRequests)
		}
	}
}

// SetCanaryWeight dynamically updates the canary weight allocation.
func (s *TrafficSplitter) SetCanaryWeight(weight int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if weight < 0 || weight > 100 {
		return ErrInvalidWeight
	}
	if s.phase == SplitRolledBack {
		return ErrCannotAdvanceSplit
	}

	s.canaryWeight = weight
	if weight == 100 {
		s.phase = SplitCompleted
	} else if weight > 0 {
		s.phase = SplitActive
	} else {
		s.phase = SplitIdle
	}

	return nil
}

// ManualRollback forcibly shifts all traffic to stable and marks phase as rolled back.
func (s *TrafficSplitter) ManualRollback(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.phase = SplitRolledBack
	s.canaryWeight = 0
	if reason == "" {
		reason = "manual operator rollback triggered"
	}
	s.rollbackReason = reason
}

// Reset clears metrics and resets phase to idle.
func (s *TrafficSplitter) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.phase = SplitIdle
	s.canaryWeight = 0
	s.rollbackReason = ""
	s.stableRequests = 0
	s.stableErrors = 0
	s.canaryRequests = 0
	s.canaryErrors = 0
}

// GetStats returns the current state and metrics snapshot.
func (s *TrafficSplitter) GetStats() SplitterStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stableRate float64
	if s.stableRequests > 0 {
		stableRate = float64(s.stableErrors) / float64(s.stableRequests)
	}

	var canaryRate float64
	if s.canaryRequests > 0 {
		canaryRate = float64(s.canaryErrors) / float64(s.canaryRequests)
	}

	return SplitterStats{
		Phase:          s.phase,
		CanaryWeight:   s.canaryWeight,
		StableWeight:   100 - s.canaryWeight,
		RollbackReason: s.rollbackReason,
		StableMetrics: RouteMetrics{
			TotalRequests: s.stableRequests,
			Errors:        s.stableErrors,
			ErrorRate:     stableRate,
		},
		CanaryMetrics: RouteMetrics{
			TotalRequests: s.canaryRequests,
			Errors:        s.canaryErrors,
			ErrorRate:     canaryRate,
		},
	}
}
