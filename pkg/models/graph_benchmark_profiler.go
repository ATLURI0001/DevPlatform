package models

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCyclicDependency = errors.New("graph: cyclic dependency detected")
	ErrNodeNotFound     = errors.New("graph: node not found in graph")
	ErrDuplicateNode    = errors.New("graph: duplicate node id")
)

// ResourceGraphNode represents a deployable infrastructure unit with explicit dependencies.
type ResourceGraphNode struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// ResourceGraph represents an in-memory DAG for deployment topological order resolution.
type ResourceGraph struct {
	mu    sync.RWMutex
	nodes map[string]ResourceGraphNode
}

// NewResourceGraph constructs an empty thread-safe resource graph.
func NewResourceGraph() *ResourceGraph {
	return &ResourceGraph{
		nodes: make(map[string]ResourceGraphNode),
	}
}

// AddNode adds a resource node to the graph.
func (g *ResourceGraph) AddNode(node ResourceGraphNode) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if node.ID == "" {
		return errors.New("graph: node ID cannot be empty")
	}
	if _, exists := g.nodes[node.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateNode, node.ID)
	}

	g.nodes[node.ID] = node
	return nil
}

// TopologicalSort computes an optimal deploy sequence using Kahn's algorithm in O(V + E).
func (g *ResourceGraph) TopologicalSort() ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	inDegree := make(map[string]int, len(g.nodes))
	adj := make(map[string][]string, len(g.nodes))

	for id := range g.nodes {
		inDegree[id] = 0
		adj[id] = make([]string, 0)
	}

	for id, node := range g.nodes {
		for _, dep := range node.Dependencies {
			if _, exists := g.nodes[dep]; !exists {
				return nil, fmt.Errorf("%w: %s -> missing %s", ErrNodeNotFound, id, dep)
			}
			// dep must be deployed before id (dep -> id)
			adj[dep] = append(adj[dep], id)
			inDegree[id]++
		}
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	// Sort initial queue for deterministic execution order
	sort.Strings(queue)

	sortedOrder := make([]string, 0, len(g.nodes))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		sortedOrder = append(sortedOrder, curr)

		for _, neighbor := range adj[curr] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sortedOrder) != len(g.nodes) {
		return nil, ErrCyclicDependency
	}

	return sortedOrder, nil
}

// GraphBenchmarkMetrics holds computed percentiles and throughput for graph operations.
type GraphBenchmarkMetrics struct {
	Name          string  `json:"name"`
	TotalOps      int     `json:"total_ops"`
	DurationSec   float64 `json:"duration_sec"`
	ThroughputRPS float64 `json:"throughput_rps"`
	P50Ms         float64 `json:"p50_ms"`
	P90Ms         float64 `json:"p90_ms"`
	P95Ms         float64 `json:"p95_ms"`
	P99Ms         float64 `json:"p99_ms"`
	MinMs         float64 `json:"min_ms"`
	MaxMs         float64 `json:"max_ms"`
	MeanMs        float64 `json:"mean_ms"`
	StdDevMs      float64 `json:"stddev_ms"`
	ErrorCount    int     `json:"error_count"`
}

