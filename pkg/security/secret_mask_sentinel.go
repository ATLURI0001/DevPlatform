package security

import (
	"fmt"
	"strings"
)

// AssertNoRawSecret rejects log lines containing unmasked secret prefixes.
func AssertNoRawSecret(line string) error {
	for _, prefix := range []string{"ghp_", "sk-", "AKIA"} {
		if strings.Contains(line, prefix) {
			return fmt.Errorf("sentinel violation: unmasked secret prefix %q in log", prefix)
		}
	}
	return nil
}
