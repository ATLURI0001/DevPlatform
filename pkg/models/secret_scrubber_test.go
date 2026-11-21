package models

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestCalculateShannonEntropy(t *testing.T) {
	// Uniform repeated character has zero entropy
	zero := CalculateShannonEntropy("AAAAAAAAAAAAAAAAAAAA")
	if zero != 0.0 {
		t.Errorf("expected 0.0 entropy for repeated string, got %f", zero)
	}

	// Mixed alphanumeric string should exhibit high entropy (> 3.5 bits/char)
	high := CalculateShannonEntropy("gH8!xK2#vL9$mP4*wQ1@")
	if high < 3.5 {
		t.Errorf("expected high entropy (> 3.5), got %f", high)
	}
}

func TestSecretScrubber_PatternScrubbing(t *testing.T) {
	scrubber := NewSecretScrubber(DefaultScrubberOptions())

	tests := []struct {
		name     string
		input    string
		contains string
		excluded string
	}{
		{
			name:     "AWS Access Key",
			input:    "Deploying with AWS key AKIAIOSFODNN7EXAMPLE to us-east-1",
			contains: "[REDACTED:AWS_ACCESS_KEY]",
			excluded: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:     "GitHub Token",
			input:    "Git clone using ghp_1234567890abcdefghijklmnopqrstuvwxyz as auth",
			contains: "[REDACTED:GITHUB_TOKEN]",
			excluded: "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		},
		{
			name:     "Slack Token",
			input:    "Notification sent via xoxb-123456789012-1234567890123-abcdef123456 successfully",
			contains: "[REDACTED:SLACK_TOKEN]",
			excluded: "xoxb-123456789012-1234567890123-abcdef123456",
		},
		{
			name:     "JWT Token",
			input:    "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			contains: "[REDACTED:JWT_TOKEN]",
			excluded: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		},
		{
			name: "RSA Private Key",
			input: "Key data:\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA0Y1m3...\n-----END RSA PRIVATE KEY-----\nDone",
			contains: "[REDACTED:PRIVATE_KEY]",
			excluded: "-----BEGIN RSA PRIVATE KEY-----",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scrubbed := scrubber.ScrubText(tc.input)
			if !strings.Contains(scrubbed, tc.contains) {
				t.Errorf("expected output to contain %q, got: %s", tc.contains, scrubbed)
			}
			if strings.Contains(scrubbed, tc.excluded) {
				t.Errorf("expected output to NOT contain %q, got: %s", tc.excluded, scrubbed)
			}
		})
	}
}

func TestSecretScrubber_HighEntropyDetection(t *testing.T) {
	scrubber := NewSecretScrubber(DefaultScrubberOptions())

	// Candidate high entropy secret token
	input := "Connection secret: 8Fk9X2mQvL0pRt4wYz7uBn1aCe3dFg6h"
	scrubbed := scrubber.ScrubText(input)

	if !strings.Contains(scrubbed, "[REDACTED:HIGH_ENTROPY_SECRET]") {
		t.Errorf("expected high entropy token to be masked, got: %s", scrubbed)
	}
}

func TestSecretScrubber_ScrubMap(t *testing.T) {
	scrubber := NewSecretScrubber(DefaultScrubberOptions())

	config := map[string]string{
		"DATABASE_URL":      "postgres://user:pass@localhost:5432/db",
		"API_SECRET_KEY":    "super-confidential-phrase",
		"CLUSTER_REGION":    "us-central1",
		"DEPLOY_AUTH_TOKEN": "token-12345",
	}

	cleaned := scrubber.ScrubMap(config)

	if cleaned["API_SECRET_KEY"] != "[REDACTED:SENSITIVE_KEY_VALUE]" {
		t.Errorf("expected API_SECRET_KEY to be redacted by key name, got: %s", cleaned["API_SECRET_KEY"])
	}
	if cleaned["DEPLOY_AUTH_TOKEN"] != "[REDACTED:SENSITIVE_KEY_VALUE]" {
		t.Errorf("expected DEPLOY_AUTH_TOKEN to be redacted by key name, got: %s", cleaned["DEPLOY_AUTH_TOKEN"])
	}
	if cleaned["CLUSTER_REGION"] != "us-central1" {
		t.Errorf("expected CLUSTER_REGION to remain unchanged, got: %s", cleaned["CLUSTER_REGION"])
	}
}

func TestSecretScrubber_ScrubJSON(t *testing.T) {
	scrubber := NewSecretScrubber(DefaultScrubberOptions())

	rawJSON := []byte(`{
		"service": "billing",
		"credentials": {
			"api_key": "AKIAIOSFODNN7EXAMPLE",
			"active": true
		},
		"tags": ["prod", "us-east-1"]
	}`)

	cleanedBytes, err := scrubber.ScrubJSON(rawJSON)
	if err != nil {
		t.Fatalf("failed scrubbing JSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(cleanedBytes, &parsed); err != nil {
		t.Fatalf("failed unmarshaling scrubbed JSON: %v", err)
	}

	creds, ok := parsed["credentials"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected credentials map")
	}

	apiKeyVal, _ := creds["api_key"].(string)
	if !strings.Contains(apiKeyVal, "REDACTED") {
		t.Errorf("expected api_key to be redacted, got: %s", apiKeyVal)
	}
}

func TestSecretScrubber_ConcurrentSafety(t *testing.T) {
	scrubber := NewSecretScrubber(DefaultScrubberOptions())
	var wg sync.WaitGroup
	workers := 10

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				txt := "Key AKIAIOSFODNN7EXAMPLE and secret 8Fk9X2mQvL0pRt4wYz7uBn1aCe3dFg6h"
				res := scrubber.ScrubText(txt)
				if strings.Contains(res, "AKIAIOSFODNN7EXAMPLE") {
					t.Errorf("concurrent leak detected")
				}
			}
		}(w)
	}

	wg.Wait()
}
