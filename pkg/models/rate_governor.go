package models

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrInvalidTenant   = errors.New("rate_governor: tenant id cannot be empty")
	ErrInvalidTier     = errors.New("rate_governor: unknown or invalid service tier")
	ErrQuotaExceeded   = errors.New("rate_governor: tenant rate quota exceeded")
)

// ServiceTier defines rate-limiting tiers across the platform.
type ServiceTier string

const (
	TierFree            ServiceTier = "free"
	TierDeveloper       ServiceTier = "developer"
	TierEnterprise      ServiceTier = "enterprise"
	TierInternalService ServiceTier = "internal_service"
)

// TierQuota specifies capacity and refill rate for a tier.
type TierQuota struct {
	Capacity     float64       // Max token burst capacity
	RefillPerSec float64       // Tokens added per second
	IdleEvictTTL time.Duration // Time of inactivity before tenant bucket is evicted
}

var DefaultTierQuotas = map[ServiceTier]TierQuota{
	TierFree: {
		Capacity:     20,
		RefillPerSec: 5,
		IdleEvictTTL: 10 * time.Minute,
	},
	TierDeveloper: {
		Capacity:     100,
		RefillPerSec: 25,
		IdleEvictTTL: 30 * time.Minute,
	},
	TierEnterprise: {
		Capacity:     1000,
		RefillPerSec: 250,
		IdleEvictTTL: 1 * time.Hour,
	},
	TierInternalService: {
		Capacity:     5000,
		RefillPerSec: 1000,
		IdleEvictTTL: 2 * time.Hour,
	},
}

type tenantBucket struct {
	tokens     float64
	lastRefill time.Time
	tier       ServiceTier
	quota      TierQuota
}

// RateGovernorTelemetry captures operational metrics.
type RateGovernorTelemetry struct {
	AllowedCount   uint64
	DeniedCount    uint64
	ActiveTenants  int
	EvictedTenants uint64
}

// TenantRateGovernor manages multi-tenant token bucket rate enforcement.
type TenantRateGovernor struct {
	mu            sync.RWMutex
	buckets       map[string]*tenantBucket
	customQuotas  map[ServiceTier]TierQuota
	allowedCount  atomic.Uint64
	deniedCount   atomic.Uint64
	evictedCount  atomic.Uint64
	stopEvictChan chan struct{}
}

// NewTenantRateGovernor instantiates a governor and starts an idle bucket eviction rotary.
func NewTenantRateGovernor(customQuotas map[ServiceTier]TierQuota) *TenantRateGovernor {
	quotas := make(map[ServiceTier]TierQuota)
	for k, v := range DefaultTierQuotas {
		quotas[k] = v
	}
	for k, v := range customQuotas {
		quotas[k] = v
	}

	g := &TenantRateGovernor{
		buckets:       make(map[string]*tenantBucket),
		customQuotas:  quotas,
		stopEvictChan: make(chan struct{}),
	}

	return g
}

// Allow evaluates if a tenant request with token cost is permitted under their tier quota.
func (g *TenantRateGovernor) Allow(tenantID string, tier ServiceTier, tokens float64) (bool, error) {
	if tenantID == "" {
		return false, ErrInvalidTenant
	}
	if tokens <= 0 {
		tokens = 1.0
	}

	quota, ok := g.customQuotas[tier]
	if !ok {
		return false, ErrInvalidTier
	}

	now := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	bucket, exists := g.buckets[tenantID]
	if !exists {
		bucket = &tenantBucket{
			tokens:     quota.Capacity,
			lastRefill: now,
			tier:       tier,
			quota:      quota,
		}
		g.buckets[tenantID] = bucket
	} else if bucket.tier != tier {
		// Tier upgrade or downgrade
		bucket.tier = tier
		bucket.quota = quota
		if bucket.tokens > quota.Capacity {
			bucket.tokens = quota.Capacity
		}
	}

	// Refill tokens
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	if elapsed > 0 {
		added := elapsed * bucket.quota.RefillPerSec
		bucket.tokens = math.Min(bucket.quota.Capacity, bucket.tokens+added)
		bucket.lastRefill = now
	}

	// Attempt consumption
	if bucket.tokens >= tokens {
		bucket.tokens -= tokens
		g.allowedCount.Add(1)
		return true, nil
	}

	g.deniedCount.Add(1)
	return false, nil
}

// EvictIdleBuckets purges buckets that have exceeded their tier idle eviction TTL.
func (g *TenantRateGovernor) EvictIdleBuckets(now time.Time) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	evicted := 0
	for id, bucket := range g.buckets {
		if now.Sub(bucket.lastRefill) > bucket.quota.IdleEvictTTL {
			delete(g.buckets, id)
			evicted++
		}
	}

	g.evictedCount.Add(uint64(evicted))
	return evicted
}

// GetTelemetry reports current diagnostic telemetry.
func (g *TenantRateGovernor) GetTelemetry() RateGovernorTelemetry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return RateGovernorTelemetry{
		AllowedCount:   g.allowedCount.Load(),
		DeniedCount:    g.deniedCount.Load(),
		ActiveTenants:  len(g.buckets),
		EvictedTenants: g.evictedCount.Load(),
	}
}

// Reset clears state for testing or system restarts.
func (g *TenantRateGovernor) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.buckets = make(map[string]*tenantBucket)
	g.allowedCount.Store(0)
	g.deniedCount.Store(0)
	g.evictedCount.Store(0)
}
