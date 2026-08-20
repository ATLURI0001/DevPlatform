package models

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	RepoURL     string    `json:"repo_url"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateProjectRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description"`
	RepoURL     string `json:"repo_url" binding:"omitempty,url"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty" binding:"omitempty,min=1,max=255"`
	Description *string `json:"description,omitempty"`
	RepoURL     *string `json:"repo_url,omitempty" binding:"omitempty,url"`
}

type Environment struct {
	ID        uuid.UUID              `json:"id"`
	ProjectID uuid.UUID              `json:"project_id"`
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Cluster   string                 `json:"cluster"`
	Config    map[string]interface{} `json:"config"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type CreateEnvironmentRequest struct {
	Name      string                 `json:"name" binding:"required,min=1,max=100"`
	Namespace string                 `json:"namespace" binding:"required,min=1,max=100"`
	Cluster   string                 `json:"cluster"`
	Config    map[string]interface{} `json:"config"`
}

type Deployment struct {
	ID             uuid.UUID  `json:"id"`
	EnvironmentID  uuid.UUID  `json:"environment_id"`
	CommitSHA      string     `json:"commit_sha"`
	Branch         string     `json:"branch"`
	Status         string     `json:"status"`
	TriggeredBy    string     `json:"triggered_by"`
	Logs           string     `json:"logs"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

type DeployRequest struct {
	CommitSHA string `json:"commit_sha" binding:"required,len=40"`
	Branch    string `json:"branch"`
}

type AIJob struct {
	ID        uuid.UUID              `json:"id"`
	ProjectID uuid.UUID              `json:"project_id"`
	Type      string                 `json:"type"`
	Status    string                 `json:"status"`
	Input     interface{}            `json:"input"`
	Output    map[string]interface{} `json:"output,omitempty"`
	Error     string                 `json:"error,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	CompletedAt *time.Time           `json:"completed_at,omitempty"`
}

type AIReviewRequest struct {
	PRNumber    int      `json:"pr_number" binding:"required"`
	RepoURL     string   `json:"repo_url" binding:"required,url"`
	BaseBranch  string   `json:"base_branch" binding:"required"`
	HeadBranch  string   `json:"head_branch" binding:"required"`
	Files       []string `json:"files"`
	FocusAreas  []string `json:"focus_areas"` // security, performance, style, architecture
}

type AIDocsRequest struct {
	RepoURL     string   `json:"repo_url" binding:"required,url"`
	Paths       []string `json:"paths"`
	DocTypes    []string `json:"doc_types"` // api, readme, architecture, adr
	Format      string   `json:"format"`    // markdown, openapi, adoc
}

type AITestsRequest struct {
	RepoURL     string   `json:"repo_url" binding:"required,url"`
	Paths       []string `json:"paths"`
	TestTypes   []string `json:"test_types"` // unit, integration, e2e
	Framework   string   `json:"framework"`  // jest, pytest, gotest, etc.
	Coverage    int      `json:"coverage"`   // target coverage %
}

type AIMigrateRequest struct {
	RepoURL        string   `json:"repo_url" binding:"required,url"`
	SourceLang     string   `json:"source_lang" binding:"required"`
	TargetLang     string   `json:"target_lang" binding:"required"`
	Paths          []string `json:"paths"`
	PreserveTests  bool     `json:"preserve_tests"`
}

type WSMessage struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type TokenClaims struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	Exp    int64    `json:"exp"`
	Iat    int64    `json:"iat"`
}