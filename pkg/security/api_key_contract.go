package security

import (
	"errors"
	"strings"
)

var ErrAPIKeyContractViolation = errors.New("invalid api key")

// ValidateAPIKey enforces minimum length and no whitespace.
func ValidateAPIKey(key string) error {
	if len(key) < 16 || strings.ContainsAny(key, " \t\n") {
		return ErrAPIKeyContractViolation
	}
	return nil
}
