package security

import "testing"

func TestValidateWebhookPayload(t *testing.T) {
	if err := ValidateWebhookPayload(WebhookPayload{Event: "push", Signature: "sig", Timestamp: 100}); err != nil {
		t.Fatalf("expected valid webhook, got %v", err)
	}
	if err := ValidateWebhookPayload(WebhookPayload{Event: "", Signature: "sig", Timestamp: 100}); err == nil {
		t.Fatal("expected contract violation for empty event")
	}
}
