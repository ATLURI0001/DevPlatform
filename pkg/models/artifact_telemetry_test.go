package models

import (
	"sync"
	"testing"
	"time"
)

func TestArtifactTelemetryEngine_NominalTracking(t *testing.T) {
	cfg := DefaultArtifactTelemetryConfig()
	cfg.MinCacheHitRatio = 0.80
	cfg.MaxP99BuildDuration = 2 * time.Minute
	engine := NewArtifactTelemetryEngine(cfg)

	// Record 10 builds: 8 CacheHit, 2 CacheMiss
	for i := 0; i < 8; i++ {
		engine.RecordBuild(BuildRecord{
			BuildID:         "build-hit",
			ProjectID:       "proj-1",
			TotalDuration:   30 * time.Second,
			CompileDuration: 10 * time.Second,
			TestDuration:    15 * time.Second,
			CacheStatus:     CacheHit,
			BytesSaved:      50000000,
			BytesDownloaded: 1000000,
			Success:         true,
			Timestamp:       time.Now(),
		})
	}
	for i := 0; i < 2; i++ {
		engine.RecordBuild(BuildRecord{
			BuildID:         "build-miss",
			ProjectID:       "proj-1",
			TotalDuration:   60 * time.Second,
			CompileDuration: 40 * time.Second,
			TestDuration:    15 * time.Second,
			CacheStatus:     CacheMiss,
			BytesSaved:      0,
			BytesDownloaded: 50000000,
			Success:         true,
			Timestamp:       time.Now(),
		})
	}

	snap := engine.Snapshot()
	if snap.TotalBuilds != 10 {
		t.Fatalf("expected 10 builds, got %d", snap.TotalBuilds)
	}
	if snap.CacheHitCount != 8 || snap.CacheMissCount != 2 {
		t.Fatalf("unexpected cache counts: hits=%d, misses=%d", snap.CacheHitCount, snap.CacheMissCount)
	}
	if snap.CacheHitRatio != 0.80 {
		t.Fatalf("expected 80%% hit ratio, got %.2f", snap.CacheHitRatio)
	}
	if snap.SLABreached {
		t.Fatalf("expected no SLA breach, got reason: %s", snap.BreachReason)
	}
}

func TestArtifactTelemetryEngine_SLABreachTriggers(t *testing.T) {
	cfg := DefaultArtifactTelemetryConfig()
	cfg.MinCacheHitRatio = 0.70
	cfg.MaxP99BuildDuration = 60 * time.Second
	engine := NewArtifactTelemetryEngine(cfg)

	// 5 builds with low cache hit ratio (1 hit, 4 miss = 20% < 70%)
	engine.RecordBuild(BuildRecord{BuildID: "b1", CacheStatus: CacheHit, TotalDuration: 10 * time.Second, Success: true})
	for i := 0; i < 4; i++ {
		engine.RecordBuild(BuildRecord{BuildID: "b-miss", CacheStatus: CacheMiss, TotalDuration: 20 * time.Second, Success: true})
	}

	snap := engine.Snapshot()
	if !snap.SLABreached {
		t.Fatalf("expected SLA breach due to cache hit ratio < 70%%")
	}

	// Now test latency SLA breach
	engine.Reset()
	for i := 0; i < 9; i++ {
		engine.RecordBuild(BuildRecord{BuildID: "fast", CacheStatus: CacheHit, TotalDuration: 10 * time.Second, Success: true})
	}
	// 10th build takes 120s > 60s max P99
	engine.RecordBuild(BuildRecord{BuildID: "slow", CacheStatus: CacheHit, TotalDuration: 120 * time.Second, Success: true})

	snapLatency := engine.Snapshot()
	if !snapLatency.SLABreached {
		t.Fatalf("expected SLA breach due to P99 build duration exceeding threshold")
	}
}

func TestArtifactTelemetryEngine_Concurrency(t *testing.T) {
	cfg := DefaultArtifactTelemetryConfig()
	engine := NewArtifactTelemetryEngine(cfg)

	var wg sync.WaitGroup
	workers := 8
	iterations := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				engine.RecordBuild(BuildRecord{
					BuildID:       "b-concurrent",
					CacheStatus:   CacheHit,
					TotalDuration: 15 * time.Second,
					Success:       true,
				})
				_ = engine.Snapshot()
			}
		}(w)
	}

	wg.Wait()
	snap := engine.Snapshot()
	expected := workers * iterations
	if snap.TotalBuilds != expected {
		t.Fatalf("expected %d builds, got %d", expected, snap.TotalBuilds)
	}
}
