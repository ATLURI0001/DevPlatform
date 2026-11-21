package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrInsufficientRoleHierarchy   = errors.New("insufficient role level for requested permission")
	ErrSystemNamespaceForbidden     = errors.New("cannot bind permissions to protected system namespace")
	ErrInvalidSubjectFormat         = errors.New("invalid subject format: must be valid email or sa:<namespace>:<name>")
	ErrDangerousEnvironmentVariable = errors.New("environment variable key violates security policy or contains forbidden system prefix")
)

// RoleLevel represents hierarchical authorization tiers.
type RoleLevel int

const (
	RoleViewer       RoleLevel = 0
	RoleDeveloper    RoleLevel = 1
	RoleOperator     RoleLevel = 2
	RoleClusterAdmin RoleLevel = 3
)

func (r RoleLevel) String() string {
	switch r {
	case RoleViewer:
		return "VIEWER"
	case RoleDeveloper:
		return "DEVELOPER"
	case RoleOperator:
		return "OPERATOR"
	case RoleClusterAdmin:
		return "CLUSTER_ADMIN"
	default:
		return "UNKNOWN"
	}
}

// Action represents discrete deployment platform actions.
type Action string

const (
	ActionRead            Action = "READ"
	ActionDeploy          Action = "DEPLOY"
	ActionMutateConfig    Action = "MUTATE_CONFIG"
	ActionDelete          Action = "DELETE"
	ActionPromoteCanary   Action = "PROMOTE_CANARY"
	ActionAdminOverride   Action = "ADMIN_OVERRIDE"
)

var actionRoleRequirement = map[Action]RoleLevel{
	ActionRead:          RoleViewer,
	ActionDeploy:        RoleDeveloper,
	ActionPromoteCanary: RoleDeveloper,
	ActionMutateConfig:  RoleOperator,
	ActionDelete:        RoleOperator,
	ActionAdminOverride: RoleClusterAdmin,
}

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	saRegex    = regexp.MustCompile(`^sa:[a-z0-9\-]+:[a-z0-9\-]+$`)
	envKeyRegex = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

	protectedNamespaces = map[string]bool{
		"kube-system":        true,
		"kube-public":        true,
		"kube-node-lease":    true,
		"devplatform-system": true,
	}

	dangerousEnvPrefixes = []string{
		"LD_PRELOAD",
		"BASH_ENV",
		"DYLD_INSERT_LIBRARIES",
		"PYTHONPATH",
		"PERL5LIB",
		"RUBYLIB",
	}
)

// RoleBindingContract defines the schema for namespace role bindings.
type RoleBindingContract struct {
	Subject   string    `json:"subject"`
	Namespace string    `json:"namespace"`
	Role      RoleLevel `json:"role"`
}

// ValidateRoleBindingContract checks namespace isolation and subject format.
func ValidateRoleBindingContract(binding RoleBindingContract, callerRole RoleLevel) error {
	trimmedSubject := strings.TrimSpace(binding.Subject)
	if !emailRegex.MatchString(trimmedSubject) && !saRegex.MatchString(trimmedSubject) {
		return fmt.Errorf("%w: %s", ErrInvalidSubjectFormat, binding.Subject)
	}

	trimmedNs := strings.TrimSpace(binding.Namespace)
	if err := ValidateNamespace(trimmedNs); err != nil {
		return err
	}

	if protectedNamespaces[trimmedNs] && callerRole < RoleClusterAdmin {
		return fmt.Errorf("%w: %s requires CLUSTER_ADMIN role", ErrSystemNamespaceForbidden, trimmedNs)
	}

	if callerRole < binding.Role {
		return fmt.Errorf("%w: caller cannot grant role %s exceeding their own level", ErrInsufficientRoleHierarchy, binding.Role)
	}

	return nil
}

// CheckPermission determines whether a given role level can execute the requested action.
func CheckPermission(role RoleLevel, action Action) bool {
	reqRole, exists := actionRoleRequirement[action]
	if !exists {
		return false
	}
	return role >= reqRole
}

// SanitizeEnvironmentOverrides validates and filters environment overrides against injection.
func SanitizeEnvironmentOverrides(env map[string]string) (map[string]string, error) {
	if env == nil {
		return make(map[string]string), nil
	}

	sanitized := make(map[string]string, len(env))
	for k, v := range env {
		trimmedKey := strings.TrimSpace(k)
		if !envKeyRegex.MatchString(trimmedKey) {
			return nil, fmt.Errorf("%w: invalid key '%s'", ErrDangerousEnvironmentVariable, k)
		}

		for _, prefix := range dangerousEnvPrefixes {
			if strings.HasPrefix(trimmedKey, prefix) {
				return nil, fmt.Errorf("%w: dangerous prefix '%s' detected", ErrDangerousEnvironmentVariable, prefix)
			}
		}

		if strings.ContainsAny(v, "\r\n\x00") {
			return nil, fmt.Errorf("environment value contains illegal CRLF or null bytes in '%s'", k)
		}

		sanitized[trimmedKey] = strings.TrimSpace(v)
	}

	return sanitized, nil
}
