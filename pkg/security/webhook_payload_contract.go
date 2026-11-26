package security

import "fmt"

// WebhookPayload represents an incoming webhook event.
type WebhookPayload struct {
	Event     string
	Signature string
	Timestamp int64
}

// ValidateWebhookPayload enforces required event, signature, and positive timestamp.
func ValidateWebhookPayload(p WebhookPayload) error {
	if p.Event == "" {
		return fmt.Errorf("contract violation: webhook event required")
	}
	if p.Signature == "" {
		return fmt.Errorf("contract violation: webhook signature required")
	}
	if p.Timestamp <= 0 {
		return fmt.Errorf("contract violation: webhook timestamp must be positive")
	}
	return nil
}
