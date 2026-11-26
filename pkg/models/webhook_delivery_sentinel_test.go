package models

import (
	"errors"
	"testing"
)

func TestWebhookDeliverySentinel_SuccessfulDelivery(t *testing.T) {
	sentinel := NewWebhookDeliverySentinel(WebhookSentinelConfig{
		MaxRetries: 3,
	})

	rec, err := sentinel.RegisterEvent("evt-100", "https://api.customer.com/webhooks", 2048)
	if err != nil {
		t.Fatalf("unexpected error registering webhook event: %v", err)
	}

	if rec.State != WebhookStatePending {
		t.Fatalf("expected PENDING state, got %s", rec.State)
	}

	delivered, err := sentinel.RecordDeliveryAttempt("evt-100", 200, nil)
	if err != nil {
		t.Fatalf("unexpected error on successful attempt: %v", err)
	}

	if delivered.State != WebhookStateDelivered {
		t.Fatalf("expected DELIVERED state, got %s", delivered.State)
	}
	if delivered.Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", delivered.Attempts)
	}
}

func TestWebhookDeliverySentinel_RetryAndDLQRouting(t *testing.T) {
	sentinel := NewWebhookDeliverySentinel(WebhookSentinelConfig{
		MaxRetries:                  2,
		EndpointQuarantineThreshold: 2,
	})

	targetURL := "https://offline.service.org/hook"

	// Event 1 fails twice -> routed to DLQ
	_, err := sentinel.RegisterEvent("evt-201", targetURL, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r1, err1 := sentinel.RecordDeliveryAttempt("evt-201", 500, errors.New("internal server error"))
	if err1 != nil {
		t.Fatalf("unexpected error on attempt 1: %v", err1)
	}
	if r1.State != WebhookStateRetrying {
		t.Fatalf("expected RETRYING on attempt 1, got %s", r1.State)
	}

	r2, err2 := sentinel.RecordDeliveryAttempt("evt-201", 503, errors.New("service unavailable"))
	if !errors.Is(err2, ErrWebhookMaxRetriesExceeded) {
		t.Fatalf("expected ErrWebhookMaxRetriesExceeded on attempt 2, got %v", err2)
	}
	if r2.State != WebhookStateDeadLetter {
		t.Fatalf("expected DEAD_LETTER state, got %s", r2.State)
	}
	if sentinel.GetDLQCount() != 1 {
		t.Fatalf("expected 1 DLQ event, got %d", sentinel.GetDLQCount())
	}
}

func TestWebhookDeliverySentinel_EndpointQuarantine(t *testing.T) {
	sentinel := NewWebhookDeliverySentinel(WebhookSentinelConfig{
		MaxRetries:                  1,
		EndpointQuarantineThreshold: 1,
	})

	badURL := "https://blackhole.broken.com/webhook"

	sentinel.RegisterEvent("evt-bad-1", badURL, 500)
	sentinel.RecordDeliveryAttempt("evt-bad-1", 500, errors.New("failed"))

	// Subsequent registration to badURL must be blocked by quarantine
	_, errBlock := sentinel.RegisterEvent("evt-bad-2", badURL, 500)
	if !errors.Is(errBlock, ErrWebhookEndpointQuarantined) {
		t.Fatalf("expected ErrWebhookEndpointQuarantined, got %v", errBlock)
	}

	// Postmortem report check
	report := sentinel.GeneratePostmortem("INC-2026-DEVPLAT-9921")
	if report["incident_id"] != "INC-2026-DEVPLAT-9921" {
		t.Errorf("unexpected incident ID: %v", report["incident_id"])
	}
	if report["quarantined_endpoints"].(int) != 1 {
		t.Errorf("expected 1 quarantined endpoint, got %v", report["quarantined_endpoints"])
	}
}
