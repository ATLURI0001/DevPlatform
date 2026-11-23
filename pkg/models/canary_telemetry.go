package models

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// CanaryVerdict represents the automated health diagnosis of the canary revision.
type CanaryVerdict string

const (
	VerdictHealthy            CanaryVerdict = "CANARY_HEALTHY"
	VerdictDegradedErrors     CanaryVerdict = "CANARY_DEGRADED_ERRORS"
	VerdictDegradedLatency    CanaryVerdict = "CANARY_DEGRADED_LATENCY"
	VerdictInsufficientSample CanaryVerdict = "CANARY_INSUFFICIENT_SAMPLES"
)

// CanaryAction recommends the next progressive delivery action.
type CanaryAction string

const (
	ActionPromote CanaryAction = "RECOMMEND_PROMOTE"
	ActionHold    CanaryAction = "RECOMMEND_HOLD"
	ActionAbort   CanaryAction = "RECOMMEND_ABORT"
)

// RouteVariant identifies whether traffic belongs to baseline or canary revision.
type RouteVariant string

const (
	VariantBaseline RouteVariant = "BASELINE"
	VariantCanary   RouteVariant = "CANARY"
)

// CanaryThresholds configures diagnostic limits for divergence detection.
type CanaryThresholds struct {
	MinSamplesPerTrack    int     `json:"min_samples_per_track"`
	MaxErrorRateDelta     float64 `json:"max_error_rate_delta"`     // e.g. 0.02 = max 2% error rate increase
	MaxLatencyDivergenceP99 float64 `json:"max_latency_divergence_p99"` // e.g. 1.30 = max 30% p99 increase
}

// DefaultCanaryThresholds returns calibrated operational defaults.
func DefaultCanaryThresholds() CanaryThresholds {
	return CanaryThresholds{
		MinSamplesPerTrack:      50,
		MaxErrorRateDelta:       0.02,
		MaxLatencyDivergenceP99: 1.30,
	}
}

// TrackMetrics captures telemetry statistics for a specific revision track.
type TrackMetrics struct {
	TotalRequests uint64  `json:"total_requests"`
	SuccessCount  uint64  `json:"success_count"`
	ClientErrors  uint64  `json:"client_errors"` // 4xx
	ServerErrors  uint64  `json:"server_errors"` // 5xx
	ErrorRate     float64 `json:"error_rate"`    // Server error fraction [0.0, 1.0]
	P50Ms         float64 `json:"p50_ms"`
	P95Ms         float64 `json:"p95_ms"`
	P99Ms         float64 `json:"p99_ms"`
}

// CanaryDiagnosticReport contains the comparative telemetry analysis and automated decision.
type CanaryDiagnosticReport struct {
	Timestamp            time.Time        `json:"timestamp"`
	Baseline             TrackMetrics     `json:"baseline"`
	Canary               TrackMetrics     `json:"canary"`
	ErrorRateDelta       float64          `json:"error_rate_delta"`
	LatencyDivergenceP99 float64          `json:"latency_divergence_p99"`
	CanaryTrafficShare   float64          `json:"canary_traffic_share"`
	Verdict              CanaryVerdict    `json:"verdict"`
	Action               CanaryAction     `json:"action"`
	Reason               string           `json:"reason"`
}

type trackRecord struct {
	mu           sync.Mutex
	totalReqs    uint64
	successCount uint64
	clientErrors uint64
	serverErrors uint64
	latencies    []float64
	maxSamples   int
}

