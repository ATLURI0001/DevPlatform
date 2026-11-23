package models

import (
	"sync"
	"testing"
)

func TestCanaryTelemetry_InsufficientSamples(t *testing.T) {
	thresholds := DefaultCanaryThresholds()
	collector := NewCanaryTelemetryCollector(thresholds, 1000)

	// Record only 20 samples per track (threshold is 50)
	for i := 0; i < 20; i++ {
		_ = collector.Record(VariantBaseline, 15.0, 200)
		_ = collector.Record(VariantCanary, 15.0, 200)
	}

	report := collector.Diagnose()
	if report.Verdict != VerdictInsufficientSample {
		t.Errorf("expected VerdictInsufficientSample, got %s", report.Verdict)
	}
	if report.Action != ActionHold {
		t.Errorf("expected ActionHold, got %s", report.Action)
	}
}

func TestCanaryTelemetry_HealthyCanary(t *testing.T) {
	thresholds := DefaultCanaryThresholds()
	collector := NewCanaryTelemetryCollector(thresholds, 1000)

	// Record 100 samples for baseline (mean 20ms, 1 error = 1%)
	for i := 0; i < 100; i++ {
		status := 200
		if i == 0 {
			status = 500
		}
		_ = collector.Record(VariantBaseline, 20.0+float64(i%5), status)
	}

	// Record 100 samples for canary (mean 21ms, 1 error = 1%)
	for i := 0; i < 100; i++ {
		status := 200
		if i == 0 {
			status = 500
		}
		_ = collector.Record(VariantCanary, 21.0+float64(i%5), status)
	}

	report := collector.Diagnose()
	if report.Verdict != VerdictHealthy {
		t.Errorf("expected VerdictHealthy, got %s: %s", report.Verdict, report.Reason)
	}
	if report.Action != ActionPromote {
		t.Errorf("expected ActionPromote, got %s", report.Action)
	}
	if report.CanaryTrafficShare != 0.5 {
		t.Errorf("expected 0.5 traffic share, got %.2f", report.CanaryTrafficShare)
	}
}

func TestCanaryTelemetry_DegradedErrors(t *testing.T) {
	thresholds := DefaultCanaryThresholds() // MaxErrorRateDelta = 0.02
	collector := NewCanaryTelemetryCollector(thresholds, 1000)

	// Baseline: 0 errors out of 100
	for i := 0; i < 100; i++ {
		_ = collector.Record(VariantBaseline, 20.0, 200)
	}

	// Canary: 5 errors out of 100 (5% error rate, delta = +0.05 > 0.02)
	for i := 0; i < 100; i++ {
		status := 200
		if i < 5 {
			status = 503
		}
		_ = collector.Record(VariantCanary, 20.0, status)
	}

	report := collector.Diagnose()
	if report.Verdict != VerdictDegradedErrors {
		t.Errorf("expected VerdictDegradedErrors, got %s", report.Verdict)
	}
	if report.Action != ActionAbort {
		t.Errorf("expected ActionAbort, got %s", report.Action)
	}
	if report.ErrorRateDelta != 0.05 {
		t.Errorf("expected error rate delta 0.05, got %.4f", report.ErrorRateDelta)
	}
}

func TestCanaryTelemetry_DegradedLatency(t *testing.T) {
	thresholds := DefaultCanaryThresholds() // MaxLatencyDivergenceP99 = 1.30 (30% increase)
	collector := NewCanaryTelemetryCollector(thresholds, 1000)

	// Baseline: latency ~20ms, p99 ~24ms
	for i := 0; i < 100; i++ {
		_ = collector.Record(VariantBaseline, 20.0+float64(i%5), 200)
	}

	// Canary: latency ~45ms, p99 ~49ms (divergence > 2.0x)
	for i := 0; i < 100; i++ {
		_ = collector.Record(VariantCanary, 45.0+float64(i%5), 200)
	}

	report := collector.Diagnose()
	if report.Verdict != VerdictDegradedLatency {
		t.Errorf("expected VerdictDegradedLatency, got %s", report.Verdict)
	}
	if report.Action != ActionAbort {
		t.Errorf("expected ActionAbort, got %s", report.Action)
	}
	if report.LatencyDivergenceP99 < 1.8 {
		t.Errorf("expected high latency divergence ratio, got %.2f", report.LatencyDivergenceP99)
	}
}

func TestCanaryTelemetry_Concurrency(t *testing.T) {
	thresholds := DefaultCanaryThresholds()
	collector := NewCanaryTelemetryCollector(thresholds, 2000)

	var wg sync.WaitGroup
	workers := 10
	iterations := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				variant := VariantBaseline
				if workerID%2 == 1 {
					variant = VariantCanary
				}
				_ = collector.Record(variant, float64(10+(j%20)), 200)
			}
		}(i)
	}

	wg.Wait()

	report := collector.Diagnose()
	totalRecorded := report.Baseline.TotalRequests + report.Canary.TotalRequests
	if totalRecorded != uint64(workers*iterations) {
		t.Errorf("expected %d total recorded requests, got %d", workers*iterations, totalRecorded)
	}
}

func TestCanaryTelemetry_InvalidVariant(t *testing.T) {
	collector := NewCanaryTelemetryCollector(DefaultCanaryThresholds(), 100)
	err := collector.Record(RouteVariant("UNKNOWN"), 10.0, 200)
	if err == nil {
		t.Error("expected error on unknown route variant, got nil")
	}
}
