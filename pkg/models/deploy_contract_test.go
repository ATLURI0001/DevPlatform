package models

import (
	"errors"
	"strings"
	"testing"
)

func validTestSpec() DeploymentSpec {
	return DeploymentSpec{
		ServiceName: "payments-api",
		Environment: EnvProduction,
		Artifact:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Replicas:    3,
		Resources: ResourceRequirements{
			CPURequest:    "250m",
			CPULimit:      "1",
			MemoryRequest: "512Mi",
			MemoryLimit:   "2Gi",
		},
		HealthProbe: HealthProbeConfig{
			Path: "/healthz",
			Port: 8080,
		},
		IngressHost: "api.payments.internal.net",
	}
}

func TestValidateAndSanitizeDeploymentSpec_Success(t *testing.T) {
	spec := validTestSpec()
	sanitized, err := ValidateAndSanitizeDeploymentSpec(spec)
	if err != nil {
		t.Fatalf("expected valid spec to pass, got: %v", err)
	}

	if sanitized.ServiceName != "payments-api" {
		t.Errorf("expected payments-api, got %s", sanitized.ServiceName)
	}
	if sanitized.Environment != EnvProduction {
		t.Errorf("expected production, got %s", sanitized.Environment)
	}
	if sanitized.CPURequestMillis != 250 || sanitized.CPULimitMillis != 1000 {
		t.Errorf("expected 250m/1000m, got %dm/%dm", sanitized.CPURequestMillis, sanitized.CPULimitMillis)
	}
	if sanitized.MemoryRequestMB != 512 || sanitized.MemoryLimitMB != 2048 {
		t.Errorf("expected 512MB/2048MB, got %dMB/%dMB", sanitized.MemoryRequestMB, sanitized.MemoryLimitMB)
	}
	if sanitized.ProbePath != "/healthz" || sanitized.ProbePort != 8080 {
		t.Errorf("expected /healthz:8080, got %s:%d", sanitized.ProbePath, sanitized.ProbePort)
	}
}

func TestValidateAndSanitizeDeploymentSpec_ServiceNameErrors(t *testing.T) {
	badNames := []string{
		"",
		"-start-hyphen",
		"end-hyphen-",
		"Uppercase-Service",
		"special_char",
		strings.Repeat("a", 64),
	}

	for _, name := range badNames {
		spec := validTestSpec()
		spec.ServiceName = name
		_, err := ValidateAndSanitizeDeploymentSpec(spec)
		if !errors.Is(err, ErrInvalidServiceName) {
			t.Errorf("expected ErrInvalidServiceName for %q, got: %v", name, err)
		}
	}
}

func TestValidateAndSanitizeDeploymentSpec_EnvironmentErrors(t *testing.T) {
	badEnvs := []DeploymentEnvironment{"", "prod", "stage", "testing", "local"}
	for _, env := range badEnvs {
		spec := validTestSpec()
		spec.Environment = env
		_, err := ValidateAndSanitizeDeploymentSpec(spec)
		if !errors.Is(err, ErrInvalidEnvironment) {
			t.Errorf("expected ErrInvalidEnvironment for %q, got: %v", env, err)
		}
	}
}

func TestValidateAndSanitizeDeploymentSpec_ArtifactHashErrors(t *testing.T) {
	badArtifacts := []string{
		"",
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // missing sha256: prefix
		"sha256:short",
		"md5:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"sha256:" + strings.Repeat("z", 64), // invalid hex
	}

	for _, art := range badArtifacts {
		spec := validTestSpec()
		spec.Artifact = art
		_, err := ValidateAndSanitizeDeploymentSpec(spec)
		if !errors.Is(err, ErrInvalidArtifactHash) {
			t.Errorf("expected ErrInvalidArtifactHash for %q, got: %v", art, err)
		}
	}
}

func TestValidateAndSanitizeDeploymentSpec_ReplicaBounds(t *testing.T) {
	badReplicas := []int{0, -1, 1001}
	for _, r := range badReplicas {
		spec := validTestSpec()
		spec.Replicas = r
		_, err := ValidateAndSanitizeDeploymentSpec(spec)
		if !errors.Is(err, ErrInvalidReplicaCount) {
			t.Errorf("expected ErrInvalidReplicaCount for replicas=%d, got: %v", r, err)
		}
	}
}

func TestValidateAndSanitizeDeploymentSpec_CPUValidation(t *testing.T) {
	spec := validTestSpec()
	spec.Resources.CPURequest = "2000m"
	spec.Resources.CPULimit = "1" // 2000m > 1000m
	_, err := ValidateAndSanitizeDeploymentSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "exceeds cpu_limit") {
		t.Errorf("expected cpu_request exceeds cpu_limit error, got: %v", err)
	}

	spec.Resources.CPURequest = "bad-cpu"
	_, err = ValidateAndSanitizeDeploymentSpec(spec)
	if !errors.Is(err, ErrInvalidCPUResource) {
		t.Errorf("expected ErrInvalidCPUResource, got: %v", err)
	}
}

func TestValidateAndSanitizeDeploymentSpec_MemoryValidation(t *testing.T) {
	spec := validTestSpec()
	spec.Resources.MemoryRequest = "4Gi"
	spec.Resources.MemoryLimit = "2Gi" // request > limit
	_, err := ValidateAndSanitizeDeploymentSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "exceeds memory_limit") {
		t.Errorf("expected memory_request exceeds memory_limit error, got: %v", err)
	}

	spec.Resources.MemoryRequest = "512MB" // invalid notation, must be Mi/Gi/Ti
	_, err = ValidateAndSanitizeDeploymentSpec(spec)
	if !errors.Is(err, ErrInvalidMemoryResource) {
		t.Errorf("expected ErrInvalidMemoryResource, got: %v", err)
	}
}

func TestValidateAndSanitizeDeploymentSpec_HealthProbeErrors(t *testing.T) {
	badPaths := []string{
		"",
		"healthz", // missing leading slash
		"/health?query=1", // forbidden query param
		"/health\x00",
		"/" + strings.Repeat("a", 257),
	}

	for _, p := range badPaths {
		spec := validTestSpec()
		spec.HealthProbe.Path = p
		_, err := ValidateAndSanitizeDeploymentSpec(spec)
		if !errors.Is(err, ErrInvalidProbePath) {
			t.Errorf("expected ErrInvalidProbePath for %q, got: %v", p, err)
		}
	}

	badPorts := []int{0, -80, 65536}
	for _, port := range badPorts {
		spec := validTestSpec()
		spec.HealthProbe.Port = port
		_, err := ValidateAndSanitizeDeploymentSpec(spec)
		if !errors.Is(err, ErrInvalidProbePort) {
			t.Errorf("expected ErrInvalidProbePort for port=%d, got: %v", port, err)
		}
	}
}

func TestValidateAndSanitizeDeploymentSpec_IngressHost(t *testing.T) {
	spec := validTestSpec()
	spec.IngressHost = "https://invalid-protocol.com"
	_, err := ValidateAndSanitizeDeploymentSpec(spec)
	if !errors.Is(err, ErrInvalidIngressHost) {
		t.Errorf("expected ErrInvalidIngressHost for URL prefix, got: %v", err)
	}

	spec.IngressHost = "invalid host with spaces.com"
	_, err = ValidateAndSanitizeDeploymentSpec(spec)
	if !errors.Is(err, ErrInvalidIngressHost) {
		t.Errorf("expected ErrInvalidIngressHost for spaced host, got: %v", err)
	}
}
