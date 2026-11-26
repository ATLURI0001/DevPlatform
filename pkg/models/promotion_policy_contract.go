package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidStageTransition   = errors.New("promotion: invalid environment stage sequence transition")
	ErrMissingRequiredApprovals = errors.New("promotion: insufficient required approvals for target stage")
	ErrSecurityScanFailed       = errors.New("promotion: artifact failed security scan gate")
	ErrInvalidArtifactDigest    = errors.New("promotion: artifact digest is malformed or empty")
)

// EnvironmentStage represents an environment tier in the deployment lifecycle.
type EnvironmentStage string

const (
	StageDev     EnvironmentStage = "dev"
	StageQA      EnvironmentStage = "qa"
	StageStaging EnvironmentStage = "staging"
	StageProd    EnvironmentStage = "prod"
)

// PromotionRequest defines metadata for promoting an artifact to a target stage.
type PromotionRequest struct {
	ArtifactDigest    string
	CurrentStage      EnvironmentStage
	TargetStage       EnvironmentStage
	Approvers         []string
	SecurityScanClean bool
	Requester         string
	Timestamp         time.Time
}

// PromotionPolicyGate validates stage transitions against strict governance contracts.
type PromotionPolicyGate struct {
	MinProdApprovals int
}

// NewPromotionPolicyGate creates a new policy gate enforcer.
func NewPromotionPolicyGate(minProdApprovals int) *PromotionPolicyGate {
	if minProdApprovals <= 0 {
		minProdApprovals = 2
	}
	return &PromotionPolicyGate{
		MinProdApprovals: minProdApprovals,
	}
}

// Validate verifies promotion request schema and stage governance invariants.
func (g *PromotionPolicyGate) Validate(req PromotionRequest) error {
	// 1. Digest format check (sha256:<64-hex>)
	if !strings.HasPrefix(req.ArtifactDigest, "sha256:") || len(req.ArtifactDigest) != 71 {
		return ErrInvalidArtifactDigest
	}

	// 2. Validate linear progression sequence
	if err := validateStageSequence(req.CurrentStage, req.TargetStage); err != nil {
		return err
	}

	// 3. Security scan gate
	if !req.SecurityScanClean {
		return ErrSecurityScanFailed
	}

	// 4. Staging and Prod approvals
	if req.TargetStage == StageProd {
		uniqueApprovers := make(map[string]bool)
		for _, a := range req.Approvers {
			if a != "" && a != req.Requester { // Separation of duties
				uniqueApprovers[a] = true
			}
		}

		if len(uniqueApprovers) < g.MinProdApprovals {
			return fmt.Errorf("%w: required %d distinct approvers (excluding requester), got %d",
				ErrMissingRequiredApprovals, g.MinProdApprovals, len(uniqueApprovers))
		}
	}

	return nil
}

func validateStageSequence(current, target EnvironmentStage) error {
	switch current {
	case StageDev:
		if target != StageQA {
			return fmt.Errorf("%w: from 'dev' can only promote to 'qa', requested '%s'", ErrInvalidStageTransition, target)
		}
	case StageQA:
		if target != StageStaging {
			return fmt.Errorf("%w: from 'qa' can only promote to 'staging', requested '%s'", ErrInvalidStageTransition, target)
		}
	case StageStaging:
		if target != StageProd {
			return fmt.Errorf("%w: from 'staging' can only promote to 'prod', requested '%s'", ErrInvalidStageTransition, target)
		}
	default:
		return fmt.Errorf("%w: unsupported current stage '%s'", ErrInvalidStageTransition, current)
	}
	return nil
}
