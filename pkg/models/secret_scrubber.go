package models

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"sync"
)

// SecretKind identifies the detected classification of a sensitive credential.
type SecretKind string

const (
	SecretKindAWSAccessKey   SecretKind = "AWS_ACCESS_KEY"
	SecretKindGitHubToken    SecretKind = "GITHUB_TOKEN"
	SecretKindJWT            SecretKind = "JWT_TOKEN"
	SecretKindSlackToken     SecretKind = "SLACK_TOKEN"
	SecretKindPrivateKey     SecretKind = "PRIVATE_KEY"
	SecretKindHighEntropy    SecretKind = "HIGH_ENTROPY_SECRET"
	SecretKindSensitiveKey   SecretKind = "SENSITIVE_KEY_VALUE"
)

// SecretPattern associates a compiled regular expression with a secret classification.
type SecretPattern struct {
	Kind    SecretKind
	Pattern *regexp.Regexp
}

var (
	defaultPatterns = []SecretPattern{
		{
			Kind:    SecretKindAWSAccessKey,
			Pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		},
		{
			Kind:    SecretKindGitHubToken,
			Pattern: regexp.MustCompile(`\b(gh[pousr]_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9_]{82})\b`),
		},
		{
			Kind:    SecretKindJWT,
			Pattern: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
		},
		{
			Kind:    SecretKindSlackToken,
			Pattern: regexp.MustCompile(`\bxox[baprs](-[0-9a-zA-Z]{4,48})+\b`),
		},
		{
			Kind:    SecretKindPrivateKey,
			Pattern: regexp.MustCompile(`-----BEGIN [A-Z ]+PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+PRIVATE KEY-----`),
		},
	}

	sensitiveKeyNames = regexp.MustCompile(`(?i)(password|secret|token|api_?key|auth|bearer|credential|private_?key|access_?key)`)
)

// ScrubberOptions configures detection sensitivity and mask formatting.
type ScrubberOptions struct {
	EnableEntropyScan bool    // Whether to compute Shannon entropy for candidate strings
	EntropyThreshold  float64 // Minimum Shannon entropy to trigger redaction (default: 3.5)
	MinEntropyLength  int     // Minimum length of token to evaluate entropy (default: 20)
	MaskFormat        string  // Template: "[REDACTED:%s]" or "***REDACTED***"
}

// DefaultScrubberOptions provides secure default options for redaction.
func DefaultScrubberOptions() ScrubberOptions {
	return ScrubberOptions{
		EnableEntropyScan: true,
		EntropyThreshold:  3.5,
		MinEntropyLength:  24,
		MaskFormat:        "[REDACTED:%s]",
	}
}

// SecretScrubber provides thread-safe redaction across logs, text, and arbitrary structures.
type SecretScrubber struct {
	mu       sync.RWMutex
	patterns []SecretPattern
	opts     ScrubberOptions
}

// NewSecretScrubber constructs a scrubber with configured options.
func NewSecretScrubber(opts ScrubberOptions) *SecretScrubber {
	if opts.EntropyThreshold <= 0 {
		opts.EntropyThreshold = 3.5
	}
	if opts.MinEntropyLength <= 0 {
		opts.MinEntropyLength = 24
	}
	if opts.MaskFormat == "" {
		opts.MaskFormat = "[REDACTED:%s]"
	}

	return &SecretScrubber{
		patterns: defaultPatterns,
		opts:     opts,
	}
}

// CalculateShannonEntropy computes information entropy H in bits per character.
// Formula: H = - sum(p_i * log2(p_i))
func CalculateShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0.0
	}

	freq := make(map[rune]int)
	for _, r := range s {
		freq[r]++
	}

	n := float64(len(s))
	entropy := 0.0
	for _, count := range freq {
		p := float64(count) / n
		entropy -= p * math.Log2(p)
	}

	return entropy
}

// ScrubText scans a string and masks all detected credentials.
func (s *SecretScrubber) ScrubText(input string) string {
	if input == "" {
		return ""
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := input

	// 1. Regex pattern scanning
	for _, sp := range s.patterns {
		mask := s.formatMask(sp.Kind)
		result = sp.Pattern.ReplaceAllString(result, mask)
	}

	// 2. High-Entropy token scanning (if enabled)
	if s.opts.EnableEntropyScan {
		words := strings.Fields(result)
		for _, word := range words {
			if strings.Contains(word, "REDACTED") {
				continue
			}
			// Strip surrounding punctuation
			trimmed := strings.Trim(word, `"',;:()[]{}<>`)
			if len(trimmed) >= s.opts.MinEntropyLength {
				// Only evaluate tokens containing mixed characters or base64 chars
				if isCandidateSecretToken(trimmed) {
					ent := CalculateShannonEntropy(trimmed)
					if ent >= s.opts.EntropyThreshold {
						mask := s.formatMask(SecretKindHighEntropy)
						result = strings.ReplaceAll(result, trimmed, mask)
					}
				}
			}
		}
	}

	return result
}

// ScrubMap sanitizes a map of string key-value configurations, redacting by key name and value content.
func (s *SecretScrubber) ScrubMap(data map[string]string) map[string]string {
	if data == nil {
		return nil
	}

	sanitized := make(map[string]string, len(data))
	for k, v := range data {
		if sensitiveKeyNames.MatchString(k) {
			sanitized[k] = s.formatMask(SecretKindSensitiveKey)
		} else {
			sanitized[k] = s.ScrubText(v)
		}
	}

	return sanitized
}

// ScrubGeneric recursively traverses JSON-compatible data structures (map, slice, string) and scrubs secrets.
func (s *SecretScrubber) ScrubGeneric(data interface{}) interface{} {
	switch v := data.(type) {
	case string:
		return s.ScrubText(v)
	case map[string]interface{}:
		res := make(map[string]interface{}, len(v))
		for k, val := range v {
			switch childVal := val.(type) {
			case map[string]interface{}, []interface{}:
				res[k] = s.ScrubGeneric(childVal)
			default:
				if sensitiveKeyNames.MatchString(k) {
					res[k] = s.formatMask(SecretKindSensitiveKey)
				} else {
					res[k] = s.ScrubGeneric(childVal)
				}
			}
		}
		return res
	case []interface{}:
		res := make([]interface{}, len(v))
		for i, item := range v {
			res[i] = s.ScrubGeneric(item)
		}
		return res
	default:
		return data
	}
}

// ScrubJSON unmarshals a JSON payload, scrubs all nested sensitive data, and re-marshals to indented JSON.
func (s *SecretScrubber) ScrubJSON(jsonBytes []byte) ([]byte, error) {
	var raw interface{}
	if err := json.Unmarshal(jsonBytes, &raw); err != nil {
		return nil, err
	}

	cleaned := s.ScrubGeneric(raw)
	return json.Marshal(cleaned)
}

func (s *SecretScrubber) formatMask(kind SecretKind) string {
	if strings.Contains(s.opts.MaskFormat, "%s") {
		return strings.Replace(s.opts.MaskFormat, "%s", string(kind), 1)
	}
	return s.opts.MaskFormat
}

func isCandidateSecretToken(s string) bool {
	hasUpper := false
	hasLower := false
	hasDigit := false
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	// Candidate secret must have alphanumeric diversity
	return (hasUpper && hasLower) || (hasLower && hasDigit) || (hasUpper && hasDigit)
}
