package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	// ErrUnapprovedProductionPromotion is returned when a production deploy lacks mandatory approvals.
	ErrUnapprovedProductionPromotion = errors.New("gitops contract error: production deployment requires >= 2 approvals")

	// ErrInvalidBranchSource is returned when production is deployed from non-main/non-release branches.
	ErrInvalidBranchSource = errors.New("gitops contract error: production deployments only permitted from main or release/* branches")

	// ErrUntrustedImageRegistry is returned when container image does not match trusted registry prefixes.
	ErrUntrustedImageRegistry = errors.New("gitops contract error: container image from untrusted registry")

	// ErrDisallowedLatestTag is returned when an immutable production deployment references :latest.
	ErrDisallowedLatestTag = errors.New("gitops contract error: ':latest' image tag is strictly forbidden in production")

	// ErrExcessiveBlastRadius is returned when production rollout concurrency exceeds safety threshold.
	ErrExcessiveBlastRadius = errors.New("gitops contract error: production rollout blast radius exceeds 25% threshold")
)

var (
	commitSHARegex = regexp.MustCompile(`^[a-f0-9]{40}$`)
)

// GitOpsDeploymentSpec defines proposed deployment configuration across environments.
type GitOpsDeploymentSpec struct {
	Environment     string
	ServiceName     string
	CommitSHA       string
	SourceBranch    string
	ContainerImage  string
	Approvals       []string
	Author          string
	RolloutPercent  int
	SkipSanityCheck bool
}

// GitOpsPolicyContractValidator enforces strict environment isolation and promotion invariants.
type GitOpsPolicyContractValidator struct {
	TrustedRegistryPrefixes []string
	MinProductionApprovals  int
	MaxProductionRollout    int
}

// NewGitOpsPolicyContractValidator constructs a new contract validator.
func NewGitOpsPolicyContractValidator(trustedPrefixes []string) *GitOpsPolicyContractValidator {
	if len(trustedPrefixes) == 0 {
		trustedPrefixes = []string{"ghcr.io/devplatform/", "gcr.io/devplatform/"}
	}
	return &GitOpsPolicyContractValidator{
		TrustedRegistryPrefixes: trustedPrefixes,
		MinProductionApprovals:  2,
		MaxProductionRollout:    25,
	}
}

// ValidateDeploymentProposal enforces all security and stability invariants on the proposal.
func (v *GitOpsPolicyContractValidator) ValidateDeploymentProposal(spec GitOpsDeploymentSpec) error {
	env := strings.ToLower(strings.TrimSpace(spec.Environment))
	if env == "" {
		return errors.New("gitops contract error: environment cannot be empty")
	}
	if strings.TrimSpace(spec.ServiceName) == "" {
		return errors.New("gitops contract error: serviceName cannot be empty")
	}

	// Commit SHA format validation
	if !commitSHARegex.MatchString(strings.TrimSpace(spec.CommitSHA)) {
		return fmt.Errorf("gitops contract error: invalid commit SHA format: '%s'", spec.CommitSHA)
	}

	// Validate Container Image Registry
	trusted := false
	for _, prefix := range v.TrustedRegistryPrefixes {
		if strings.HasPrefix(spec.ContainerImage, prefix) {
			trusted = true
			break
		}
	}
	if !trusted {
		return fmt.Errorf("%w: '%s'", ErrUntrustedImageRegistry, spec.ContainerImage)
	}

	// Environment specific policies: PRODUCTION
	if env == "production" || env == "prod" {
		// 1. Branch source check
		branch := strings.TrimSpace(spec.SourceBranch)
		if branch != "main" && !strings.HasPrefix(branch, "release/") {
			return fmt.Errorf("%w: got '%s'", ErrInvalidBranchSource, branch)
		}

		// 2. Disallow latest tag
		if strings.HasSuffix(spec.ContainerImage, ":latest") || !strings.Contains(spec.ContainerImage, ":") {
			return fmt.Errorf("%w: '%s'", ErrDisallowedLatestTag, spec.ContainerImage)
		}

		// 3. Approval count & separation of duties
		validApprovals := 0
		author := strings.TrimSpace(spec.Author)
		for _, approver := range spec.Approvals {
			cleanApprover := strings.TrimSpace(approver)
			if cleanApprover != "" && !strings.EqualFold(cleanApprover, author) {
				validApprovals++
			}
		}
		if validApprovals < v.MinProductionApprovals {
			return fmt.Errorf("%w: valid approvals %d < required %d", ErrUnapprovedProductionPromotion, validApprovals, v.MinProductionApprovals)
		}

		// 4. Rollout blast radius
		if spec.RolloutPercent <= 0 || spec.RolloutPercent > v.MaxProductionRollout {
			return fmt.Errorf("%w: rollout %d%% exceeds max %d%%", ErrExcessiveBlastRadius, spec.RolloutPercent, v.MaxProductionRollout)
		}
	}

	return nil
}
