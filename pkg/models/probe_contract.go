package models

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ProbeType specifies the lifecycle phase of the health check.
type ProbeType string

const (
	ProbeTypeLiveness  ProbeType = "Liveness"
	ProbeTypeReadiness ProbeType = "Readiness"
	ProbeTypeStartup   ProbeType = "Startup"
)

// ProbeHandlerType defines the transport mechanism of the probe.
type ProbeHandlerType string

const (
	ProbeHandlerHTTPGet   ProbeHandlerType = "HTTPGet"
	ProbeHandlerTCPSocket ProbeHandlerType = "TCPSocket"
	ProbeHandlerExec      ProbeHandlerType = "Exec"
	ProbeHandlerGRPC      ProbeHandlerType = "GRPC"
)

// HTTPGetAction specifies the HTTP request configuration.
type HTTPGetAction struct {
	Path        string            `json:"path"`
	Port        int               `json:"port"`
	Host        string            `json:"host,omitempty"`
	Scheme      string            `json:"scheme,omitempty"`
	HTTPHeaders map[string]string `json:"httpHeaders,omitempty"`
}

// TCPSocketAction specifies TCP port connection parameters.
type TCPSocketAction struct {
	Port int    `json:"port"`
	Host string `json:"host,omitempty"`
}

// ExecAction defines an in-container command execution check.
type ExecAction struct {
	Command []string `json:"command"`
}

// GRPCAction specifies gRPC health checking parameters.
type GRPCAction struct {
	Port    int    `json:"port"`
	Service string `json:"service,omitempty"`
}

// ProbeSpec defines the full health probe configuration for service deployments.
type ProbeSpec struct {
	Type                ProbeType        `json:"type"`
	Handler             ProbeHandlerType `json:"handler"`
	HTTPGet             *HTTPGetAction   `json:"httpGet,omitempty"`
	TCPSocket           *TCPSocketAction `json:"tcpSocket,omitempty"`
	Exec                *ExecAction      `json:"exec,omitempty"`
	GRPC                *GRPCAction      `json:"grpc,omitempty"`
	InitialDelaySeconds int              `json:"initialDelaySeconds"`
	TimeoutSeconds      int              `json:"timeoutSeconds"`
	PeriodSeconds       int              `json:"periodSeconds"`
	SuccessThreshold    int              `json:"successThreshold"`
	FailureThreshold    int              `json:"failureThreshold"`
}

var (
	// Blocked SSRF metadata targets
	blockedHosts = map[string]bool{
		"169.254.169.254":          true,
		"metadata.google.internal": true,
		"metadata":                 true,
		"instance-data":            true,
		"100.100.100.200":          true, // Alibaba Cloud metadata
		"fd00:ec2::254":            true, // AWS IPv6 metadata
	}

	shellInjectionChars = []string{";", "&&", "||", "|", "`", "$(", "${", ">", "<", "\n", "\r"}
)

// ProbeContractValidator enforces contract boundaries on deployment health probe specifications.
type ProbeContractValidator struct {
	AllowCustomHosts bool
}

// NewProbeContractValidator creates a validator instance with defensive defaults.
func NewProbeContractValidator() *ProbeContractValidator {
	return &ProbeContractValidator{
		AllowCustomHosts: false,
	}
}

// ValidateProbe hardens and sanitizes a single ProbeSpec against security and boundary invariants.
func (v *ProbeContractValidator) ValidateProbe(probe *ProbeSpec) (*ProbeSpec, error) {
	if probe == nil {
		return nil, errors.New("probe specification cannot be nil")
	}

	// 1. Validate Probe Type
	switch probe.Type {
	case ProbeTypeLiveness, ProbeTypeReadiness, ProbeTypeStartup:
		// Valid
	default:
		return nil, fmt.Errorf("invalid probe type '%s'", probe.Type)
	}

	// 2. Validate Invariant Timing Boundaries
	if probe.InitialDelaySeconds < 0 || probe.InitialDelaySeconds > 3600 {
		return nil, fmt.Errorf("initialDelaySeconds %d out of bounds [0, 3600]", probe.InitialDelaySeconds)
	}

	if probe.TimeoutSeconds < 1 || probe.TimeoutSeconds > 60 {
		return nil, fmt.Errorf("timeoutSeconds %d out of bounds [1, 60]", probe.TimeoutSeconds)
	}

	if probe.PeriodSeconds < 1 || probe.PeriodSeconds > 300 {
		return nil, fmt.Errorf("periodSeconds %d out of bounds [1, 300]", probe.PeriodSeconds)
	}

	if probe.TimeoutSeconds >= probe.PeriodSeconds {
		return nil, fmt.Errorf("timeoutSeconds (%d) must be strictly less than periodSeconds (%d)",
			probe.TimeoutSeconds, probe.PeriodSeconds)
	}

	if probe.FailureThreshold < 1 || probe.FailureThreshold > 10 {
		return nil, fmt.Errorf("failureThreshold %d out of bounds [1, 10]", probe.FailureThreshold)
	}

	if probe.Type == ProbeTypeLiveness || probe.Type == ProbeTypeStartup {
		if probe.SuccessThreshold != 1 && probe.SuccessThreshold != 0 {
			return nil, fmt.Errorf("successThreshold for %s must be 1, got %d", probe.Type, probe.SuccessThreshold)
		}
	} else if probe.SuccessThreshold < 1 || probe.SuccessThreshold > 10 {
		return nil, fmt.Errorf("successThreshold %d out of bounds [1, 10]", probe.SuccessThreshold)
	}

	// Default normalized values
	normalized := *probe
	if normalized.SuccessThreshold == 0 {
		normalized.SuccessThreshold = 1
	}

	// 3. Validate Handler and Transport Action
	switch probe.Handler {
	case ProbeHandlerHTTPGet:
		if err := v.validateHTTPGet(probe.HTTPGet); err != nil {
			return nil, fmt.Errorf("invalid HTTPGet probe: %w", err)
		}
	case ProbeHandlerTCPSocket:
		if err := v.validateTCPSocket(probe.TCPSocket); err != nil {
			return nil, fmt.Errorf("invalid TCPSocket probe: %w", err)
		}
	case ProbeHandlerExec:
		if err := v.validateExec(probe.Exec); err != nil {
			return nil, fmt.Errorf("invalid Exec probe: %w", err)
		}
	case ProbeHandlerGRPC:
		if err := v.validateGRPC(probe.GRPC); err != nil {
			return nil, fmt.Errorf("invalid GRPC probe: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported probe handler '%s'", probe.Handler)
	}

	return &normalized, nil
}

func (v *ProbeContractValidator) validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d out of range [1, 65535]", port)
	}
	return nil
}

