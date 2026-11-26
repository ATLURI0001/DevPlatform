package models

import (
	"errors"
	"strings"
	"testing"
)

func TestPipelineDAGProfiler_ExecutionPlan(t *testing.T) {
	profiler := NewPipelineDAGProfiler()

	// 1. Diamond topology
	diamond := profiler.GenerateTopology("diamond", 0)
	plan, err := profiler.ComputeExecutionPlan(diamond)
	if err != nil {
		t.Fatalf("unexpected error computing diamond plan: %v", err)
	}

	if plan.TotalStages != 6 {
		t.Fatalf("expected 6 total stages, got %d", plan.TotalStages)
	}
	if len(plan.Waves) != 5 {
		// Wave 1: lint
		// Wave 2: unit-tests, security-scan
		// Wave 3: build-image
		// Wave 4: e2e-tests
		// Wave 5: deploy
		t.Fatalf("expected 5 waves, got %d", len(plan.Waves))
	}

	// Critical path: lint(50) + security-scan(200) + build-image(300) + e2e-tests(400) + deploy(100) = 1050ms
	// Sequential: 50 + 150 + 200 + 300 + 400 + 100 = 1200ms
	if plan.CriticalPathMs != 1050 {
		t.Fatalf("expected critical path 1050ms, got %d", plan.CriticalPathMs)
	}
	if plan.TheoreticalSpeedup <= 1.0 {
		t.Fatalf("expected parallel speedup > 1.0, got %f", plan.TheoreticalSpeedup)
	}

	// 2. Empty pipeline check
	_, errEmpty := profiler.ComputeExecutionPlan(nil)
	if !errors.Is(errEmpty, ErrEmptyPipeline) {
		t.Fatalf("expected ErrEmptyPipeline, got %v", errEmpty)
	}

	// 3. Cyclic pipeline check
	cyclicStages := []PipelineStage{
		{ID: "A", Dependencies: []string{"B"}},
		{ID: "B", Dependencies: []string{"A"}},
	}
	_, errCycle := profiler.ComputeExecutionPlan(cyclicStages)
	if !errors.Is(errCycle, ErrPipelineCycle) {
		t.Fatalf("expected ErrPipelineCycle, got %v", errCycle)
	}
}

func TestPipelineDAGProfiler_ProfilingAndReporting(t *testing.T) {
	profiler := NewPipelineDAGProfiler()

	linear := profiler.GenerateTopology("linear", 8)
	resLinear, err := profiler.ProfileTopology("linear-8", linear, 100)
	if err != nil {
		t.Fatalf("failed profiling linear: %v", err)
	}

	fanout := profiler.GenerateTopology("fanout_fanin", 8)
	resFanout, err := profiler.ProfileTopology("fanout-8", fanout, 100)
	if err != nil {
		t.Fatalf("failed profiling fanout: %v", err)
	}

	if resLinear.EvaluationsPerSec <= 0 || resFanout.EvaluationsPerSec <= 0 {
		t.Fatalf("expected positive throughput")
	}
	if resLinear.Stats.P50Microseconds > resLinear.Stats.P99Microseconds {
		t.Fatalf("percentile inversion: P50 %f > P99 %f", resLinear.Stats.P50Microseconds, resLinear.Stats.P99Microseconds)
	}

	report := profiler.FormatMarkdownReport([]PipelineDAGProfileResult{resLinear, resFanout})
	if !strings.Contains(report, "# Deployment Pipeline DAG Evaluation Benchmark Report") {
		t.Fatalf("report missing title")
	}
	if !strings.Contains(report, "`linear-8`") || !strings.Contains(report, "`fanout-8`") {
		t.Fatalf("report missing topology rows")
	}
}
