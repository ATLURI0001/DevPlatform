package models

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SaturationState levels for tenant resource utilization.
type SaturationState string

const (
	SaturationNormal   SaturationState = "NORMAL"
	SaturationWarning  SaturationState = "WARNING"
	SaturationCritical SaturationState = "CRITICAL"
	SaturationBreached SaturationState = "BREACHED"
)

var (
	ErrTenantNotFound         = errors.New("tenant quota profile not found")
	ErrResourceQuotaExceeded  = errors.New("resource allocation exceeds tenant quota ceiling")
	ErrInvalidQuotaConfig     = errors.New("invalid tenant quota configuration")
	ErrInvalidAllocation      = errors.New("invalid resource allocation parameters")
)

// TenantQuota defines the upper limits allocated to a specific tenant.
type TenantQuota struct {
	TenantID        string `json:"tenant_id"`
	MaxDeployments  int    `json:"max_deployments"`
	MaxReplicas     int    `json:"max_replicas"`
	MaxCPUMillicores int   `json:"max_cpu_millicores"`
	MaxMemoryMB     int    `json:"max_memory_mb"`
}

// TenantUsage tracks currently provisioned resources for a tenant.
type TenantUsage struct {
	ActiveDeployments   int       `json:"active_deployments"`
	ActiveReplicas      int       `json:"active_replicas"`
	AllocatedCPUMillicores int    `json:"allocated_cpu_millicores"`
	AllocatedMemoryMB   int       `json:"allocated_memory_mb"`
	LastAllocatedAt     time.Time `json:"last_allocated_at"`
}

// ResourceAllocationRequest defines the capacity requested by a deployment.
type ResourceAllocationRequest struct {
	Deployments   int `json:"deployments"`
	Replicas      int `json:"replicas"`
	CPUMillicores int `json:"cpu_millicores"`
	MemoryMB      int `json:"memory_mb"`
}

// TenantSaturationDiagnostic provides telemetry and diagnostic status.
type TenantSaturationDiagnostic struct {
	TenantID              string          `json:"tenant_id"`
	DeploymentUtilization float64         `json:"deployment_utilization"`
	ReplicaUtilization    float64         `json:"replica_utilization"`
	CPUUtilization        float64         `json:"cpu_utilization"`
	MemoryUtilization     float64         `json:"memory_utilization"`
	MaxUtilization        float64         `json:"max_utilization"`
	SaturationState       SaturationState `json:"saturation_state"`
	Warnings              []string        `json:"warnings,omitempty"`
}

// QuotaTelemetryEngine manages tenant quota enforcement, utilization tracking, and diagnostics.
type QuotaTelemetryEngine struct {
	mu      sync.RWMutex
	quotas  map[string]TenantQuota
	usages  map[string]*TenantUsage
	history []string
}

// NewQuotaTelemetryEngine initializes an empty thread-safe quota telemetry engine.
func NewQuotaTelemetryEngine() *QuotaTelemetryEngine {
	return &QuotaTelemetryEngine{
		quotas: make(map[string]TenantQuota),
		usages: make(map[string]*TenantUsage),
	}
}

