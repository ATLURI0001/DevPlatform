package models

import (
	"errors"
	"testing"
	"time"
)

func TestTenantRateGovernor_BasicAllowanceAndDepletion(t *testing.T) {
	g := NewTenantRateGovernor(nil)

	// Free tier has capacity 20, refill 5/sec
	for i := 0; i < 20; i++ {
		allowed, err := g.Allow("tenant-free-1", TierFree, 1.0)
		if err != nil || !allowed {
			t.Fatalf("expected allowed for iteration %d, got allowed=%v, err=%v", i, allowed, err)
		}
	}

	// 21st request should be denied
	allowed, err := g.Allow("tenant-free-1", TierFree, 1.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatalf("expected 21st request to be denied due to token exhaustion")
	}

	telemetry := g.GetTelemetry()
	if telemetry.AllowedCount != 20 {
		t.Errorf("expected 20 allowed, got %d", telemetry.AllowedCount)
	}
	if telemetry.DeniedCount != 1 {
		t.Errorf("expected 1 denied, got %d", telemetry.DeniedCount)
	}
	if telemetry.ActiveTenants != 1 {
		t.Errorf("expected 1 active tenant, got %d", telemetry.ActiveTenants)
	}
}

func TestTenantRateGovernor_Refill(t *testing.T) {
	customQuotas := map[ServiceTier]TierQuota{
		TierFree: {
			Capacity:     5,
			RefillPerSec: 50, // fast refill for testing
			IdleEvictTTL: 1 * time.Minute,
		},
	}
	g := NewTenantRateGovernor(customQuotas)

	// Consume all 5 tokens
	for i := 0; i < 5; i++ {
		allowed, _ := g.Allow("tenant-fast", TierFree, 1.0)
		if !allowed {
			t.Fatalf("expected allowed at %d", i)
		}
	}

	// Should be exhausted
	allowed, _ := g.Allow("tenant-fast", TierFree, 1.0)
	if allowed {
		t.Fatalf("expected exhaustion")
	}

	// Sleep 50ms (refill 50 * 0.05 = 2.5 tokens)
	time.Sleep(50 * time.Millisecond)

	allowed, _ = g.Allow("tenant-fast", TierFree, 2.0)
	if !allowed {
		t.Fatalf("expected allowed after token refill")
	}
}

func TestTenantRateGovernor_Validation(t *testing.T) {
	g := NewTenantRateGovernor(nil)

	_, err := g.Allow("", TierFree, 1.0)
	if !errors.Is(err, ErrInvalidTenant) {
		t.Errorf("expected ErrInvalidTenant, got %v", err)
	}

	_, err = g.Allow("valid-tenant", "unknown-tier", 1.0)
	if !errors.Is(err, ErrInvalidTier) {
		t.Errorf("expected ErrInvalidTier, got %v", err)
	}
}

func TestTenantRateGovernor_IdleEviction(t *testing.T) {
	customQuotas := map[ServiceTier]TierQuota{
		TierFree: {
			Capacity:     10,
			RefillPerSec: 1,
			IdleEvictTTL: 10 * time.Millisecond,
		},
	}
	g := NewTenantRateGovernor(customQuotas)

	_, _ = g.Allow("tenant-temp", TierFree, 1.0)
	if g.GetTelemetry().ActiveTenants != 1 {
		t.Fatalf("expected 1 active tenant")
	}

	// Wait past TTL
	time.Sleep(20 * time.Millisecond)

	evicted := g.EvictIdleBuckets(time.Now())
	if evicted != 1 {
		t.Fatalf("expected 1 evicted tenant, got %d", evicted)
	}
	if g.GetTelemetry().ActiveTenants != 0 {
		t.Fatalf("expected 0 active tenants after eviction")
	}
}
