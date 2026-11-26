package models

import (
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"
)

var (
	ErrWebhookMaxRetriesExceeded   = errors.New("webhook_sentinel: maximum delivery retries exceeded, dispatched to dead-letter queue")
	ErrWebhookEndpointQuarantined  = errors.New("webhook_sentinel: destination endpoint quarantined due to repeated failures")
	ErrWebhookPayloadOversized     = errors.New("webhook_sentinel: payload size exceeds maximum threshold")
	ErrWebhookInvalidURL           = errors.New("webhook_sentinel: target URL is invalid or malformed")
)

type WebhookDeliveryState string

const (
	WebhookStatePending     WebhookDeliveryState = "PENDING"
	WebhookStateDelivered   WebhookDeliveryState = "DELIVERED"
	WebhookStateRetrying    WebhookDeliveryState = "RETRYING"
	WebhookStateDeadLetter  WebhookDeliveryState = "DEAD_LETTER"
	WebhookStateQuarantined WebhookDeliveryState = "QUARANTINED"
)

// WebhookEventRecord tracks execution history and delivery metadata.
type WebhookEventRecord struct {
	EventID         string               `json:"event_id"`
	TargetURL       string               `json:"target_url"`
	PayloadSize     int                  `json:"payload_size"`
	Attempts        int                  `json:"attempts"`
	State           WebhookDeliveryState `json:"state"`
	LastStatusCode  int                  `json:"last_status_code"`
	LastAttemptTime time.Time            `json:"last_attempt_time"`
	FailureReason   string               `json:"failure_reason"`
}

// WebhookSentinelConfig configures delivery limits, timeouts, and quarantine thresholds.
type WebhookSentinelConfig struct {
	MaxRetries                  int
	MaxPayloadBytes             int
	EndpointQuarantineThreshold int
}

// WebhookDeliverySentinel defends workers against delivery retry storms and manages dead-letter isolation.
type WebhookDeliverySentinel struct {
	mu                   sync.RWMutex
	config               WebhookSentinelConfig
	events               map[string]*WebhookEventRecord
	endpointFailures     map[string]int
	quarantinedEndpoints map[string]time.Time
	dlqHistory           []*WebhookEventRecord
}

// NewWebhookDeliverySentinel constructs a sentinel with defensive default limits.
func NewWebhookDeliverySentinel(cfg WebhookSentinelConfig) *WebhookDeliverySentinel {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.MaxPayloadBytes <= 0 {
		cfg.MaxPayloadBytes = 1024 * 1024 // 1 MB
	}
	if cfg.EndpointQuarantineThreshold <= 0 {
		cfg.EndpointQuarantineThreshold = 5
	}

	return &WebhookDeliverySentinel{
		config:               cfg,
		events:               make(map[string]*WebhookEventRecord),
		endpointFailures:     make(map[string]int),
		quarantinedEndpoints: make(map[string]time.Time),
		dlqHistory:           make([]*WebhookEventRecord, 0),
	}
}

// RegisterEvent admits a new webhook notification into the delivery supervisor.
func (s *WebhookDeliverySentinel) RegisterEvent(eventID, targetURL string, payloadBytes int) (*WebhookEventRecord, error) {
	if eventID == "" {
		return nil, errors.New("webhook_sentinel: eventID cannot be empty")
	}

	u, err := url.ParseRequestURI(targetURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("%w: %s", ErrWebhookInvalidURL, targetURL)
	}

	if payloadBytes > s.config.MaxPayloadBytes {
		return nil, fmt.Errorf("%w: %d > %d", ErrWebhookPayloadOversized, payloadBytes, s.config.MaxPayloadBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if target endpoint is actively quarantined
	if _, quarantined := s.quarantinedEndpoints[targetURL]; quarantined {
		return nil, fmt.Errorf("%w: %s", ErrWebhookEndpointQuarantined, targetURL)
	}

	rec := &WebhookEventRecord{
		EventID:         eventID,
		TargetURL:       targetURL,
		PayloadSize:     payloadBytes,
		Attempts:        0,
		State:           WebhookStatePending,
		LastAttemptTime: time.Now(),
	}

	s.events[eventID] = rec
	return rec, nil
}

// RecordDeliveryAttempt evaluates HTTP response status and handles retry or DLQ routing.
func (s *WebhookDeliverySentinel) RecordDeliveryAttempt(eventID string, statusCode int, deliveryErr error) (*WebhookEventRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, exists := s.events[eventID]
	if !exists {
		return nil, errors.New("webhook_sentinel: event not found")
	}

	rec.Attempts++
	rec.LastStatusCode = statusCode
	rec.LastAttemptTime = time.Now()

	// HTTP 2xx -> Success
	if statusCode >= 200 && statusCode < 300 && deliveryErr == nil {
		rec.State = WebhookStateDelivered
		rec.FailureReason = ""
		s.endpointFailures[rec.TargetURL] = 0 // Reset failures on success
		return rec, nil
	}

	// Failure encountered
	reason := fmt.Sprintf("HTTP %d", statusCode)
	if deliveryErr != nil {
		reason = deliveryErr.Error()
	}
	rec.FailureReason = reason

	if rec.Attempts >= s.config.MaxRetries {
		// Route to DLQ
		rec.State = WebhookStateDeadLetter
		s.dlqHistory = append(s.dlqHistory, rec)

		// Increment endpoint failures
		s.endpointFailures[rec.TargetURL]++
		if s.endpointFailures[rec.TargetURL] >= s.config.EndpointQuarantineThreshold {
			s.quarantinedEndpoints[rec.TargetURL] = time.Now()
		}

		return rec, ErrWebhookMaxRetriesExceeded
	}

	// Still has retries remaining
	rec.State = WebhookStateRetrying
	return rec, nil
}

// GetDLQCount returns the number of dead-lettered events.
func (s *WebhookDeliverySentinel) GetDLQCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.dlqHistory)
}

// GeneratePostmortem compiles machine-readable incident diagnostics.
func (s *WebhookDeliverySentinel) GeneratePostmortem(incidentID string) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"incident_id":           incidentID,
		"timestamp_utc":         time.Now().UTC().Format(time.RFC3339),
		"status":                "RESOLVED",
		"total_events_tracked":  len(s.events),
		"dead_letter_count":     len(s.dlqHistory),
		"quarantined_endpoints": len(s.quarantinedEndpoints),
		"root_cause":            "Unbounded retry storms against offline webhook endpoints starved asynchronous worker connection pools.",
		"corrective_actions": []string{
			"Enforce strict 3-attempt retry ceiling with DLQ overflow routing",
			"Isolate destination endpoints in quarantine after 5 consecutive failures",
			"Bound maximum notification payload size to 1MB",
		},
	}
}
