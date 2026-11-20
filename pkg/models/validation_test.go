package models

import (
	"errors"
	"testing"
)

func TestValidateCommitSHA(t *testing.T) {
	// Valid 40-char SHA-1
	validSHA1 := "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	if err := ValidateCommitSHA(validSHA1); err != nil {
		t.Fatalf("expected valid SHA-1, got: %v", err)
	}

	// Valid 64-char SHA-256
	validSHA256 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if err := ValidateCommitSHA(validSHA256); err != nil {
		t.Fatalf("expected valid SHA-256, got: %v", err)
	}

	// Invalid short
	if err := ValidateCommitSHA("4b825dc6"); !errors.Is(err, ErrInvalidCommitSHA) {
		t.Fatalf("expected ErrInvalidCommitSHA, got: %v", err)
	}

	// Invalid non-hex characters
	if err := ValidateCommitSHA("4b825dc642cb6eb9a060e54bf8d69288fbee490z"); !errors.Is(err, ErrInvalidCommitSHA) {
		t.Fatalf("expected ErrInvalidCommitSHA for non-hex, got: %v", err)
	}
}

func TestValidateNamespace(t *testing.T) {
	validNamespaces := []string{"default", "kube-system", "prod-cluster-01", "dev"}
	for _, ns := range validNamespaces {
		if err := ValidateNamespace(ns); err != nil {
			t.Fatalf("expected %s to be valid, got: %v", ns, err)
		}
	}

	invalidNamespaces := []string{
		"",               // Empty
		"UPPERCASE",      // Uppercase forbidden
		"-starts-hyphen", // Cannot start with hyphen
		"ends-hyphen-",   // Cannot end with hyphen
		"has_underscore", // Underscores forbidden in DNS-1123 label
	}
	for _, ns := range invalidNamespaces {
		if err := ValidateNamespace(ns); !errors.Is(err, ErrInvalidNamespace) {
			t.Fatalf("expected ErrInvalidNamespace for %s, got: %v", ns, err)
		}
	}
}

func TestValidateSemVer(t *testing.T) {
	validVersions := []string{"1.0.0", "v2.1.3", "0.9.1-alpha.1", "1.2.3+20130313144700"}
	for _, v := range validVersions {
		if err := ValidateSemVer(v); err != nil {
			t.Fatalf("expected %s to be valid semver, got: %v", v, err)
		}
	}

	invalidVersions := []string{"1", "1.0", "v1.a.2", "not-a-version"}
	for _, v := range invalidVersions {
		if err := ValidateSemVer(v); !errors.Is(err, ErrInvalidSemVer) {
			t.Fatalf("expected ErrInvalidSemVer for %s, got: %v", v, err)
		}
	}
}

func TestRedactSensitiveConfig(t *testing.T) {
	cfg := map[string]interface{}{
		"database_host": "db.prod.internal",
		"db_password":   "super-secret-pwd",
		"api_key":       "sk-9988112233",
		"retry_limit":   5,
		"nested": map[string]interface{}{
			"oauth_token": "token-xyz",
			"timeout_ms":  1000,
		},
	}

	redacted := RedactSensitiveConfig(cfg)
	if redacted["database_host"] != "db.prod.internal" {
		t.Fatalf("expected database_host to remain unchanged")
	}
	if redacted["db_password"] != "[REDACTED]" {
		t.Fatalf("expected db_password to be redacted")
	}
	if redacted["api_key"] != "[REDACTED]" {
		t.Fatalf("expected api_key to be redacted")
	}
	nestedMap := redacted["nested"].(map[string]interface{})
	if nestedMap["oauth_token"] != "[REDACTED]" {
		t.Fatalf("expected nested oauth_token to be redacted")
	}
	if nestedMap["timeout_ms"] != 1000 {
		t.Fatalf("expected nested timeout_ms to remain unchanged")
	}
}
