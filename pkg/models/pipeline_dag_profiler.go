package models

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

var (
	ErrEmptyPipeline = errors.New("pipeline: cannot profile empty stage list")
	ErrPipelineCycle = errors.New("pipeline: cyclic stage dependency detected")
)

// PipelineStage represents a discrete task within a deployment pipeline.
type PipelineStage struct {
	ID           string   `json:"id"`
	EstimatedMs  int      `json:"estimated_ms"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// PipelineExecutionPlan details parallel waves and critical path schedule.
type PipelineExecutionPlan struct {
	Waves              [][]string `json:"waves"`
	TotalStages        int        `json:"total_stages"`
	CriticalPathMs     int        `json:"critical_path_ms"`
	SequentialMs       int        `json:"sequential_ms"`
	TheoreticalSpeedup float64    `json:"theoretical_speedup"`
}

// DAGProfileStats holds fine-grained evaluation latency distributions.
type DAGProfileStats struct {
	SampleCount     int     `json:"sample_count"`
	MinMicroseconds float64 `json:"min_us"`
	P50Microseconds float64 `json:"p50_us"`
	P90Microseconds float64 `json:"p90_us"`
	P99Microseconds float64 `json:"p99_us"`
	MaxMicroseconds float64 `json:"max_us"`
	MeanMicroseconds float64 `json:"mean_us"`
}

// PipelineDAGProfileResult holds the full benchmark outcome.
type PipelineDAGProfileResult struct {
	TopologyName     string                `json:"topology_name"`
	StageCount       int                   `json:"stage_count"`
	Iterations       int                   `json:"iterations"`
	EvaluationsPerSec float64              `json:"evaluations_per_sec"`
	Plan             PipelineExecutionPlan `json:"plan"`
	Stats            DAGProfileStats       `json:"stats"`
}

// PipelineDAGProfiler benchmarks topological wave scheduling and critical path resolution.
type PipelineDAGProfiler struct{}

// NewPipelineDAGProfiler creates a pipeline DAG profiler.
func NewPipelineDAGProfiler() *PipelineDAGProfiler {
	return &PipelineDAGProfiler{}
}

// ComputeExecutionPlan resolves parallel wave execution and critical path latency.
func (p *PipelineDAGProfiler) ComputeExecutionPlan(stages []PipelineStage) (PipelineExecutionPlan, error) {
	if len(stages) == 0 {
		return PipelineExecutionPlan{}, ErrEmptyPipeline
	}

	stageMap := make(map[string]PipelineStage, len(stages))
	inDegree := make(map[string]int, len(stages))
	adjList := make(map[string][]string, len(stages))
	sequentialMs := 0

	for _, s := range stages {
		stageMap[s.ID] = s
		inDegree[s.ID] = 0
		sequentialMs += s.EstimatedMs
	}

	for _, s := range stages {
		for _, dep := range s.Dependencies {
			if _, exists := stageMap[dep]; !exists {
				return PipelineExecutionPlan{}, fmt.Errorf("dependency stage '%s' not found", dep)
			}
			adjList[dep] = append(adjList[dep], s.ID)
			inDegree[s.ID]++
		}
	}

	// Kahn's algorithm for level-by-level wave partitioning
	var waves [][]string
	ready := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}

	sort.Strings(ready)
	processedCount := 0

	// Earliest completion times for critical path calculation
	earliestEnd := make(map[string]int, len(stages))

	for len(ready) > 0 {
		currentWave := make([]string, len(ready))
		copy(currentWave, ready)
		sort.Strings(currentWave)
		waves = append(waves, currentWave)

		nextReady := make([]string, 0)

		for _, nodeID := range currentWave {
			processedCount++
			nodeDuration := stageMap[nodeID].EstimatedMs

			// Calculate earliest finish time
			maxDepEnd := 0
			for _, depID := range stageMap[nodeID].Dependencies {
				if earliestEnd[depID] > maxDepEnd {
					maxDepEnd = earliestEnd[depID]
				}
			}
			earliestEnd[nodeID] = maxDepEnd + nodeDuration

			for _, dependent := range adjList[nodeID] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextReady = append(nextReady, dependent)
				}
			}
		}

		sort.Strings(nextReady)
		ready = nextReady
	}

	if processedCount != len(stages) {
		return PipelineExecutionPlan{}, ErrPipelineCycle
	}

	criticalPathMs := 0
	for _, end := range earliestEnd {
		if end > criticalPathMs {
			criticalPathMs = end
		}
	}

	speedup := 1.0
	if criticalPathMs > 0 {
		speedup = float64(sequentialMs) / float64(criticalPathMs)
	}

	return PipelineExecutionPlan{
		Waves:              waves,
		TotalStages:        len(stages),
		CriticalPathMs:     criticalPathMs,
		SequentialMs:       sequentialMs,
		TheoreticalSpeedup: math.Round(speedup*100) / 100,
	}, nil
}

// GenerateTopology generates synthetic pipeline DAGs for benchmarking.
func (p *PipelineDAGProfiler) GenerateTopology(kind string, size int) []PipelineStage {
	if size <= 0 {
		size = 10
	}

	switch kind {
	case "linear":
		stages := make([]PipelineStage, size)
		for i := 0; i < size; i++ {
			var deps []string
			if i > 0 {
				deps = []string{fmt.Sprintf("stage-%d", i-1)}
			}
			stages[i] = PipelineStage{
				ID:           fmt.Sprintf("stage-%d", i),
				EstimatedMs:  100 + (i % 50),
				Dependencies: deps,
			}
		}
		return stages

	case "fanout_fanin":
		// stage-0 fans out to N-2 parallel workers, converging into stage-final
		stages := make([]PipelineStage, 0, size)
		stages = append(stages, PipelineStage{ID: "stage-start", EstimatedMs: 50})

		workerCount := size - 2
		if workerCount <= 0 {
			workerCount = 1
		}
		workerIDs := make([]string, workerCount)
		for i := 0; i < workerCount; i++ {
			id := fmt.Sprintf("worker-%d", i)
			workerIDs[i] = id
			stages = append(stages, PipelineStage{
				ID:           id,
				EstimatedMs:  200 + (i % 100),
				Dependencies: []string{"stage-start"},
			})
		}

		stages = append(stages, PipelineStage{
			ID:           "stage-finish",
			EstimatedMs:  100,
			Dependencies: workerIDs,
		})
		return stages

	case "diamond":
		// Multi-tier diamond DAG
		return []PipelineStage{
			{ID: "lint", EstimatedMs: 50},
			{ID: "unit-tests", EstimatedMs: 150, Dependencies: []string{"lint"}},
			{ID: "security-scan", EstimatedMs: 200, Dependencies: []string{"lint"}},
			{ID: "build-image", EstimatedMs: 300, Dependencies: []string{"unit-tests", "security-scan"}},
			{ID: "e2e-tests", EstimatedMs: 400, Dependencies: []string{"build-image"}},
			{ID: "deploy", EstimatedMs: 100, Dependencies: []string{"e2e-tests"}},
		}

	default:
		return p.GenerateTopology("linear", size)
	}
}

// ProfileTopology runs microbenchmarks measuring DAG evaluation throughput and latency.
func (p *PipelineDAGProfiler) ProfileTopology(name string, stages []PipelineStage, iterations int) (PipelineDAGProfileResult, error) {
	if iterations <= 0 {
		iterations = 1000
	}

	plan, err := p.ComputeExecutionPlan(stages)
	if err != nil {
		return PipelineDAGProfileResult{}, err
	}

	latencies := make([]time.Duration, iterations)
	t0 := time.Now()

	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, _ = p.ComputeExecutionPlan(stages)
		latencies[i] = time.Since(start)
	}

	elapsed := time.Since(t0).Seconds()
	opsPerSec := float64(iterations) / math.Max(0.000001, elapsed)

	return PipelineDAGProfileResult{
		TopologyName:      name,
		StageCount:        len(stages),
		Iterations:        iterations,
		EvaluationsPerSec: math.Round(opsPerSec),
		Plan:              plan,
		Stats:             computeDAGStats(latencies),
	}, nil
}

func computeDAGStats(latencies []time.Duration) DAGProfileStats {
	n := len(latencies)
	if n == 0 {
		return DAGProfileStats{}
	}

	micros := make([]float64, n)
	var sum float64
	for i, d := range latencies {
		us := float64(d.Nanoseconds()) / 1e3
		micros[i] = us
		sum += us
	}
	sort.Float64s(micros)

	pct := func(p float64) float64 {
		idx := int(math.Ceil((p/100.0)*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return micros[idx]
	}

	return DAGProfileStats{
		SampleCount:     n,
		MinMicroseconds: math.Round(micros[0]*100) / 100,
		P50Microseconds: math.Round(pct(50.0)*100) / 100,
		P90Microseconds: math.Round(pct(90.0)*100) / 100,
		P99Microseconds: math.Round(pct(99.0)*100) / 100,
		MaxMicroseconds: math.Round(micros[n-1]*100) / 100,
		MeanMicroseconds: math.Round((sum/float64(n))*100) / 100,
	}
}

// FormatMarkdownReport renders a markdown comparison table for pipeline topologies.
func (p *PipelineDAGProfiler) FormatMarkdownReport(results []PipelineDAGProfileResult) string {
	report := "# Deployment Pipeline DAG Evaluation Benchmark Report\n\n"
	report += "| Topology | Stages | Waves | Sequential | Critical Path | Speedup | Throughput | P50 Latency |\n"
	report += "| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n"

	for _, r := range results {
		report += fmt.Sprintf(
			"| `%s` | %d | %d | %dms | %dms | **%.2fx** | %0.0f eval/s | %.2f µs |\n",
			r.TopologyName, r.StageCount, len(r.Plan.Waves),
			r.Plan.SequentialMs, r.Plan.CriticalPathMs, r.Plan.TheoreticalSpeedup,
			r.EvaluationsPerSec, r.Stats.P50Microseconds,
		)
	}

	return report
}
