package models

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// CacheStatus describes the artifact cache lookup result.
type CacheStatus string

const (
	CacheHit    CacheStatus = "HIT"
	CacheMiss   CacheStatus = "MISS"
	CacheBypass CacheStatus = "BYPASS"
)

// ArtifactTelemetryConfig configures SLA targets and storage thresholds.
type ArtifactTelemetryConfig struct {
	WindowDuration     time.Duration `json:"window_duration"`
	MinCacheHitRatio   float64       `json:"min_cache_hit_ratio"`   // e.g. 0.75 -> 75% target
	MaxP99BuildDuration time.Duration `json:"max_p99_build_duration"` // e.g. 3m SLA
	MaxTrackedBuilds   int           `json:"max_tracked_builds"`
}

// DefaultArtifactTelemetryConfig provides production default bounds.
func DefaultArtifactTelemetryConfig() ArtifactTelemetryConfig {
	return ArtifactTelemetryConfig{
		WindowDuration:      24 * time.Hour,
		MinCacheHitRatio:    0.75,
		MaxP99BuildDuration: 3 * time.Minute,
		MaxTrackedBuilds:    2000,
	}
}

// BuildRecord captures execution stats for a completed CI/CD build run.
type BuildRecord struct {
	BuildID         string        `json:"build_id"`
	ProjectID       string        `json:"project_id"`
	TotalDuration   time.Duration `json:"total_duration"`
	CompileDuration time.Duration `json:"compile_duration"`
	TestDuration    time.Duration `json:"test_duration"`
	CacheStatus     CacheStatus   `json:"cache_status"`
	BytesSaved      int64         `json:"bytes_saved"`
	BytesDownloaded int64         `json:"bytes_downloaded"`
	Success         bool          `json:"success"`
	Timestamp       time.Time     `json:"timestamp"`
}

// ArtifactTelemetrySnapshot represents aggregated telemetry metrics over the active window.
type ArtifactTelemetrySnapshot struct {
	TotalBuilds        int           `json:"total_builds"`
	SuccessfulBuilds   int           `json:"successful_builds"`
	FailedBuilds       int           `json:"failed_builds"`
	CacheHitCount      int           `json:"cache_hit_count"`
	CacheMissCount     int           `json:"cache_miss_count"`
	CacheBypassCount   int           `json:"cache_bypass_count"`
	CacheHitRatio      float64       `json:"cache_hit_ratio"`
	TotalBytesSaved    int64         `json:"total_bytes_saved"`
	TotalBytesDownload int64         `json:"total_bytes_download"`
	ByteSavingsRatio   float64       `json:"byte_savings_ratio"`
	P50Duration        time.Duration `json:"p50_duration"`
	P90Duration        time.Duration `json:"p90_duration"`
	P99Duration        time.Duration `json:"p99_duration"`
	MeanDuration       time.Duration `json:"mean_duration"`
	SLABreached        bool          `json:"sla_breached"`
	BreachReason       string        `json:"breach_reason,omitempty"`
}

// ArtifactTelemetryEngine aggregates build performance and caching metrics.
type ArtifactTelemetryEngine struct {
	mu     sync.RWMutex
	config ArtifactTelemetryConfig
	builds []BuildRecord
}

// NewArtifactTelemetryEngine constructs a new telemetry engine.
func NewArtifactTelemetryEngine(cfg ArtifactTelemetryConfig) *ArtifactTelemetryEngine {
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = 24 * time.Hour
	}
	if cfg.MinCacheHitRatio <= 0.0 || cfg.MinCacheHitRatio > 1.0 {
		cfg.MinCacheHitRatio = 0.75
	}
	if cfg.MaxP99BuildDuration <= 0 {
		cfg.MaxP99BuildDuration = 3 * time.Minute
	}
	if cfg.MaxTrackedBuilds <= 0 {
		cfg.MaxTrackedBuilds = 2000
	}

	return &ArtifactTelemetryEngine{
		config: cfg,
		builds: make([]BuildRecord, 0, 128),
	}
}

// RecordBuild registers a completed build execution.
func (e *ArtifactTelemetryEngine) RecordBuild(record BuildRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	e.pruneOldBuildsLocked(record.Timestamp)

	e.builds = append(e.builds, record)
	if len(e.builds) > e.config.MaxTrackedBuilds {
		e.builds = e.builds[len(e.builds)-e.config.MaxTrackedBuilds:]
	}
}

// Snapshot aggregates build telemetry over the current window.
func (e *ArtifactTelemetryEngine) Snapshot() ArtifactTelemetrySnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	e.pruneOldBuildsLocked(now)

	n := len(e.builds)
	if n == 0 {
		return ArtifactTelemetrySnapshot{}
	}

	var successful, failed, hits, misses, bypasses int
	var bytesSaved, bytesDownload int64
	var sumDuration time.Duration
	durations := make([]time.Duration, 0, n)

	for _, b := range e.builds {
		if b.Success {
			successful++
		} else {
			failed++
		}

		switch b.CacheStatus {
		case CacheHit:
			hits++
		case CacheMiss:
			misses++
		default:
			bypasses++
		}

		bytesSaved += b.BytesSaved
		bytesDownload += b.BytesDownloaded
		sumDuration += b.TotalDuration
		durations = append(durations, b.TotalDuration)
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	var hitRatio float64
	cacheLookups := hits + misses
	if cacheLookups > 0 {
		hitRatio = float64(hits) / float64(cacheLookups)
	}

	var byteRatio float64
	totalBytes := bytesSaved + bytesDownload
	if totalBytes > 0 {
		byteRatio = float64(bytesSaved) / float64(totalBytes)
	}

	p50 := durations[n*50/100]
	p90 := durations[n*90/100]
	p99 := durations[n*99/100]
	mean := sumDuration / time.Duration(n)

	slaBreached := false
	var breachReasons []string

	if cacheLookups >= 5 && hitRatio < e.config.MinCacheHitRatio {
		slaBreached = true
		breachReasons = append(breachReasons, fmt.Sprintf("Cache hit ratio %.1f%% below target %.1f%%", hitRatio*100.0, e.config.MinCacheHitRatio*100.0))
	}

	if p99 > e.config.MaxP99BuildDuration {
		slaBreached = true
		breachReasons = append(breachReasons, fmt.Sprintf("P99 build latency %v exceeded SLA threshold %v", p99, e.config.MaxP99BuildDuration))
	}

	return ArtifactTelemetrySnapshot{
		TotalBuilds:        n,
		SuccessfulBuilds:   successful,
		FailedBuilds:       failed,
		CacheHitCount:      hits,
		CacheMissCount:     misses,
		CacheBypassCount:   bypasses,
		CacheHitRatio:      hitRatio,
		TotalBytesSaved:    bytesSaved,
		TotalBytesDownload: bytesDownload,
		ByteSavingsRatio:   byteRatio,
		P50Duration:        p50,
		P90Duration:        p90,
		P99Duration:        p99,
		MeanDuration:       mean,
		SLABreached:        slaBreached,
		BreachReason:       strings.Join(breachReasons, "; "),
	}
}

// Reset purges all historical build records.
func (e *ArtifactTelemetryEngine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.builds = e.builds[:0]
}

func (e *ArtifactTelemetryEngine) pruneOldBuildsLocked(now time.Time) {
	cutoff := now.Add(-e.config.WindowDuration)
	validIdx := 0
	for validIdx < len(e.builds) && e.builds[validIdx].Timestamp.Before(cutoff) {
		validIdx++
	}
	if validIdx > 0 {
		e.builds = append(e.builds[:0], e.builds[validIdx:]...)
	}
}
