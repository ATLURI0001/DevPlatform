package models

import (
	"strings"
	"testing"
)

func TestProbeContractValidator_NominalHTTPGet(t *testing.T) {
	validator := NewProbeContractValidator()

	probe := &ProbeSpec{
		Type:    ProbeTypeLiveness,
		Handler: ProbeHandlerHTTPGet,
		HTTPGet: &HTTPGetAction{
			Path:   "/healthz",
			Port:   8080,
			Scheme: "HTTP",
			HTTPHeaders: map[string]string{
				"X-Custom-Header": "HealthCheckValue",
			},
		},
		InitialDelaySeconds: 5,
		TimeoutSeconds:      2,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}

	val, err := validator.ValidateProbe(probe)
	if err != nil {
		t.Fatalf("expected nominal probe to pass, got error: %v", err)
	}
	if val.SuccessThreshold != 1 {
		t.Fatalf("expected success threshold 1, got %d", val.SuccessThreshold)
	}
}

func TestProbeContractValidator_NominalExecAndTCPSocket(t *testing.T) {
	validator := NewProbeContractValidator()

	// Exec probe
	execProbe := &ProbeSpec{
		Type:    ProbeTypeReadiness,
		Handler: ProbeHandlerExec,
		Exec: &ExecAction{
			Command: []string{"cat", "/tmp/ready.txt"},
		},
		TimeoutSeconds:   3,
		PeriodSeconds:    5,
		SuccessThreshold: 1,
		FailureThreshold: 2,
	}

	if _, err := validator.ValidateProbe(execProbe); err != nil {
		t.Fatalf("expected exec probe to pass, got: %v", err)
	}

	// TCP Socket probe
	tcpProbe := &ProbeSpec{
		Type:    ProbeTypeStartup,
		Handler: ProbeHandlerTCPSocket,
		TCPSocket: &TCPSocketAction{
			Port: 5432,
		},
		TimeoutSeconds:   1,
		PeriodSeconds:    3,
		SuccessThreshold: 1,
		FailureThreshold: 5,
	}

	if _, err := validator.ValidateProbe(tcpProbe); err != nil {
		t.Fatalf("expected tcp probe to pass, got: %v", err)
	}
}

func TestProbeContractValidator_TimingBoundaries(t *testing.T) {
	validator := NewProbeContractValidator()

	base := func() *ProbeSpec {
		return &ProbeSpec{
			Type:    ProbeTypeLiveness,
			Handler: ProbeHandlerHTTPGet,
			HTTPGet: &HTTPGetAction{Path: "/health", Port: 80},
			TimeoutSeconds: 2,
			PeriodSeconds:  5,
			FailureThreshold: 3,
		}
	}

	// Timeout >= Period
	p := base()
	p.TimeoutSeconds = 5
	p.PeriodSeconds = 5
	if _, err := validator.ValidateProbe(p); err == nil || !strings.Contains(err.Error(), "strictly less") {
		t.Fatalf("expected error for timeout >= period, got %v", err)
	}

	// Negative initial delay
	p = base()
	p.InitialDelaySeconds = -10
	if _, err := validator.ValidateProbe(p); err == nil || !strings.Contains(err.Error(), "initialDelaySeconds") {
		t.Fatalf("expected error for negative initial delay, got %v", err)
	}

	// Liveness SuccessThreshold > 1
	p = base()
	p.SuccessThreshold = 2
	if _, err := validator.ValidateProbe(p); err == nil || !strings.Contains(err.Error(), "successThreshold for Liveness must be 1") {
		t.Fatalf("expected error for invalid liveness success threshold, got %v", err)
	}
}

func TestProbeContractValidator_SSRFMitigation(t *testing.T) {
	validator := NewProbeContractValidator()

	probes := []*ProbeSpec{
		{
			Type:    ProbeTypeReadiness,
			Handler: ProbeHandlerHTTPGet,
			HTTPGet: &HTTPGetAction{Path: "/health", Port: 80, Host: "169.254.169.254"},
			TimeoutSeconds: 1, PeriodSeconds: 5, FailureThreshold: 3,
		},
		{
			Type:    ProbeTypeReadiness,
			Handler: ProbeHandlerHTTPGet,
			HTTPGet: &HTTPGetAction{Path: "/health", Port: 80, Host: "metadata.google.internal"},
			TimeoutSeconds: 1, PeriodSeconds: 5, FailureThreshold: 3,
		},
		{
			Type:    ProbeTypeReadiness,
			Handler: ProbeHandlerTCPSocket,
			TCPSocket: &TCPSocketAction{Port: 2379, Host: "127.0.0.1"},
			TimeoutSeconds: 1, PeriodSeconds: 5, FailureThreshold: 3,
		},
	}

	for _, p := range probes {
		if _, err := validator.ValidateProbe(p); err == nil {
			t.Fatalf("expected SSRF target to be blocked, but passed: host=%v", p.HTTPGet)
		}
	}
}