func (v *ProbeContractValidator) validateHost(host string) error {
	cleanHost := strings.TrimSpace(strings.ToLower(host))
	if cleanHost == "" {
		return nil
	}

	if blockedHosts[cleanHost] {
		return fmt.Errorf("host '%s' is blocked by SSRF perimeter protection", host)
	}

	if ip := net.ParseIP(cleanHost); ip != nil {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsLoopback() {
			if !v.AllowCustomHosts {
				return fmt.Errorf("host IP '%s' is restricted by link-local/loopback policy", host)
			}
		}
	}

	return nil
}

func (v *ProbeContractValidator) validateHTTPGet(http *HTTPGetAction) error {
	if http == nil {
		return errors.New("httpGet action cannot be nil")
	}

	if err := v.validatePort(http.Port); err != nil {
		return err
	}

	if err := v.validateHost(http.Host); err != nil {
		return err
	}

	cleanPath := strings.TrimSpace(http.Path)
	if cleanPath == "" || !strings.HasPrefix(cleanPath, "/") {
		return fmt.Errorf("path '%s' must begin with '/'", http.Path)
	}

	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path '%s' contains forbidden path traversal sequence", http.Path)
	}

	if strings.ContainsAny(cleanPath, "\r\n") {
		return errors.New("path contains forbidden CRLF control characters")
	}

	// Validate path parsing
	if _, err := url.Parse(cleanPath); err != nil {
		return fmt.Errorf("malformed URL path: %w", err)
	}

	if http.Scheme != "" && http.Scheme != "HTTP" && http.Scheme != "HTTPS" {
		return fmt.Errorf("scheme must be 'HTTP' or 'HTTPS', got '%s'", http.Scheme)
	}

	for k, val := range http.HTTPHeaders {
		if strings.ContainsAny(k, "\r\n:") || strings.ContainsAny(val, "\r\n") {
			return fmt.Errorf("header '%s: %s' contains illegal CRLF or colon characters", k, val)
		}
	}

	return nil
}

func (v *ProbeContractValidator) validateTCPSocket(tcp *TCPSocketAction) error {
	if tcp == nil {
		return errors.New("tcpSocket action cannot be nil")
	}
	if err := v.validatePort(tcp.Port); err != nil {
		return err
	}
	return v.validateHost(tcp.Host)
}

func (v *ProbeContractValidator) validateExec(exec *ExecAction) error {
	if exec == nil || len(exec.Command) == 0 {
		return errors.New("exec command list cannot be empty")
	}

	for i, arg := range exec.Command {
		if strings.TrimSpace(arg) == "" {
			return fmt.Errorf("command arg[%d] cannot be empty or blank", i)
		}

		for _, char := range shellInjectionChars {
			if strings.Contains(arg, char) {
				return fmt.Errorf("command arg[%d] '%s' contains forbidden shell injection token '%s'", i, arg, char)
			}
		}
	}

	return nil
}

func (v *ProbeContractValidator) validateGRPC(grpc *GRPCAction) error {
	if grpc == nil {
		return errors.New("grpc action cannot be nil")
	}
	if err := v.validatePort(grpc.Port); err != nil {
		return err
	}
	if strings.ContainsAny(grpc.Service, "\r\n\t") {
		return errors.New("grpc service name contains invalid control characters")
	}
	return nil
}

// ValidateManifestProbes validates a batch of probe specifications from a deployment manifest.
func (v *ProbeContractValidator) ValidateManifestProbes(probes []*ProbeSpec) ([]*ProbeSpec, error) {
	if len(probes) == 0 {
		return nil, errors.New("deployment manifest contains no probe specifications")
	}

	seenTypes := make(map[ProbeType]bool)
	validated := make([]*ProbeSpec, len(probes))

	for i, probe := range probes {
		if probe == nil {
			return nil, fmt.Errorf("probe[%d] is nil", i)
		}
		if seenTypes[probe.Type] {
			return nil, fmt.Errorf("duplicate probe definition for lifecycle phase '%s'", probe.Type)
		}
		seenTypes[probe.Type] = true

		val, err := v.ValidateProbe(probe)
		if err != nil {
			return nil, fmt.Errorf("probe[%d] (%s): %w", i, probe.Type, err)
		}
		validated[i] = val
	}

	return validated, nil
}
