package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	sha1Regex       = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	sha256Regex     = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	dns1123Subdomain = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	semverRegex     = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)
)

var (
	ErrInvalidCommitSHA = errors.New("invalid commit SHA: must be 40-char or 64-char hexadecimal string")
	ErrInvalidNamespace = errors.New("invalid namespace: must conform to Kubernetes DNS-1123 label standard")
	ErrInvalidSemVer    = errors.New("invalid semantic version format")
)

// ValidateCommitSHA verifies that sha is a valid 40-character (SHA-1) or 64-character (SHA-256) hex string.
func ValidateCommitSHA(sha string) error {
	clean := strings.TrimSpace(sha)
	if sha1Regex.MatchString(clean) || sha256Regex.MatchString(clean) {
		return nil
	}
	return ErrInvalidCommitSHA
}

// ValidateNamespace checks if ns conforms to RFC 1123 DNS label standards (max 63 chars, lowercase alphanumeric or hyphens).
func ValidateNamespace(ns string) error {
	if len(ns) == 0 || len(ns) > 63 {
		return fmt.Errorf("%w: length must be between 1 and 63 characters", ErrInvalidNamespace)
	}
	if !dns1123Subdomain.MatchString(ns) {
		return fmt.Errorf("%w: must consist of lower case alphanumeric characters or '-', and start/end with an alphanumeric character", ErrInvalidNamespace)
	}
	return nil
}

// ValidateSemVer checks whether version complies with Semantic Versioning 2.0.0.
func ValidateSemVer(version string) error {
	if !semverRegex.MatchString(strings.TrimSpace(version)) {
		return ErrInvalidSemVer
	}
	return nil
}

// RedactSensitiveConfig creates a copy of the configuration map with sensitive credentials masked as "[REDACTED]".
func RedactSensitiveConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}

	sanitized := make(map[string]interface{}, len(config))
	sensitiveKeywords := []string{"secret", "password", "token", "key", "credential", "auth", "private"}

	for k, v := range config {
		lowerKey := strings.ToLower(k)
		isSensitive := false
		for _, kw := range sensitiveKeywords {
			if strings.Contains(lowerKey, kw) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			sanitized[k] = "[REDACTED]"
		} else if nested, ok := v.(map[string]interface{}); ok {
			sanitized[k] = RedactSensitiveConfig(nested)
		} else {
			sanitized[k] = v
		}
	}
	return sanitized
}
