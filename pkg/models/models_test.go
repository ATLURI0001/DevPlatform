package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestProjectJSONSerialization(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	proj := Project{
		ID:          id,
		Name:        "DevPlatform Control Plane",
		Description: "Central orchestration plane for microservices",
		RepoURL:     "https://github.com/ATLURI0001/DevPlatform",
		OwnerID:     "user-12345",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	data, err := json.Marshal(proj)
	if err != nil {
		t.Fatalf("failed to marshal project: %v", err)
	}

	var decoded Project
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal project: %v", err)
	}

	if decoded.ID != proj.ID {
		t.Errorf("expected ID %s, got %s", proj.ID, decoded.ID)
	}
	if decoded.Name != proj.Name {
		t.Errorf("expected Name %q, got %q", proj.Name, decoded.Name)
	}
	if decoded.RepoURL != proj.RepoURL {
		t.Errorf("expected RepoURL %q, got %q", proj.RepoURL, decoded.RepoURL)
	}
}

func TestUpdateProjectRequestPartialFields(t *testing.T) {
	name := "Updated Name"
	req := UpdateProjectRequest{
		Name: &name,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal UpdateProjectRequest: %v", err)
	}

	var decoded UpdateProjectRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal UpdateProjectRequest: %v", err)
	}

	if decoded.Name == nil || *decoded.Name != name {
		t.Errorf("expected name %q, got %v", name, decoded.Name)
	}
	if decoded.Description != nil {
		t.Errorf("expected nil Description, got %v", decoded.Description)
	}
	if decoded.RepoURL != nil {
		t.Errorf("expected nil RepoURL, got %v", decoded.RepoURL)
	}
}

func TestAIReviewRequestSerialization(t *testing.T) {
	req := AIReviewRequest{
		PRNumber:   42,
		RepoURL:    "https://github.com/ATLURI0001/DevPlatform",
		BaseBranch: "main",
		HeadBranch: "feature/ci",
		Files:      []string{"internal/ai/client.go", "pkg/models/models.go"},
		FocusAreas: []string{"security", "performance"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal AIReviewRequest: %v", err)
	}

	var decoded AIReviewRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal AIReviewRequest: %v", err)
	}

	if decoded.PRNumber != 42 {
		t.Errorf("expected PRNumber 42, got %d", decoded.PRNumber)
	}
	if len(decoded.Files) != 2 || decoded.Files[0] != "internal/ai/client.go" {
		t.Errorf("unexpected files slice: %v", decoded.Files)
	}
	if len(decoded.FocusAreas) != 2 || decoded.FocusAreas[1] != "performance" {
		t.Errorf("unexpected focus areas: %v", decoded.FocusAreas)
	}
}

func TestDeploymentStatusAndTimestamps(t *testing.T) {
	now := time.Now().UTC()
	depID := uuid.New()
	envID := uuid.New()

	dep := Deployment{
		ID:            depID,
		EnvironmentID: envID,
		CommitSHA:     "a1b2c3d4e5f6789012345678901234567890abcd",
		Branch:        "main",
		Status:        "running",
		TriggeredBy:   "github-action",
		Logs:          "Starting container deployment...",
		CreatedAt:     now,
		StartedAt:     &now,
		CompletedAt:   nil,
	}

	data, err := json.Marshal(dep)
	if err != nil {
		t.Fatalf("failed to marshal Deployment: %v", err)
	}

	var decoded Deployment
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Deployment: %v", err)
	}

	if decoded.CompletedAt != nil {
		t.Errorf("expected nil CompletedAt for running deployment, got %v", decoded.CompletedAt)
	}
	if decoded.StartedAt == nil {
		t.Errorf("expected non-nil StartedAt")
	}
}
