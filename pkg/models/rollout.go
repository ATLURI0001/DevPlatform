package models

import (
	"errors"
	"fmt"
	"time"
)

// RolloutPhase represents the lifecycle state of a canary deployment.
type RolloutPhase string

const (
	RolloutPhasePending   RolloutPhase = "PENDING"
	RolloutPhaseCanary    RolloutPhase = "CANARY"
	RolloutPhasePromoting RolloutPhase = "PROMOTING"
	RolloutPhaseStable    RolloutPhase = "STABLE"
	RolloutPhaseAborting  RolloutPhase = "ABORTING"
	RolloutPhaseAborted   RolloutPhase = "ABORTED"
)

var (
	ErrInvalidCanaryWeight    = errors.New("canary weight must be between 1 and 100 percent")
	ErrNonMonotonicSteps      = errors.New("canary rollout steps must have strictly increasing weights")
	ErrInvalidSlaThreshold    = errors.New("SLA thresholds must be positive finite values")
	ErrRolloutAlreadyFinished = errors.New("rollout is already in a terminal state (STABLE or ABORTED)")
)

// CanaryStep configures a single progressive traffic shifting phase.
type CanaryStep struct {
	WeightPercent          int           `json:"weight_percent"` // 1 - 100
	PauseDuration          time.Duration `json:"pause_duration"`
	MaxFailureRatePercent  float64       `json:"max_failure_rate_percent"` // e.g. 1.0%
	LatencyP99ThresholdMs  float64       `json:"latency_p99_threshold_ms"` // e.g. 200.0ms
}

// RolloutTelemetry captures real-time observation metrics for canary evaluation.
type RolloutTelemetry struct {
	TotalRequests   uint64  `json:"total_requests"`
	FailedRequests  uint64  `json:"failed_requests"`
	LatencyP99Ms    float64 `json:"latency_p99_ms"`
}

// CanaryPlan orchestrates progressive delivery across discrete traffic increments.
type CanaryPlan struct {
	DeploymentID string       `json:"deployment_id"`
	Steps        []CanaryStep `json:"steps"`
	CurrentStep  int          `json:"current_step"`
	Phase        RolloutPhase `json:"phase"`
	AbortReason  string       `json:"abort_reason,omitempty"`
}

// NewCanaryPlan constructs and validates a monotonic progressive canary plan.
func NewCanaryPlan(deploymentID string, steps []CanaryStep) (*CanaryPlan, error) {
	if deploymentID == "" {
		return nil, errors.New("deploymentID cannot be empty")
	}
	if len(steps) == 0 {
		return nil, errors.New("at least one canary step is required")
	}

	prevWeight := 0
	for i, step := range steps {
		if step.WeightPercent <= 0 || step.WeightPercent > 100 {
			return nil, fmt.Errorf("%w: step %d has weight %d", ErrInvalidCanaryWeight, i, step.WeightPercent)
		}
		if step.WeightPercent <= prevWeight {
			return nil, fmt.Errorf("%w: step %d weight %d <= prev %d", ErrNonMonotonicSteps, i, step.WeightPercent, prevWeight)
		}
		if step.MaxFailureRatePercent < 0 || step.LatencyP99ThresholdMs <= 0 {
			return nil, fmt.Errorf("%w: invalid SLA constraints in step %d", ErrInvalidSlaThreshold, i)
		}
		prevWeight = step.WeightPercent
	}

	return &CanaryPlan{
		DeploymentID: deploymentID,
		Steps:        steps,
		CurrentStep:  0,
		Phase:        RolloutPhasePending,
	}, nil
}

// EvaluateStepMetrics verifies whether current canary telemetry conforms to step SLA thresholds.
func (cp *CanaryPlan) EvaluateStepMetrics(tel RolloutTelemetry) (bool, string) {
	if cp.Phase == RolloutPhaseStable || cp.Phase == RolloutPhaseAborted {
		return false, "rollout finished"
	}

	if cp.CurrentStep >= len(cp.Steps) {
		return false, "step index out of bounds"
	}

	step := cp.Steps[cp.CurrentStep]

	// Check failure rate if samples exist
	if tel.TotalRequests > 0 {
		failRate := (float64(tel.FailedRequests) / float64(tel.TotalRequests)) * 100.0
		if failRate > step.MaxFailureRatePercent {
			return false, fmt.Sprintf("failure rate %.2f%% exceeded threshold %.2f%%", failRate, step.MaxFailureRatePercent)
		}
	}

	// Check P99 latency
	if tel.LatencyP99Ms > step.LatencyP99ThresholdMs {
		return false, fmt.Sprintf("p99 latency %.2fms exceeded threshold %.2fms", tel.LatencyP99Ms, step.LatencyP99ThresholdMs)
	}

	return true, "healthy"
}

// AdvanceStep transitions the plan to the next traffic tier, or finishes if at 100%.
func (cp *CanaryPlan) AdvanceStep() (RolloutPhase, error) {
	if cp.Phase == RolloutPhaseStable || cp.Phase == RolloutPhaseAborted {
		return cp.Phase, ErrRolloutAlreadyFinished
	}

	cp.Phase = RolloutPhaseCanary
	cp.CurrentStep++

	if cp.CurrentStep >= len(cp.Steps) {
		cp.Phase = RolloutPhaseStable
		return cp.Phase, nil
	}

	if cp.Steps[cp.CurrentStep].WeightPercent >= 100 {
		cp.Phase = RolloutPhasePromoting
	}

	return cp.Phase, nil
}

// Abort halts the rollout and sets failure reason.
func (cp *CanaryPlan) Abort(reason string) {
	cp.Phase = RolloutPhaseAborted
	cp.AbortReason = reason
}

// ActiveTrafficWeights returns the current baseline and canary traffic split.
func (cp *CanaryPlan) ActiveTrafficWeights() (int, int) {
	if cp.Phase == RolloutPhasePending {
		return 100, 0
	}
	if cp.Phase == RolloutPhaseAborted {
		return 100, 0
	}
	if cp.Phase == RolloutPhaseStable {
		return 0, 100
	}

	canaryWeight := cp.Steps[cp.CurrentStep].WeightPercent
	baselineWeight := 100 - canaryWeight
	return baselineWeight, canaryWeight
}
