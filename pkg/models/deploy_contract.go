package models

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrInvalidServiceName    = errors.New("invalid service name: must conform to RFC 1123 DNS subdomain (1-63 chars, lowercase alphanumeric or hyphens)")
	ErrInvalidEnvironment    = errors.New("invalid environment: must be development, staging, canary, production, or dr")
	ErrInvalidArtifactHash   = errors.New("invalid artifact hash: must follow 'sha256:<64-hex-digits>' format")
	ErrInvalidReplicaCount   = errors.New("invalid replica count: must be between 1 and 1000")
	ErrInvalidCPUResource    = errors.New("invalid CPU resource notation: must be integer cores (e.g. '2') or millicores (e.g. '500m')")
	ErrInvalidMemoryResource = errors.New("invalid memory resource notation: must specify units Mi, Gi, or Ti (e.g. '512Mi', '2Gi')")
	ErrInvalidProbePath      = errors.New("invalid probe path: must start with '/' without whitespace, query params, or control characters")
	ErrInvalidProbePort      = errors.New("invalid probe port: must be between 1 and 65535")
	ErrInvalidIngressHost    = errors.New("invalid ingress host: malformed FQDN format")
)

var (
	deployServiceNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	artifactDigestRegex    = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	cpuMillicoresRegex     = regexp.MustCompile(`^([1-9][0-9]*)m$`)
	cpuCoresRegex          = regexp.MustCompile(`^([1-9][0-9]*)$`)
	memoryRegex            = regexp.MustCompile(`^([1-9][0-9]*)(Mi|Gi|Ti)$`)
	probePathRegex         = regexp.MustCompile(`^/[a-zA-Z0-9_\-\./]*$`)
	ingressHostRegex       = regexp.MustCompile(`^([a-z0-9]([-a-z0-9]*[a-z0-9])?\.)+[a-z]{2,}$`)
)

type DeploymentEnvironment string

const (
	EnvDevelopment DeploymentEnvironment = "development"
	EnvStaging     DeploymentEnvironment = "staging"
	EnvCanary      DeploymentEnvironment = "canary"
	EnvProduction  DeploymentEnvironment = "production"
	EnvDR          DeploymentEnvironment = "dr"
)

// ResourceRequirements defines bounded CPU and Memory configurations.
type ResourceRequirements struct {
	CPURequest    string `json:"cpu_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryRequest string `json:"memory_request"`
	MemoryLimit   string `json:"memory_limit"`
}

// HealthProbeConfig defines container liveness/readiness specifications.
type HealthProbeConfig struct {
	Path                string `json:"path"`
	Port                int    `json:"port"`
	InitialDelaySeconds int    `json:"initial_delay_seconds"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
	PeriodSeconds       int    `json:"period_seconds"`
}

// DeploymentSpec encapsulates deployment manifest inputs before validation.
type DeploymentSpec struct {
	ServiceName string                `json:"service_name"`
	Environment DeploymentEnvironment `json:"environment"`
	Artifact    string                `json:"artifact"`
	Replicas    int                   `json:"replicas"`
	Resources   ResourceRequirements  `json:"resources"`
	HealthProbe HealthProbeConfig     `json:"health_probe"`
	IngressHost string                `json:"ingress_host,omitempty"`
}

// SanitizedDeploymentSpec encapsulates normalized, verified deployment parameters.
type SanitizedDeploymentSpec struct {
	ServiceName        string                `json:"service_name"`
	Environment        DeploymentEnvironment `json:"environment"`
	NormalizedArtifact string                `json:"normalized_artifact"`
	Replicas           int                   `json:"replicas"`
	CPURequestMillis   int64                 `json:"cpu_request_millis"`
	CPULimitMillis     int64                 `json:"cpu_limit_millis"`
	MemoryRequestMB    int64                 `json:"memory_request_mb"`
	MemoryLimitMB      int64                 `json:"memory_limit_mb"`
	ProbePath          string                `json:"probe_path"`
	ProbePort          int                   `json:"probe_port"`
	IngressHost        string                `json:"ingress_host,omitempty"`
}

// ParseCPUToMillis converts CPU string notation (e.g. "500m" or "2") to millicores.
func ParseCPUToMillis(cpu string) (int64, error) {
	clean := strings.TrimSpace(cpu)
	if m := cpuMillicoresRegex.FindStringSubmatch(clean); len(m) == 2 {
		val, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || val <= 0 {
			return 0, ErrInvalidCPUResource
		}
		return val, nil
	}
	if m := cpuCoresRegex.FindStringSubmatch(clean); len(m) == 2 {
		val, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || val <= 0 {
			return 0, ErrInvalidCPUResource
		}
		return val * 1000, nil
	}
	return 0, ErrInvalidCPUResource
}

