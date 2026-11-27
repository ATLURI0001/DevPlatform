package security

import (
	"fmt"
	"strings"
)

// AssertSafeEnvValue rejects newline or NUL injection in environment variable values.
func AssertSafeEnvValue(val string) error {
	if strings.ContainsAny(val, "\r\n\x00") {
		return fmt.Errorf("sentinel violation: newline or NUL in env value")
	}
	return nil
}
