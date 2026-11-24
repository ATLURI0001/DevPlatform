package models

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func buildSampleDAG() *ResourceGraph {
	g := NewResourceGraph()
	// DB has no deps
	g.AddNode(ResourceGraphNode{ID: "db", Kind: "Database"})
	// Redis has no deps
	g.AddNode(ResourceGraphNode{ID: "redis", Kind: "Cache"})
	// Auth depends on DB and Redis
	g.AddNode(ResourceGraphNode{ID: "auth-svc", Kind: "Service", Dependencies: []string{"db", "redis"}})
	// API depends on Auth
	g.AddNode(ResourceGraphNode{ID: "api-gateway", Kind: "Gateway", Dependencies: []string{"auth-svc"}})
	// Ingress depends on API Gateway
	g.AddNode(ResourceGraphNode{ID: "ingress", Kind: "Ingress", Dependencies: []string{"api-gateway"}})
	return g
}

func TestResourceGraph_TopologicalSortAndCycleDetection(t *testing.T) {
	g := buildSampleDAG()

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error during topological sort: %v", err)
	}

	if len(order) != 5 {
		t.Fatalf("expected 5 nodes in sorted order, got %d", len(order))
	}

	// Verify order constraints
	indexMap := make(map[string]int)
	for i, id := range order {
		indexMap[id] = i
	}

	if indexMap["db"] > indexMap["auth-svc"] || indexMap["redis"] > indexMap["auth-svc"] {
		t.Errorf("db/redis must come before auth-svc in deploy order: %v", order)
	}
	if indexMap["auth-svc"] > indexMap["api-gateway"] {
		t.Errorf("auth-svc must come before api-gateway: %v", order)
	}
	if indexMap["api-gateway"] > indexMap["ingress"] {
		t.Errorf("api-gateway must come before ingress: %v", order)
	}

	// Cycle test: add A -> B -> A
	gCycle := NewResourceGraph()
	gCycle.AddNode(ResourceGraphNode{ID: "nodeA", Kind: "Service", Dependencies: []string{"nodeB"}})
	gCycle.AddNode(ResourceGraphNode{ID: "nodeB", Kind: "Service", Dependencies: []string{"nodeA"}})

	_, errCycle := gCycle.TopologicalSort()
	if !errors.Is(errCycle, ErrCyclicDependency) {
		t.Errorf("expected ErrCyclicDependency, got %v", errCycle)
	}
}

func TestCalculateDurationPercentiles(t *testing.T) {
	durations := make([]time.Duration, 100)
	for i := 1; i <= 100; i++ {
		durations[i-1] = time.Duration(i) * time.Millisecond
	}

	p50, p90, p95, p99, min, max, mean, stddev := CalculateDurationPercentiles(durations)
	if min != 1.0 || max != 100.0 {
		t.Errorf("unexpected min/max: min=%f, max=%f", min, max)
	}
	if p50 != 50.0 || p90 != 90.0 || p95 != 95.0 || p99 != 99.0 {
		t.Errorf("unexpected percentiles: p50=%f, p90=%f, p95=%f, p99=%f", p50, p90, p95, p99)
	}
	if mean != 50.5 || stddev <= 0 {
		t.Errorf("unexpected mean/stddev: mean=%f, stddev=%f", mean, stddev)
	}
}

func TestGraphBenchmarkProfiler_Execution(t *testing.T) {
	g := buildSampleDAG()
	profiler := NewGraphBenchmarkProfiler(g)

	// Sequential
	seq := profiler.RunSequentialSortBenchmark(50)
	if seq.TotalOps != 50 || seq.ErrorCount != 0 {
		t.Errorf("unexpected sequential benchmark result: %+v", seq)
	}
	if seq.ThroughputRPS <= 0 {
		t.Errorf("expected positive throughput, got %f", seq.ThroughputRPS)
	}

	// Concurrent
	concur := profiler.RunConcurrentSortBenchmark(4, 25)
	if concur.TotalOps != 100 || concur.ErrorCount != 0 {
		t.Errorf("unexpected concurrent benchmark result: %+v", concur)
	}

	// Markdown
	md := seq.FormatMarkdown()
	if !strings.Contains(md, "Throughput") || !strings.Contains(md, "P99 Latency") {
		t.Errorf("unexpected markdown output:\n%s", md)
	}
}

func BenchmarkTopologicalSort(b *testing.B) {
	g := buildSampleDAG()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = g.TopologicalSort()
	}
}

func BenchmarkTopologicalSortParallel(b *testing.B) {
	g := buildSampleDAG()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = g.TopologicalSort()
		}
	})
}
