package models

import (
	"errors"
	"testing"
	"time"
)

const validDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestPromotionPolicyGate_ValidTransitions(t *testing.T) {
	gate := NewPromotionPolicyGate(2)

	// Dev -> QA
	reqQA := PromotionRequest{
		ArtifactDigest:    validDigest,
		CurrentStage:      StageDev,
		TargetStage:       StageQA,
		SecurityScanClean: true,
		Requester:         "alice",
		Timestamp:         time.Now(),
	}
	if err := gate.Validate(reqQA); err != nil {
		t.Fatalf("expected valid dev -> qa transition, got %v", err)
	}

	// Staging -> Prod
	reqProd := PromotionRequest{
		ArtifactDigest:    validDigest,
		CurrentStage:      StageStaging,
		TargetStage:       StageProd,
		SecurityScanClean: true,
		Requester:         "alice",
		Approvers:         []string{"bob", "carol"},
		Timestamp:         time.Now(),
	}
	if err := gate.Validate(reqProd); err != nil {
		t.Fatalf("expected valid staging -> prod transition, got %v", err)
	}
}

func TestPromotionPolicyGate_SkipStageForbidden(t *testing.T) {
	gate := NewPromotionPolicyGate(2)

	req := PromotionRequest{
		ArtifactDigest:    validDigest,
		CurrentStage:      StageDev,
		TargetStage:       StageProd, // Forbidden skip!
		SecurityScanClean: true,
		Requester:         "alice",
		Approvers:         []string{"bob", "carol"},
	}

	err := gate.Validate(req)
	if !errors.Is(err, ErrInvalidStageTransition) {
		t.Fatalf("expected ErrInvalidStageTransition, got %v", err)
	}
}

func TestPromotionPolicyGate_SecurityScanFailed(t *testing.T) {
	gate := NewPromotionPolicyGate(2)

	req := PromotionRequest{
		ArtifactDigest:    validDigest,
		CurrentStage:      StageQA,
		TargetStage:       StageStaging,
		SecurityScanClean: false,
		Requester:         "alice",
	}

	err := gate.Validate(req)
	if !errors.Is(err, ErrSecurityScanFailed) {
		t.Fatalf("expected ErrSecurityScanFailed, got %v", err)
	}
}

func TestPromotionPolicyGate_InsufficientApprovals(t *testing.T) {
	gate := NewPromotionPolicyGate(2)

	// Requester cannot approve their own prod promotion
	req := PromotionRequest{
		ArtifactDigest:    validDigest,
		CurrentStage:      StageStaging,
		TargetStage:       StageProd,
		SecurityScanClean: true,
		Requester:         "alice",
		Approvers:         []string{"alice", "bob"}, // only 1 distinct external
	}

	err := gate.Validate(req)
	if !errors.Is(err, ErrMissingRequiredApprovals) {
		t.Fatalf("expected ErrMissingRequiredApprovals, got %v", err)
	}
}

func TestPromotionPolicyGate_InvalidDigest(t *testing.T) {
	gate := NewPromotionPolicyGate(2)

	req := PromotionRequest{
		ArtifactDigest:    "not-a-sha256",
		CurrentStage:      StageDev,
		TargetStage:       StageQA,
		SecurityScanClean: true,
		Requester:         "alice",
	}

	err := gate.Validate(req)
	if !errors.Is(err, ErrInvalidArtifactDigest) {
		t.Fatalf("expected ErrInvalidArtifactDigest, got %v", err)
	}
}