// ParseMemoryToMB converts memory string notation (e.g. "512Mi", "2Gi") to Megabytes.
func ParseMemoryToMB(mem string) (int64, error) {
	clean := strings.TrimSpace(mem)
	m := memoryRegex.FindStringSubmatch(clean)
	if len(m) != 3 {
		return 0, ErrInvalidMemoryResource
	}

	val, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || val <= 0 {
		return 0, ErrInvalidMemoryResource
	}

	unit := m[2]
	switch unit {
	case "Mi":
		return val, nil
	case "Gi":
		return val * 1024, nil
	case "Ti":
		return val * 1024 * 1024, nil
	default:
		return 0, ErrInvalidMemoryResource
	}
}

// ValidateAndSanitizeDeploymentSpec applies ingress contracts and bounds checks.
func ValidateAndSanitizeDeploymentSpec(spec DeploymentSpec) (*SanitizedDeploymentSpec, error) {
	// Service name
	sName := strings.TrimSpace(spec.ServiceName)
	if len(sName) == 0 || len(sName) > 63 || !deployServiceNameRegex.MatchString(sName) {
		return nil, ErrInvalidServiceName
	}

	// Environment
	env := DeploymentEnvironment(strings.ToLower(strings.TrimSpace(string(spec.Environment))))
	switch env {
	case EnvDevelopment, EnvStaging, EnvCanary, EnvProduction, EnvDR:
		// valid
	default:
		return nil, ErrInvalidEnvironment
	}

	// Artifact hash
	artifact := strings.ToLower(strings.TrimSpace(spec.Artifact))
	if !artifactDigestRegex.MatchString(artifact) {
		return nil, ErrInvalidArtifactHash
	}

	// Replicas
	if spec.Replicas < 1 || spec.Replicas > 1000 {
		return nil, ErrInvalidReplicaCount
	}

	// CPU resources
	cpuReq, err := ParseCPUToMillis(spec.Resources.CPURequest)
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_request: %w", err)
	}
	cpuLim, err := ParseCPUToMillis(spec.Resources.CPULimit)
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_limit: %w", err)
	}
	if cpuReq > cpuLim {
		return nil, fmt.Errorf("%w: cpu_request (%dm) exceeds cpu_limit (%dm)", ErrInvalidCPUResource, cpuReq, cpuLim)
	}

	// Memory resources
	memReq, err := ParseMemoryToMB(spec.Resources.MemoryRequest)
	if err != nil {
		return nil, fmt.Errorf("invalid memory_request: %w", err)
	}
	memLim, err := ParseMemoryToMB(spec.Resources.MemoryLimit)
	if err != nil {
		return nil, fmt.Errorf("invalid memory_limit: %w", err)
	}
	if memReq > memLim {
		return nil, fmt.Errorf("%w: memory_request (%dMB) exceeds memory_limit (%dMB)", ErrInvalidMemoryResource, memReq, memLim)
	}

	// Health probe
	probePath := strings.TrimSpace(spec.HealthProbe.Path)
	if len(probePath) == 0 || len(probePath) > 256 || !strings.HasPrefix(probePath, "/") || !probePathRegex.MatchString(probePath) {
		return nil, ErrInvalidProbePath
	}
	if spec.HealthProbe.Port < 1 || spec.HealthProbe.Port > 65535 {
		return nil, ErrInvalidProbePort
	}

	// Ingress host (optional)
	var ingressHost string
	if spec.IngressHost != "" {
		ingressHost = strings.ToLower(strings.TrimSpace(spec.IngressHost))
		if len(ingressHost) > 253 || !ingressHostRegex.MatchString(ingressHost) {
			return nil, ErrInvalidIngressHost
		}
	}

	return &SanitizedDeploymentSpec{
		ServiceName:        sName,
		Environment:        env,
		NormalizedArtifact: artifact,
		Replicas:           spec.Replicas,
		CPURequestMillis:   cpuReq,
		CPULimitMillis:     cpuLim,
		MemoryRequestMB:    memReq,
		MemoryLimitMB:      memLim,
		ProbePath:          probePath,
		ProbePort:          spec.HealthProbe.Port,
		IngressHost:        ingressHost,
	}, nil
}
