package models

import (
	"errors"
	"testing"
)

func TestGitOpsPolicyContractValidator_NominalProduction(t *testing.T) {
	validator := NewGitOpsPolicyContractValidator(nil)

	spec := GitOpsDeploymentSpec{
		Environment:    "production",
		ServiceName:    "auth-service",
		CommitSHA:      "e0ff97a5bfcbef6ccc9e60e19c92a6da07b11234",
		SourceBranch:   "main",
		ContainerImage: "ghcr.io/devplatform/auth-service:v2.4.1",
		Approvals:      []string{"lead-engineer", "security-officer"},
		Author:         "developer-1",
		RolloutPercent: 10,
	}

	if err := validator.ValidateDeploymentProposal(spec); err != nil {
		t.Fatalf("expected nominal production deploy to pass, got: %v", err)
	}
}

func TestGitOpsPolicyContractValidator_UnapprovedProduction(t *testing.T) {
	validator := NewGitOpsPolicyContractValidator(nil)

	spec := GitOpsDeploymentSpec{
		Environment:    "production",
		ServiceName:    "billing-service",
		CommitSHA:      "e0ff97a5bfcbef6ccc9e60e19c92a6da07b11234",
		SourceBranch:   "main",
		ContainerImage: "ghcr.io/devplatform/billing-service:v1.0.0",
		Approvals:      []string{"developer-1"}, // Only author approved!
		Author:         "developer-1",
		RolloutPercent: 10,
	}

	err := validator.ValidateDeploymentProposal(spec)
	if err == nil {
		t.Fatal("expected unapproved production deploy to fail")
	}
	if !errors.Is(err, ErrUnapprovedProductionPromotion) {
		t.Fatalf("expected ErrUnapprovedProductionPromotion, got %v", err)
	}
}

func TestGitOpsPolicyContractValidator_DisallowLatestTagInProd(t *testing.T) {
	validator := NewGitOpsPolicyContractValidator(nil)

	spec := GitOpsDeploymentSpec{
		Environment:    "production",
		ServiceName:    "api-gateway",
		CommitSHA:      "e0ff97a5bfcbef6ccc9e60e19c92a6da07b11234",
		SourceBranch:   "main",
		ContainerImage: "ghcr.io/devplatform/api-gateway:latest", // Forbidden!
		Approvals:      []string{"approver-1", "approver-2"},
		Author:         "author",
		RolloutPercent: 10,
	}

	err := validator.ValidateDeploymentProposal(spec)
	if err == nil {
		t.Fatal("expected :latest tag error")
	}
	if !errors.Is(err, ErrDisallowedLatestTag) {
		t.Fatalf("expected ErrDisallowedLatestTag, got %v", err)
	}
}

func TestGitOpsPolicyContractValidator_InvalidBranchSource(t *testing.T) {
	validator := NewGitOpsPolicyContractValidator(nil)

	spec := GitOpsDeploymentSpec{
		Environment:    "production",
		ServiceName:    "core-engine",
		CommitSHA:      "e0ff97a5bfcbef6ccc9e60e19c92a6da07b11234",
		SourceBranch:   "feature/quick-hack", // Forbidden for prod!
		ContainerImage: "ghcr.io/devplatform/core-engine:v1.2.3",
		Approvals:      []string{"approver-1", "approver-2"},
		Author:         "author",
		RolloutPercent: 10,
	}

	err := validator.ValidateDeploymentProposal(spec)
	if err == nil {
		t.Fatal("expected invalid branch error")
	}
	if !errors.Is(err, ErrInvalidBranchSource) {
		t.Fatalf("expected ErrInvalidBranchSource, got %v", err)
	}
}

func TestGitOpsPolicyContractValidator_UntrustedRegistry(t *testing.T) {
	validator := NewGitOpsPolicyContractValidator([]string{"ghcr.io/devplatform/"})

	spec := GitOpsDeploymentSpec{
		Environment:    "staging",
		ServiceName:    "worker",
		CommitSHA:      "e0ff97a5bfcbef6ccc9e60e19c92a6da07b11234",
		SourceBranch:   "feature/test",
		ContainerImage: "docker.io/randomuser/malicious:v1",
	}

	err := validator.ValidateDeploymentProposal(spec)
	if err == nil {
		t.Fatal("expected untrusted registry error")
	}
	if !errors.Is(err, ErrUntrustedImageRegistry) {
		t.Fatalf("expected ErrUntrustedImageRegistry, got %v", err)
	}
}

func TestGitOpsPolicyContractValidator_ExcessiveBlastRadius(t *testing.T) {
	validator := NewGitOpsPolicyContractValidator(nil)

	spec := GitOpsDeploymentSpec{
		Environment:    "production",
		ServiceName:    "worker",
		CommitSHA:      "e0ff97a5bfcbef6ccc9e60e19c92a6da07b11234",
		SourceBranch:   "main",
		ContainerImage: "ghcr.io/devplatform/worker:v1.0.0",
		Approvals:      []string{"app-1", "app-2"},
		Author:         "author",
		RolloutPercent: 50, // Exceeds 25% max!
	}

	err := validator.ValidateDeploymentProposal(spec)
	if err == nil {
		t.Fatal("expected excessive blast radius error")
	}
	if !errors.Is(err, ErrExcessiveBlastRadius) {
		t.Fatalf("expected ErrExcessiveBlastRadius, got %v", err)
	}
}