// SetTenantQuota establishes or updates quota limits for a tenant.
func (e *QuotaTelemetryEngine) SetTenantQuota(quota TenantQuota) error {
	trimmedID := strings.TrimSpace(quota.TenantID)
	if trimmedID == "" {
		return fmt.Errorf("%w: tenant ID must not be empty", ErrInvalidQuotaConfig)
	}
	if quota.MaxDeployments <= 0 || quota.MaxReplicas <= 0 || quota.MaxCPUMillicores <= 0 || quota.MaxMemoryMB <= 0 {
		return fmt.Errorf("%w: quota limits must be strictly positive", ErrInvalidQuotaConfig)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	quota.TenantID = trimmedID
	e.quotas[trimmedID] = quota
	if _, exists := e.usages[trimmedID]; !exists {
		e.usages[trimmedID] = &TenantUsage{LastAllocatedAt: time.Now().UTC()}
	}

	return nil
}

// Allocate attempts to increment tenant resource usage if within quota bounds.
func (e *QuotaTelemetryEngine) Allocate(tenantID string, req ResourceAllocationRequest) error {
	trimmedID := strings.TrimSpace(tenantID)
	if trimmedID == "" {
		return ErrTenantNotFound
	}
	if req.Deployments < 0 || req.Replicas < 0 || req.CPUMillicores < 0 || req.MemoryMB < 0 {
		return ErrInvalidAllocation
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	quota, ok := e.quotas[trimmedID]
	if !ok {
		return ErrTenantNotFound
	}
	usage := e.usages[trimmedID]

	newDeployments := usage.ActiveDeployments + req.Deployments
	newReplicas := usage.ActiveReplicas + req.Replicas
	newCPU := usage.AllocatedCPUMillicores + req.CPUMillicores
	newMem := usage.AllocatedMemoryMB + req.MemoryMB

	if newDeployments > quota.MaxDeployments {
		return fmt.Errorf("%w: deployments (%d > %d)", ErrResourceQuotaExceeded, newDeployments, quota.MaxDeployments)
	}
	if newReplicas > quota.MaxReplicas {
		return fmt.Errorf("%w: replicas (%d > %d)", ErrResourceQuotaExceeded, newReplicas, quota.MaxReplicas)
	}
	if newCPU > quota.MaxCPUMillicores {
		return fmt.Errorf("%w: cpu (%dm > %dm)", ErrResourceQuotaExceeded, newCPU, quota.MaxCPUMillicores)
	}
	if newMem > quota.MaxMemoryMB {
		return fmt.Errorf("%w: memory (%dMB > %dMB)", ErrResourceQuotaExceeded, newMem, quota.MaxMemoryMB)
	}

	usage.ActiveDeployments = newDeployments
	usage.ActiveReplicas = newReplicas
	usage.AllocatedCPUMillicores = newCPU
	usage.AllocatedMemoryMB = newMem
	usage.LastAllocatedAt = time.Now().UTC()

	return nil
}

// Release decrements resource usage when deployments are terminated.
func (e *QuotaTelemetryEngine) Release(tenantID string, req ResourceAllocationRequest) error {
	trimmedID := strings.TrimSpace(tenantID)
	if trimmedID == "" {
		return ErrTenantNotFound
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	usage, ok := e.usages[trimmedID]
	if !ok {
		return ErrTenantNotFound
	}

	usage.ActiveDeployments = maxInt(0, usage.ActiveDeployments-req.Deployments)
	usage.ActiveReplicas = maxInt(0, usage.ActiveReplicas-req.Replicas)
	usage.AllocatedCPUMillicores = maxInt(0, usage.AllocatedCPUMillicores-req.CPUMillicores)
	usage.AllocatedMemoryMB = maxInt(0, usage.AllocatedMemoryMB-req.MemoryMB)

	return nil
}

// GetTenantMetrics computes the saturation diagnostic for a given tenant.
func (e *QuotaTelemetryEngine) GetTenantMetrics(tenantID string) (TenantSaturationDiagnostic, error) {
	trimmedID := strings.TrimSpace(tenantID)
	e.mu.RLock()
	defer e.mu.RUnlock()

	quota, ok := e.quotas[trimmedID]
	if !ok {
		return TenantSaturationDiagnostic{}, ErrTenantNotFound
	}
	usage := e.usages[trimmedID]

	return computeDiagnostic(quota, usage), nil
}

// GetAllTenantMetrics gathers diagnostics across all configured tenants.
func (e *QuotaTelemetryEngine) GetAllTenantMetrics() []TenantSaturationDiagnostic {
	e.mu.RLock()
	defer e.mu.RUnlock()

	diagnostics := make([]TenantSaturationDiagnostic, 0, len(e.quotas))
	for id, quota := range e.quotas {
		usage := e.usages[id]
		diagnostics = append(diagnostics, computeDiagnostic(quota, usage))
	}
	return diagnostics
}

// ExportPrometheusMetrics renders Prometheus-compatible metrics text.
func (e *QuotaTelemetryEngine) ExportPrometheusMetrics() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("# HELP tenant_quota_utilization_ratio Current tenant quota utilization ratio (0.0 to 1.0)\n")
	sb.WriteString("# TYPE tenant_quota_utilization_ratio gauge\n")

	for id, quota := range e.quotas {
		usage := e.usages[id]
		diag := computeDiagnostic(quota, usage)
		sb.WriteString(fmt.Sprintf("tenant_quota_utilization_ratio{tenant=\"%s\",resource=\"deployments\"} %.4f\n", id, diag.DeploymentUtilization))
		sb.WriteString(fmt.Sprintf("tenant_quota_utilization_ratio{tenant=\"%s\",resource=\"replicas\"} %.4f\n", id, diag.ReplicaUtilization))
		sb.WriteString(fmt.Sprintf("tenant_quota_utilization_ratio{tenant=\"%s\",resource=\"cpu\"} %.4f\n", id, diag.CPUUtilization))
		sb.WriteString(fmt.Sprintf("tenant_quota_utilization_ratio{tenant=\"%s\",resource=\"memory\"} %.4f\n", id, diag.MemoryUtilization))
	}

	return sb.String()
}

func computeDiagnostic(quota TenantQuota, usage *TenantUsage) TenantSaturationDiagnostic {
	depUtil := clampRatio(float64(usage.ActiveDeployments) / float64(quota.MaxDeployments))
	repUtil := clampRatio(float64(usage.ActiveReplicas) / float64(quota.MaxReplicas))
	cpuUtil := clampRatio(float64(usage.AllocatedCPUMillicores) / float64(quota.MaxCPUMillicores))
	memUtil := clampRatio(float64(usage.AllocatedMemoryMB) / float64(quota.MaxMemoryMB))

	maxUtil := depUtil
	if repUtil > maxUtil {
		maxUtil = repUtil
	}
	if cpuUtil > maxUtil {
		maxUtil = cpuUtil
	}
	if memUtil > maxUtil {
		maxUtil = memUtil
	}

	var state SaturationState
	var warnings []string

	if maxUtil >= 1.0 {
		state = SaturationBreached
		warnings = append(warnings, "Tenant has fully exhausted one or more allocated resource quotas")
	} else if maxUtil >= 0.90 {
		state = SaturationCritical
		warnings = append(warnings, "Tenant resource utilization exceeds 90% critical threshold")
	} else if maxUtil >= 0.75 {
		state = SaturationWarning
		warnings = append(warnings, "Tenant resource utilization is elevated above 75% warning threshold")
	} else {
		state = SaturationNormal
	}

	return TenantSaturationDiagnostic{
		TenantID:              quota.TenantID,
		DeploymentUtilization: depUtil,
		ReplicaUtilization:    repUtil,
		CPUUtilization:        cpuUtil,
		MemoryUtilization:     memUtil,
		MaxUtilization:        maxUtil,
		SaturationState:       state,
		Warnings:              warnings,
	}
}

func clampRatio(v float64) float64 {
	if v < 0 {
		return 0.0
	}
	return float64(int(v*10000)) / 10000.0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
