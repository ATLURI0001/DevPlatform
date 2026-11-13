package worker

import (
	"testing"

	"github.com/nats-io/nats.go"
)

func TestHandleMalformedDeploymentJobDoesNotPanic(t *testing.T) {
	p := NewPool(Config{})

	// Nil message
	p.handleDeployment(nil)

	// Empty message data
	p.handleDeployment(&nats.Msg{Data: []byte{}})

	// Malformed JSON
	p.handleDeployment(&nats.Msg{Data: []byte("not-json")})

	// Missing fields or non-string types
	p.handleDeployment(&nats.Msg{Data: []byte(`{"deployment_id": 123, "project_id": null}`)})
	p.handleDeployment(&nats.Msg{Data: []byte(`{"deployment_id": "dep-1"}`)})
}

func TestHandleMalformedAIJobDoesNotPanic(t *testing.T) {
	p := NewPool(Config{})

	// Nil message
	p.handleAIJob(nil)

	// Empty message data
	p.handleAIJob(&nats.Msg{Data: []byte{}})

	// Invalid JSON
	p.handleAIJob(&nats.Msg{Data: []byte("{invalid}")})

	// Missing job_id or type
	p.handleAIJob(&nats.Msg{Data: []byte(`{"project_id": "proj-1"}`)})

	// Invalid input type for PR review
	_, err := p.handlePRReview("not-a-map")
	if err == nil {
		t.Fatal("expected error for non-map PR review input")
	}

	_, err = p.handlePRReview(map[string]interface{}{
		"repo_url":    "https://github.com/example/repo",
		"pr_number":   42.0,
		"base_branch": "main",
		"head_branch": "feat",
	})
	if err != nil {
		t.Fatalf("unexpected error for valid PR review input: %v", err)
	}
}