func newTrackRecord(maxSamples int) *trackRecord {
	if maxSamples <= 0 {
		maxSamples = 1000
	}
	return &trackRecord{
		latencies:  make([]float64, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

func (tr *trackRecord) record(latencyMs float64, statusCode int) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.totalReqs++
	if statusCode >= 500 {
		tr.serverErrors++
	} else if statusCode >= 400 {
		tr.clientErrors++
	} else {
		tr.successCount++
	}

	if latencyMs >= 0 && !math.IsNaN(latencyMs) && !math.IsInf(latencyMs, 0) {
		if len(tr.latencies) < tr.maxSamples {
			tr.latencies = append(tr.latencies, latencyMs)
		} else {
			// Reservoir sampling to maintain a bounded representative distribution
			idx := tr.totalReqs % uint64(tr.maxSamples)
			tr.latencies[idx] = latencyMs
		}
	}
}

func (tr *trackRecord) compute() TrackMetrics {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	var errRate float64
	if tr.totalReqs > 0 {
		errRate = float64(tr.serverErrors) / float64(tr.totalReqs)
	}

	if len(tr.latencies) == 0 {
		return TrackMetrics{
			TotalRequests: tr.totalReqs,
			SuccessCount:  tr.successCount,
			ClientErrors:  tr.clientErrors,
			ServerErrors:  tr.serverErrors,
			ErrorRate:     math.Round(errRate*10000) / 10000,
			P50Ms:         0,
			P95Ms:         0,
			P99Ms:         0,
		}
	}

	sorted := make([]float64, len(tr.latencies))
	copy(sorted, tr.latencies)
	sort.Float64s(sorted)

	n := len(sorted)
	p50 := percentile(sorted, 0.50, n)
	p95 := percentile(sorted, 0.95, n)
	p99 := percentile(sorted, 0.99, n)

	return TrackMetrics{
		TotalRequests: tr.totalReqs,
		SuccessCount:  tr.successCount,
		ClientErrors:  tr.clientErrors,
		ServerErrors:  tr.serverErrors,
		ErrorRate:     math.Round(errRate*10000) / 10000,
		P50Ms:         math.Round(p50*100) / 100,
		P95Ms:         math.Round(p95*100) / 100,
		P99Ms:         math.Round(p99*100) / 100,
	}
}

func percentile(sorted []float64, p float64, n int) float64 {
	if n == 1 {
		return sorted[0]
	}
	rank := p * float64(n-1)
	low := int(math.Floor(rank))
	high := int(math.Ceil(rank))
	weight := rank - float64(low)
	return sorted[low]*(1.0-weight) + sorted[high]*weight
}

// CanaryTelemetryCollector gathers and analyzes dual-track production metrics during canary deployments.
type CanaryTelemetryCollector struct {
	thresholds CanaryThresholds
	baseline   *trackRecord
	canary     *trackRecord
}

// NewCanaryTelemetryCollector initializes a collector with specified thresholds.
func NewCanaryTelemetryCollector(thresholds CanaryThresholds, maxSamplesPerTrack int) *CanaryTelemetryCollector {
	return &CanaryTelemetryCollector{
		thresholds: thresholds,
		baseline:   newTrackRecord(maxSamplesPerTrack),
		canary:     newTrackRecord(maxSamplesPerTrack),
	}
}

// Record observes an individual request outcome for a route variant.
func (c *CanaryTelemetryCollector) Record(variant RouteVariant, latencyMs float64, statusCode int) error {
	switch variant {
	case VariantBaseline:
		c.baseline.record(latencyMs, statusCode)
	case VariantCanary:
		c.canary.record(latencyMs, statusCode)
	default:
		return errors.New("unrecognized route variant")
	}
	return nil
}

// Diagnose evaluates current telemetry against configured divergence thresholds and outputs an automated report.
func (c *CanaryTelemetryCollector) Diagnose() CanaryDiagnosticReport {
	bMetrics := c.baseline.compute()
	cMetrics := c.canary.compute()

	totalReqs := bMetrics.TotalRequests + cMetrics.TotalRequests
	var trafficShare float64
	if totalReqs > 0 {
		trafficShare = math.Round((float64(cMetrics.TotalRequests)/float64(totalReqs))*10000) / 10000
	}

	report := CanaryDiagnosticReport{
		Timestamp:          time.Now().UTC(),
		Baseline:           bMetrics,
		Canary:             cMetrics,
		CanaryTrafficShare: trafficShare,
	}

	// Check sample sufficiency
	if int(bMetrics.TotalRequests) < c.thresholds.MinSamplesPerTrack ||
		int(cMetrics.TotalRequests) < c.thresholds.MinSamplesPerTrack {
		report.Verdict = VerdictInsufficientSample
		report.Action = ActionHold
		report.Reason = fmt.Sprintf("insufficient samples for evaluation: baseline=%d, canary=%d (required >= %d)",
			bMetrics.TotalRequests, cMetrics.TotalRequests, c.thresholds.MinSamplesPerTrack)
		return report
	}

	// Compute error rate delta
	errDelta := cMetrics.ErrorRate - bMetrics.ErrorRate
	report.ErrorRateDelta = math.Round(errDelta*10000) / 10000

	// Compute latency divergence ratio
	var latDivergence float64
	if bMetrics.P99Ms > 0 {
		latDivergence = cMetrics.P99Ms / bMetrics.P99Ms
	} else if cMetrics.P99Ms > 0 {
		latDivergence = 2.0 // Arbitrary penalty if baseline p99 is 0 but canary is non-zero
	} else {
		latDivergence = 1.0
	}
	report.LatencyDivergenceP99 = math.Round(latDivergence*100) / 100

	// Evaluate degradation rules
	if report.ErrorRateDelta > c.thresholds.MaxErrorRateDelta {
		report.Verdict = VerdictDegradedErrors
		report.Action = ActionAbort
		report.Reason = fmt.Sprintf("canary error rate delta (+%.2f%%) breaches threshold (max +%.2f%%)",
			report.ErrorRateDelta*100.0, c.thresholds.MaxErrorRateDelta*100.0)
		return report
	}

	if report.LatencyDivergenceP99 > c.thresholds.MaxLatencyDivergenceP99 {
		report.Verdict = VerdictDegradedLatency
		report.Action = ActionAbort
		report.Reason = fmt.Sprintf("canary p99 latency divergence (%.2fx) breaches threshold (max %.2fx)",
			report.LatencyDivergenceP99, c.thresholds.MaxLatencyDivergenceP99)
		return report
	}

	// Healthy
	report.Verdict = VerdictHealthy
	report.Action = ActionPromote
	report.Reason = fmt.Sprintf("canary within all SLA bounds: errDelta=%.2f%%, latDivergence=%.2fx",
		report.ErrorRateDelta*100.0, report.LatencyDivergenceP99)
	return report
}
