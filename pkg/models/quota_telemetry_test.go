package models

import (
	"strings"
	"sync"
	"testing"
)

func TestQuotaTelemetryEngine_QuotaConfiguration(t *testing.T) {
	engine := NewQuotaTelemetryEngine()

	// 1. Valid quota
	err := engine.SetTenantQuota(TenantQuota{
		TenantID:         "tenant-alpha",
		MaxDeployments:   10,
		MaxReplicas:      50,
		MaxCPUMillicores: 16000,
		MaxMemoryMB:      32768,
	})
	if err != nil {
		t.Fatalf("expected valid quota setting, got: %v", err)
	}

	// 2. Empty ID error
	err = engine.SetTenantQuota(TenantQuota{
		TenantID:         "",
		MaxDeployments:   10,
		MaxReplicas:      50,
		MaxCPUMillicores: 16000,
		MaxMemoryMB:      32768,
	})
	if err == nil {
		t.Fatal("expected error on empty tenant ID")
	}

	// 3. Non-positive limits
	err = engine.SetTenantQuota(TenantQuota{
		TenantID:         "tenant-beta",
		MaxDeployments:   0,
		MaxReplicas:      50,
		MaxCPUMillicores: 16000,
		MaxMemoryMB:      32768,
	})
	if err == nil {
		t.Fatal("expected error on zero max deployments")
	}
}

func TestQuotaTelemetryEngine_AllocationAndSaturationStates(t *testing.T) {
	engine := NewQuotaTelemetryEngine()
	_ = engine.SetTenantQuota(TenantQuota{
		TenantID:         "tenant-prod",
		MaxDeployments:   10,
		MaxReplicas:      20,
		MaxCPUMillicores: 10000,
		MaxMemoryMB:      20000,
	})

	// Initial state NORMAL (0% utilization)
	diag, err := engine.GetTenantMetrics("tenant-prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diag.SaturationState != SaturationNormal || diag.MaxUtilization != 0.0 {
		t.Fatalf("expected NORMAL at 0, got state=%s maxUtil=%f", diag.SaturationState, diag.MaxUtilization)
	}

	// Allocate 8 deployments (80% -> WARNING)
	err = engine.Allocate("tenant-prod", ResourceAllocationRequest{
		Deployments:   8,
		Replicas:      10,
		CPUMillicores: 5000,
		MemoryMB:      10000,
	})
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}

	diag, _ = engine.GetTenantMetrics("tenant-prod")
	if diag.SaturationState != SaturationWarning {
		t.Fatalf("expected WARNING at 80%%, got: %s", diag.SaturationState)
	}
	if len(diag.Warnings) == 0 {
		t.Fatal("expected diagnostic warning advisory")
	}

	// Allocate 1 more deployment (90% -> CRITICAL)
	err = engine.Allocate("tenant-prod", ResourceAllocationRequest{
		Deployments:   1,
		Replicas:      2,
		CPUMillicores: 1000,
		MemoryMB:      2000,
	})
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}

	diag, _ = engine.GetTenantMetrics("tenant-prod")
	if diag.SaturationState != SaturationCritical {
		t.Fatalf("expected CRITICAL at 90%%, got: %s", diag.SaturationState)
	}

	// Allocate 1 more deployment (100% -> BREACHED)
	err = engine.Allocate("tenant-prod", ResourceAllocationRequest{
		Deployments: 1,
	})
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}

	diag, _ = engine.GetTenantMetrics("tenant-prod")
	if diag.SaturationState != SaturationBreached {
		t.Fatalf("expected BREACHED at 100%%, got: %s", diag.SaturationState)
	}

	// Exceed ceiling -> must fail
	err = engine.Allocate("tenant-prod", ResourceAllocationRequest{
		Deployments: 1,
	})
	if err == nil {
		t.Fatal("expected ResourceQuotaExceeded error when over ceiling")
	}

	// Release 5 deployments -> back to NORMAL
	err = engine.Release("tenant-prod", ResourceAllocationRequest{
		Deployments:   5,
		Replicas:      6,
		CPUMillicores: 3000,
		MemoryMB:      6000,
	})
	if err != nil {
		t.Fatalf("release failed: %v", err)
	}

	diag, _ = engine.GetTenantMetrics("tenant-prod")
	if diag.SaturationState != SaturationNormal {
		t.Fatalf("expected return to NORMAL, got: %s", diag.SaturationState)
	}
}

func TestQuotaTelemetryEngine_PrometheusExport(t *testing.T) {
	engine := NewQuotaTelemetryEngine()
	_ = engine.SetTenantQuota(TenantQuota{
		TenantID:         "tenant-analytics",
		MaxDeployments:   10,
		MaxReplicas:      20,
		MaxCPUMillicores: 8000,
		MaxMemoryMB:      16000,
	})

	_ = engine.Allocate("tenant-analytics", ResourceAllocationRequest{
		Deployments:   5,
		Replicas:      10,
		CPUMillicores: 4000,
		MemoryMB:      8000,
	})

	metrics := engine.ExportPrometheusMetrics()
	if !strings.Contains(metrics, `tenant="tenant-analytics"`) {
		t.Fatalf("expected Prometheus metric to include tenant label, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `resource="deployments"} 0.5000`) {
		t.Fatalf("expected 0.5000 utilization ratio for deployments, got:\n%s", metrics)
	}
}

func TestQuotaTelemetryEngine_ConcurrentAllocateRelease(t *testing.T) {
	engine := NewQuotaTelemetryEngine()
	_ = engine.SetTenantQuota(TenantQuota{
		TenantID:         "tenant-load",
		MaxDeployments:   100,
		MaxReplicas:      200,
		MaxCPUMillicores: 100000,
		MaxMemoryMB:      200000,
	})

	var wg sync.WaitGroup
	workers := 10
	iterations := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				req := ResourceAllocationRequest{
					Deployments:   1,
					Replicas:      1,
					CPUMillicores: 100,
					MemoryMB:      200,
				}
				_ = engine.Allocate("tenant-load", req)
				_ = engine.Release("tenant-load", req)
				_ = engine.GetAllTenantMetrics()
			}
		}()
	}

	wg.Wait()
	diag, err := engine.GetTenantMetrics("tenant-load")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diag.DeploymentUtilization != 0.0 {
		t.Errorf("expected clean 0.0 utilization after balanced releases, got: %f", diag.DeploymentUtilization)
	}
}
