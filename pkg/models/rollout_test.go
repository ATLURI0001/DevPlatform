package models

import (
	"errors"
	"testing"
	"time"
)

func TestNewCanaryPlan_Validation(t *testing.T) {
	// Valid progressive plan
	validSteps := []CanaryStep{
		{WeightPercent: 10, PauseDuration: 10 * time.Second, MaxFailureRatePercent: 1.0, LatencyP99ThresholdMs: 200},
		{WeightPercent: 25, PauseDuration: 10 * time.Second, MaxFailureRatePercent: 1.0, LatencyP99ThresholdMs: 200},
		{WeightPercent: 50, PauseDuration: 10 * time.Second, MaxFailureRatePercent: 0.5, LatencyP99ThresholdMs: 150},
		{WeightPercent: 100, PauseDuration: 0, MaxFailureRatePercent: 0.5, LatencyP99ThresholdMs: 150},
	}
	plan, err := NewCanaryPlan("deploy-99", validSteps)
	if err != nil {
		t.Fatalf("unexpected error creating valid plan: %v", err)
	}
	if plan.Phase != RolloutPhasePending {
		t.Fatalf("expected initial phase PENDING, got %s", plan.Phase)
	}

	// Non-monotonic steps
	badSteps := []CanaryStep{
		{WeightPercent: 25, MaxFailureRatePercent: 1.0, LatencyP99ThresholdMs: 200},
		{WeightPercent: 10, MaxFailureRatePercent: 1.0, LatencyP99ThresholdMs: 200},
	}
	_, errNonMono := NewCanaryPlan("deploy-99", badSteps)
	if !errors.Is(errNonMono, ErrNonMonotonicSteps) {
		t.Fatalf("expected ErrNonMonotonicSteps, got %v", errNonMono)
	}

	// Out of bounds weight (> 100)
	overWeight := []CanaryStep{
		{WeightPercent: 120, MaxFailureRatePercent: 1.0, LatencyP99ThresholdMs: 200},
	}
	_, errOver := NewCanaryPlan("deploy-99", overWeight)
	if !errors.Is(errOver, ErrInvalidCanaryWeight) {
		t.Fatalf("expected ErrInvalidCanaryWeight, got %v", errOver)
	}
}

func TestCanaryPlan_TrafficSplittingAndAdvancement(t *testing.T) {
	steps := []CanaryStep{
		{WeightPercent: 10, MaxFailureRatePercent: 1.0, LatencyP99ThresholdMs: 200},
		{WeightPercent: 50, MaxFailureRatePercent: 1.0, LatencyP99ThresholdMs: 200},
		{WeightPercent: 100, MaxFailureRatePercent: 1.0, LatencyP99ThresholdMs: 200},
	}
	plan, err := NewCanaryPlan("deploy-1", steps)
	if err != nil {
		t.Fatalf("unexpected plan creation error: %v", err)
	}

	// Initial pending traffic: 100% baseline, 0% canary
	base, canary := plan.ActiveTrafficWeights()
	if base != 100 || canary != 0 {
		t.Fatalf("expected 100/0 split in pending, got %d/%d", base, canary)
	}

	// Step 0: 10% canary
	plan.Phase = RolloutPhaseCanary
	base, canary = plan.ActiveTrafficWeights()
	if base != 90 || canary != 10 {
		t.Fatalf("expected 90/10 split at step 0, got %d/%d", base, canary)
	}

	// Advance to Step 1 (50%)
	phase, err := plan.AdvanceStep()
	if err != nil || phase != RolloutPhaseCanary {
		t.Fatalf("advance step 1 failed: %v", err)
	}
	base, canary = plan.ActiveTrafficWeights()
	if base != 50 || canary != 50 {
		t.Fatalf("expected 50/50 split at step 1, got %d/%d", base, canary)
	}

	// Advance to Step 2 (100% -> PROMOTING)
	phase, err = plan.AdvanceStep()
	if err != nil || phase != RolloutPhasePromoting {
		t.Fatalf("advance step 2 failed: phase=%s, err=%v", phase, err)
	}
	base, canary = plan.ActiveTrafficWeights()
	if base != 0 || canary != 100 {
		t.Fatalf("expected 0/100 split at step 2, got %d/%d", base, canary)
	}

	// Advance past last step -> STABLE
	phase, err = plan.AdvanceStep()
	if err != nil || phase != RolloutPhaseStable {
		t.Fatalf("advance to stable failed: phase=%s, err=%v", phase, err)
	}

	// Trying to advance further returns ErrRolloutAlreadyFinished
	_, errDone := plan.AdvanceStep()
	if !errors.Is(errDone, ErrRolloutAlreadyFinished) {
		t.Fatalf("expected ErrRolloutAlreadyFinished, got %v", errDone)
	}
}

func TestCanaryPlan_EvaluateSlaAndAbort(t *testing.T) {
	steps := []CanaryStep{
		{WeightPercent: 10, MaxFailureRatePercent: 2.0, LatencyP99ThresholdMs: 250},
	}
	plan, _ := NewCanaryPlan("deploy-2", steps)
	plan.Phase = RolloutPhaseCanary

	// Healthy telemetry
	healthy := RolloutTelemetry{
		TotalRequests:  1000,
		FailedRequests: 5,   // 0.5% failure rate
		LatencyP99Ms:   120, // 120ms < 250ms
	}
	pass, msg := plan.EvaluateStepMetrics(healthy)
	if !pass {
		t.Fatalf("expected healthy evaluation, got failed: %s", msg)
	}

	// Breach 1: Failure rate overshoot
	highErrors := RolloutTelemetry{
		TotalRequests:  1000,
		FailedRequests: 50, // 5.0% failure rate > 2.0%
		LatencyP99Ms:   120,
	}
	pass, _ = plan.EvaluateStepMetrics(highErrors)
	if pass {
		t.Fatal("expected failure rate breach to fail evaluation")
	}

	// Breach 2: Latency P99 overshoot
	highLatency := RolloutTelemetry{
		TotalRequests:  1000,
		FailedRequests: 5,
		LatencyP99Ms:   350, // 350ms > 250ms
	}
	pass, _ = plan.EvaluateStepMetrics(highLatency)
	if pass {
		t.Fatal("expected latency breach to fail evaluation")
	}

	// Abort action
	plan.Abort("excessive latency detected")
	if plan.Phase != RolloutPhaseAborted {
		t.Fatalf("expected phase ABORTED, got %s", plan.Phase)
	}
	base, canary := plan.ActiveTrafficWeights()
	if base != 100 || canary != 0 {
		t.Fatalf("expected traffic to instantly revert to 100/0 on abort, got %d/%d", base, canary)
	}
}