// CalculateDurationPercentiles computes exact sorted percentiles from a slice of durations.
func CalculateDurationPercentiles(durations []time.Duration) (p50, p90, p95, p99, min, max, mean, stddev float64) {
	n := len(durations)
	if n == 0 {
		return 0, 0, 0, 0, 0, 0, 0, 0
	}

	floats := make([]float64, n)
	var sum float64
	for i, d := range durations {
		ms := float64(d.Nanoseconds()) / 1e6
		floats[i] = ms
		sum += ms
	}
	sort.Float64s(floats)

	pct := func(p float64) float64 {
		idx := int(math.Ceil((p / 100.0) * float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return floats[idx]
	}

	p50 = pct(50)
	p90 = pct(90)
	p95 = pct(95)
	p99 = pct(99)
	min = floats[0]
	max = floats[n-1]
	mean = sum / float64(n)

	var varSum float64
	for _, v := range floats {
		diff := v - mean
		varSum += diff * diff
	}
	stddev = math.Sqrt(varSum / float64(n))
	return
}

// GraphBenchmarkProfiler runs sequential and parallel benchmark workloads on resource DAGs.
type GraphBenchmarkProfiler struct {
	graph *ResourceGraph
}

// NewGraphBenchmarkProfiler creates a profiler instance for a given graph.
func NewGraphBenchmarkProfiler(graph *ResourceGraph) *GraphBenchmarkProfiler {
	return &GraphBenchmarkProfiler{graph: graph}
}

// RunSequentialSortBenchmark executes single-threaded topological sort profiling.
func (p *GraphBenchmarkProfiler) RunSequentialSortBenchmark(iterations int) GraphBenchmarkMetrics {
	durations := make([]time.Duration, 0, iterations)
	errorsCount := 0

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t0 := time.Now()
		_, err := p.graph.TopologicalSort()
		durations = append(durations, time.Since(t0))
		if err != nil {
			errorsCount++
		}
	}
	totalDuration := time.Since(start)

	p50, p90, p95, p99, min, max, mean, stddev := CalculateDurationPercentiles(durations)
	rps := float64(iterations) / totalDuration.Seconds()

	return GraphBenchmarkMetrics{
		Name:          "ResourceGraph TopologicalSort Sequential",
		TotalOps:      iterations,
		DurationSec:   totalDuration.Seconds(),
		ThroughputRPS: rps,
		P50Ms:         p50,
		P90Ms:         p90,
		P95Ms:         p95,
		P99Ms:         p99,
		MinMs:         min,
		MaxMs:         max,
		MeanMs:        mean,
		StdDevMs:      stddev,
		ErrorCount:    errorsCount,
	}
}

// RunConcurrentSortBenchmark executes multi-worker parallel topological sort profiling.
func (p *GraphBenchmarkProfiler) RunConcurrentSortBenchmark(workers int, iterationsPerWorker int) GraphBenchmarkMetrics {
	totalOps := workers * iterationsPerWorker
	durations := make([]time.Duration, totalOps)
	var errCount int64
	var wg sync.WaitGroup

	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			offset := workerID * iterationsPerWorker
			for i := 0; i < iterationsPerWorker; i++ {
				t0 := time.Now()
				_, err := p.graph.TopologicalSort()
				durations[offset+i] = time.Since(t0)
				if err != nil {
					syncAtomicAddInt64(&errCount, 1)
				}
			}
		}(w)
	}
	wg.Wait()
	totalDuration := time.Since(start)

	p50, p90, p95, p99, min, max, mean, stddev := CalculateDurationPercentiles(durations)
	rps := float64(totalOps) / totalDuration.Seconds()

	return GraphBenchmarkMetrics{
		Name:          fmt.Sprintf("ResourceGraph TopologicalSort Concurrent (Workers: %d)", workers),
		TotalOps:      totalOps,
		DurationSec:   totalDuration.Seconds(),
		ThroughputRPS: rps,
		P50Ms:         p50,
		P90Ms:         p90,
		P95Ms:         p95,
		P99Ms:         p99,
		MinMs:         min,
		MaxMs:         max,
		MeanMs:        mean,
		StdDevMs:      stddev,
		ErrorCount:    int(errCount),
	}
}

// FormatMarkdown outputs metrics as a Markdown table.
func (m *GraphBenchmarkMetrics) FormatMarkdown() string {
	return fmt.Sprintf("| Metric | Value |\n"+
		"| :--- | :--- |\n"+
		"| **Benchmark** | `%s` |\n"+
		"| **Total Traversals** | `%d` ops |\n"+
		"| **Duration** | `%.4f` s |\n"+
		"| **Throughput** | `%.2f` ops/sec |\n"+
		"| **P50 Latency** | `%.4f` ms |\n"+
		"| **P90 Latency** | `%.4f` ms |\n"+
		"| **P95 Latency** | `%.4f` ms |\n"+
		"| **P99 Latency** | `%.4f` ms |\n"+
		"| **Min / Max Latency** | `%.4f` ms / `%.4f` ms |\n"+
		"| **Mean / StdDev** | `%.4f` ms ± `%.4f` ms |\n"+
		"| **Errors** | `%d` |\n",
		m.Name, m.TotalOps, m.DurationSec, m.ThroughputRPS,
		m.P50Ms, m.P90Ms, m.P95Ms, m.P99Ms, m.MinMs, m.MaxMs, m.MeanMs, m.StdDevMs, m.ErrorCount)
}

func syncAtomicAddInt64(val *int64, delta int64) {
	atomic.AddInt64(val, delta)
}
