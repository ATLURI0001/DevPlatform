package models

import (
	"fmt"
	"sync"
	"testing"
)

func TestTrafficSplitter_DeterministicRouting(t *testing.T) {
	t.Parallel()

	cfg := CanarySplitConfig{
		DeploymentID:          "deploy-v2.4.0",
		Salt:                  "salt-prod-cluster",
		CanaryWeight:          25,
		MaxErrorRate:          0.05,
		MinEvaluationRequests: 20,
	}

	splitter, err := NewTrafficSplitter(cfg)
	if err != nil {
		t.Fatalf("failed to create traffic splitter: %v", err)
	}

	// 1. Same user key always routes to same target
	first := splitter.RouteKey("user-12345")
	for i := 0; i < 50; i++ {
		repeat := splitter.RouteKey("user-12345")
		if repeat != first {
			t.Fatalf("routing non-deterministic for same key: got %s, expected %s", repeat, first)
		}
	}

	// 2. Hash distribution approximates configured weight (25%) over 1,000 keys
	canaryCount := 0
	totalKeys := 1000
	for i := 0; i < totalKeys; i++ {
		key := fmt.Sprintf("test-user-id-%d", i)
		if splitter.RouteKey(key) == RouteCanary {
			canaryCount++
		}
	}

	// Expect roughly 250 with tolerance [200, 300]
	canaryRatio := float64(canaryCount) / float64(totalKeys)
	if canaryRatio < 0.20 || canaryRatio > 0.30 {
		t.Errorf("canary ratio %.3f deviated excessively from expected 0.25 (count: %d)", canaryRatio, canaryCount)
	}
}

func TestTrafficSplitter_WeightSteppingAndCompletion(t *testing.T) {
	t.Parallel()

	cfg := CanarySplitConfig{
		DeploymentID: "deploy-step-1",
		Salt:         "test-salt",
		CanaryWeight: 0,
	}
	splitter, _ := NewTrafficSplitter(cfg)

	if splitter.GetStats().Phase != SplitIdle {
		t.Errorf("expected SplitIdle at 0 weight")
	}

	// Step to 10%
	_ = splitter.SetCanaryWeight(10)
	if splitter.GetStats().Phase != SplitActive || splitter.GetStats().CanaryWeight != 10 {
		t.Errorf("expected SplitActive with weight 10")
	}

	// Step to 50%
	_ = splitter.SetCanaryWeight(50)
	if splitter.GetStats().CanaryWeight != 50 {
		t.Errorf("expected weight 50")
	}

	// Step to 100% -> Completion
	_ = splitter.SetCanaryWeight(100)
	stats := splitter.GetStats()
	if stats.Phase != SplitCompleted || stats.CanaryWeight != 100 {
		t.Errorf("expected SplitCompleted, got %+v", stats)
	}

	// At 100%, all keys route to Canary
	for i := 0; i < 50; i++ {
		if splitter.RouteKey(fmt.Sprintf("key-%d", i)) != RouteCanary {
			t.Fatalf("expected all traffic to route to canary at 100%% weight")
		}
	}
}

func TestTrafficSplitter_AutoRollbackTrigger(t *testing.T) {
	t.Parallel()

	cfg := CanarySplitConfig{
		DeploymentID:          "deploy-faulty",
		Salt:                  "salt-fault",
		CanaryWeight:          30,
		MaxErrorRate:          0.10, // 10%
		MinEvaluationRequests: 20,
	}
	splitter, _ := NewTrafficSplitter(cfg)

	// Send 15 successful canary requests (below min evaluation threshold of 20)
	for i := 0; i < 15; i++ {
		splitter.RecordOutcome(RouteCanary, true)
	}
	if splitter.GetStats().Phase != SplitActive {
		t.Errorf("should still be SplitActive")
	}

	// Send 5 errors -> 5/20 = 25% error rate > 10% threshold
	for i := 0; i < 5; i++ {
		splitter.RecordOutcome(RouteCanary, false)
	}

	stats := splitter.GetStats()
	if stats.Phase != SplitRolledBack {
		t.Fatalf("expected auto-rollback, got phase %s", stats.Phase)
	}
	if stats.CanaryWeight != 0 {
		t.Errorf("expected canary weight 0 after rollback, got %d", stats.CanaryWeight)
	}
	if stats.RollbackReason == "" {
		t.Errorf("expected rollback reason to be populated")
	}

	// All subsequent traffic must now route to STABLE
	for i := 0; i < 50; i++ {
		if splitter.RouteKey(fmt.Sprintf("user-%d", i)) != RouteStable {
			t.Fatalf("expected all traffic to route to stable after rollback")
		}
	}

	// Attempting to increase weight while rolled back must fail
	err := splitter.SetCanaryWeight(50)
	if err != ErrCannotAdvanceSplit {
		t.Errorf("expected ErrCannotAdvanceSplit, got %v", err)
	}

	// Reset allows reactivation
	splitter.Reset()
	if splitter.GetStats().Phase != SplitIdle {
		t.Errorf("expected SplitIdle after reset")
	}
}

func TestTrafficSplitter_ManualRollback(t *testing.T) {
	t.Parallel()

	cfg := CanarySplitConfig{
		DeploymentID: "deploy-manual",
		Salt:         "salt",
		CanaryWeight: 40,
	}
	splitter, _ := NewTrafficSplitter(cfg)

	splitter.ManualRollback("critical alert fired on upstream API")
	stats := splitter.GetStats()
	if stats.Phase != SplitRolledBack || stats.CanaryWeight != 0 {
		t.Errorf("expected phase SplitRolledBack, got %+v", stats)
	}
	if stats.RollbackReason != "critical alert fired on upstream API" {
		t.Errorf("unexpected reason: %s", stats.RollbackReason)
	}
}

func TestTrafficSplitter_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	cfg := CanarySplitConfig{
		DeploymentID:          "deploy-race",
		Salt:                  "salt-race",
		CanaryWeight:          50,
		MaxErrorRate:          0.20,
		MinEvaluationRequests: 500,
	}
	splitter, _ := NewTrafficSplitter(cfg)

	var wg sync.WaitGroup
	workers := 10
	iterations := 200

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("user-%d-%d", workerID, i)
				route := splitter.RouteKey(key)
				splitter.RecordOutcome(route, i%5 != 0) // 20% errors
				_ = splitter.GetStats()
			}
		}(w)
	}
	wg.Wait()

	stats := splitter.GetStats()
	if stats.StableMetrics.TotalRequests+stats.CanaryMetrics.TotalRequests != uint64(workers*iterations) {
		t.Errorf("mismatched total requests: expected %d, got %d",
			workers*iterations, stats.StableMetrics.TotalRequests+stats.CanaryMetrics.TotalRequests)
	}
}
