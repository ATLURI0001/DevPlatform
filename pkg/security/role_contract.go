package security

import (
	"errors"
	"regexp"
)

var (
	ErrRoleContractViolation = errors.New("invalid role")
	rolePattern              = regexp.MustCompile(`^[a-z][a-z0-9_:-]{1,63}$`)
)

// ValidateRole enforces lowercase alphanumeric role names.
func ValidateRole(role string) error {
	if !rolePattern.MatchString(role) {
		return ErrRoleContractViolation
	}
	return nil
}
