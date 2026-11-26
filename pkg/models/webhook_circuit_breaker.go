package models

import (
	"errors"
	"sync"
	"time"
)

var ErrWebhookCircuitOpen = errors.New("webhook: delivery endpoint circuit open, queued for dead-letter retry")

// WebhookCircuitBreaker protects outbound notification dispatchers from dead webhook endpoints.
type WebhookCircuitBreaker struct {
	mu                sync.Mutex
	failureThreshold  int
	cooldown          time.Duration
	endpoints         map[string]*endpointCircuit
}

type endpointCircuit struct {
	state       string
	failures    int
	tripTime    time.Time
}

// NewWebhookCircuitBreaker creates a per-endpoint webhook circuit breaker.
func NewWebhookCircuitBreaker(failureThreshold int, cooldown time.Duration) *WebhookCircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 3
	}
	if cooldown <= 0 {
		cooldown = 2 * time.Second
	}
	return &WebhookCircuitBreaker{
		failureThreshold: failureThreshold,
		cooldown:         cooldown,
		endpoints:        make(map[string]*endpointCircuit),
	}
}

// AllowDispatch checks if an outbound webhook delivery can proceed.
func (cb *WebhookCircuitBreaker) AllowDispatch(endpoint string, now time.Time) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	ep, ok := cb.endpoints[endpoint]
	if !ok {
		cb.endpoints[endpoint] = &endpointCircuit{state: "CLOSED"}
		return nil
	}
	if ep.state == "OPEN" {
		if now.Sub(ep.tripTime) >= cb.cooldown {
			ep.state = "HALF_OPEN"
			return nil
		}
		return ErrWebhookCircuitOpen
	}
	return nil
}

// RecordOutcome records HTTP delivery success or failure.
func (cb *WebhookCircuitBreaker) RecordOutcome(endpoint string, success bool, now time.Time) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	ep, ok := cb.endpoints[endpoint]
	if !ok {
		ep = &endpointCircuit{state: "CLOSED"}
		cb.endpoints[endpoint] = ep
	}
	if success {
		ep.state = "CLOSED"
		ep.failures = 0
		return
	}
	ep.failures++
	if ep.state == "HALF_OPEN" || ep.failures >= cb.failureThreshold {
		ep.state = "OPEN"
		ep.tripTime = now
	}
}