func TestProbeContractValidator_PathTraversalAndCRLF(t *testing.T) {
	validator := NewProbeContractValidator()

	// Path traversal
	p1 := &ProbeSpec{
		Type:    ProbeTypeLiveness,
		Handler: ProbeHandlerHTTPGet,
		HTTPGet: &HTTPGetAction{Path: "/health/../../etc/passwd", Port: 80},
		TimeoutSeconds: 1, PeriodSeconds: 5, FailureThreshold: 3,
	}
	if _, err := validator.ValidateProbe(p1); err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("expected path traversal detection, got %v", err)
	}

	// CRLF in path
	p2 := &ProbeSpec{
		Type:    ProbeTypeLiveness,
		Handler: ProbeHandlerHTTPGet,
		HTTPGet: &HTTPGetAction{Path: "/health\r\nHost: evil.com", Port: 80},
		TimeoutSeconds: 1, PeriodSeconds: 5, FailureThreshold: 3,
	}
	if _, err := validator.ValidateProbe(p2); err == nil || !strings.Contains(err.Error(), "CRLF") {
		t.Fatalf("expected CRLF path detection, got %v", err)
	}

	// CRLF in headers
	p3 := &ProbeSpec{
		Type:    ProbeTypeLiveness,
		Handler: ProbeHandlerHTTPGet,
		HTTPGet: &HTTPGetAction{
			Path: "/health",
			Port: 80,
			HTTPHeaders: map[string]string{
				"X-Forwarded\r\nInjected": "true",
			},
		},
		TimeoutSeconds: 1, PeriodSeconds: 5, FailureThreshold: 3,
	}
	if _, err := validator.ValidateProbe(p3); err == nil || !strings.Contains(err.Error(), "illegal CRLF") {
		t.Fatalf("expected header CRLF detection, got %v", err)
	}
}

func TestProbeContractValidator_ExecCommandInjection(t *testing.T) {
	validator := NewProbeContractValidator()

	injections := [][]string{
		{"/bin/sh", "-c", "check.sh; rm -rf /"},
		{"curl", "http://internal/ready && reboot"},
		{"cat", "`id`"},
		{"test", "$(whoami)"},
		{"echo", "ok > /etc/shadow"},
	}

	for _, cmd := range injections {
		probe := &ProbeSpec{
			Type:    ProbeTypeReadiness,
			Handler: ProbeHandlerExec,
			Exec:    &ExecAction{Command: cmd},
			TimeoutSeconds: 1, PeriodSeconds: 5, FailureThreshold: 3,
		}
		if _, err := validator.ValidateProbe(probe); err == nil {
			t.Fatalf("expected command injection to be blocked: %v", cmd)
		}
	}
}

func TestProbeContractValidator_ValidateManifestProbes(t *testing.T) {
	validator := NewProbeContractValidator()

	probes := []*ProbeSpec{
		{
			Type:    ProbeTypeStartup,
			Handler: ProbeHandlerTCPSocket,
			TCPSocket: &TCPSocketAction{Port: 8080},
			TimeoutSeconds: 1, PeriodSeconds: 3, FailureThreshold: 5,
		},
		{
			Type:    ProbeTypeReadiness,
			Handler: ProbeHandlerHTTPGet,
			HTTPGet: &HTTPGetAction{Path: "/ready", Port: 8080},
			TimeoutSeconds: 2, PeriodSeconds: 5, SuccessThreshold: 1, FailureThreshold: 3,
		},
		{
			Type:    ProbeTypeLiveness,
			Handler: ProbeHandlerHTTPGet,
			HTTPGet: &HTTPGetAction{Path: "/healthz", Port: 8080},
			TimeoutSeconds: 2, PeriodSeconds: 10, SuccessThreshold: 1, FailureThreshold: 3,
		},
	}

	validated, err := validator.ValidateManifestProbes(probes)
	if err != nil {
		t.Fatalf("expected valid manifest probes to pass, got: %v", err)
	}
	if len(validated) != 3 {
		t.Fatalf("expected 3 validated probes, got %d", len(validated))
	}

	// Duplicate phase check
	duplicateProbes := append(probes, &ProbeSpec{
		Type:    ProbeTypeLiveness,
		Handler: ProbeHandlerTCPSocket,
		TCPSocket: &TCPSocketAction{Port: 9090},
		TimeoutSeconds: 1, PeriodSeconds: 5, FailureThreshold: 3,
	})
	if _, err := validator.ValidateManifestProbes(duplicateProbes); err == nil || !strings.Contains(err.Error(), "duplicate probe definition") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}
