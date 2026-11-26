package models

import (
	"sort"
	"sync"
	"time"
)

// PipelineQueueSample captures CI/CD runner queue wait duration and active jobs.
type PipelineQueueSample struct {
	RunnerPool   string
	QueueWaitMs  int64
	ActiveJobs   int
	Timestamp    time.Time
}

// PipelineQueueSummary aggregates CI/CD runner saturation metrics.
type PipelineQueueSummary struct {
	SampleCount   int
	MaxQueueWaitMs int64
	P50WaitMs     int64
	P90WaitMs     int64
	MeanActive    float64
}

// PipelineQueueRotary is a bounded ring buffer for CI/CD runner queue telemetry.
type PipelineQueueRotary struct {
	mu       sync.RWMutex
	capacity int
	buffer   []PipelineQueueSample
	head     int
	count    int
}

// NewPipelineQueueRotary creates a new bounded telemetry rotary.
func NewPipelineQueueRotary(capacity int) *PipelineQueueRotary {
	if capacity <= 0 {
		capacity = 256
	}
	return &PipelineQueueRotary{
		capacity: capacity,
		buffer:   make([]PipelineQueueSample, capacity),
	}
}

// Record stores a queue wait sample.
func (r *PipelineQueueRotary) Record(pool string, waitMs int64, activeJobs int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buffer[r.head] = PipelineQueueSample{
		RunnerPool:  pool,
		QueueWaitMs: waitMs,
		ActiveJobs:  activeJobs,
		Timestamp:   time.Now(),
	}
	r.head = (r.head + 1) % r.capacity
	if r.count < r.capacity {
		r.count++
	}
}

// GetSummary returns aggregated wait quantiles and active runner averages.
func (r *PipelineQueueRotary) GetSummary() PipelineQueueSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.count == 0 {
		return PipelineQueueSummary{}
	}

	waits := make([]int64, r.count)
	var maxWait int64
	var sumActive int

	for i := 0; i < r.count; i++ {
		s := r.buffer[i]
		waits[i] = s.QueueWaitMs
		if s.QueueWaitMs > maxWait {
			maxWait = s.QueueWaitMs
		}
		sumActive += s.ActiveJobs
	}

	sort.Slice(waits, func(i, j int) bool { return waits[i] < waits[j] })
	return PipelineQueueSummary{
		SampleCount:    r.count,
		MaxQueueWaitMs: maxWait,
		P50WaitMs:      waits[int(float64(r.count)*0.5)],
		P90WaitMs:      waits[int(float64(r.count)*0.9)],
		MeanActive:     float64(sumActive) / float64(r.count),
	}
}
