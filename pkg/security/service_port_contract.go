package security

import "fmt"

// ValidateServicePort enforces valid TCP port range [1, 65535].
func ValidateServicePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("contract violation: port %d out of range [1, 65535]", port)
	}
	return nil
}
