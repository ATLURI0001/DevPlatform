package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/atlur/devplatform/internal/ai"
	"github.com/atlur/devplatform/internal/auth"
	"github.com/atlur/devplatform/internal/gitops"
	"github.com/atlur/devplatform/internal/worker"
	"github.com/atlur/devplatform/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

type Platform struct {
	config      *Config
	db          *pgxpool.Pool
	nats        *nats.Conn
	redis       *redis.Client
	aiClient    *ai.Client
	gitops      *gitops.Manager
	workerPool  *worker.Pool
	wsUpgrader  websocket.Upgrader
	wsClients   map[string]*websocket.Conn
	logger      zerolog.Logger
}

func New(cfg *Config) (*Platform, error) {
	ctx := context.Background()

	// Database
	dbConfig, err := pgxpool.ParseConfig(cfg.Database.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}
	dbConfig.MaxConns = int32(cfg.Database.MaxConns)
	db, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	// NATS
	nc, err := nats.Connect(cfg.NATS.URL)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	// Redis
	redisOpt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	redisClient := redis.NewClient(redisOpt)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	// AI Client
	aiClient, err := ai.NewClient(ai.Config{
		Provider:   cfg.AI.Provider,
		APIKey:     cfg.AI.APIKey,
		Model:      cfg.AI.Model,
		BaseURL:    cfg.AI.BaseURL,
		MaxTokens:  cfg.AI.MaxTokens,
		Temperature: cfg.AI.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("init ai client: %w", err)
	}

	// GitOps Manager
	gitopsMgr, err := gitops.NewManager(gitops.Config{
		GitURL:      viper.GetString("gitops.git_url"),
		GitToken:    viper.GetString("gitops.git_token"),
		BasePath:    viper.GetString("gitops.base_path"),
		ArgoCDAddr:  viper.GetString("gitops.argocd_addr"),
		ArgoCDToken: viper.GetString("gitops.argocd_token"),
	})
	if err != nil {
		return nil, fmt.Errorf("init gitops: %w", err)
	}

	// Worker Pool
	wp := worker.NewPool(worker.Config{
		NATS:     nc,
		Redis:    redisClient,
		DB:       db,
		AIClient: aiClient,
		GitOps:   gitopsMgr,
	})

	plt := &Platform{
		config:     cfg,
		db:         db,
		nats:       nc,
		redis:      redisClient,
		aiClient:   aiClient,
		gitops:     gitopsMgr,
		workerPool: wp,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		wsClients: make(map[string]*websocket.Conn),
		logger:    zerolog.Logger{},
	}

	// Start workers
	go wp.Start(ctx)

	// Run migrations
	if err := plt.runMigrations(ctx); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return plt, nil
}

func (p *Platform) Close() error {
	p.workerPool.Stop()
	p.nats.Close()
	p.redis.Close()
	p.db.Close()
	return nil
}

func (p *Platform) runMigrations(ctx context.Context) error {
	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name VARCHAR(255) NOT NULL,
		description TEXT,
		repo_url VARCHAR(500),
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS environments (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
		name VARCHAR(100) NOT NULL,
		namespace VARCHAR(100) NOT NULL,
		cluster VARCHAR(100),
		config JSONB DEFAULT '{}',
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(project_id, name)
	);

	CREATE TABLE IF NOT EXISTS deployments (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		environment_id UUID REFERENCES environments(id) ON DELETE CASCADE,
		commit_sha VARCHAR(64) NOT NULL,
		branch VARCHAR(255),
		status VARCHAR(50) NOT NULL DEFAULT 'pending',
		triggered_by VARCHAR(255),
		logs TEXT,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		started_at TIMESTAMPTZ,
		completed_at TIMESTAMPTZ
	);

	CREATE TABLE IF NOT EXISTS ai_jobs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
		type VARCHAR(50) NOT NULL,
		status VARCHAR(50) NOT NULL DEFAULT 'pending',
		input JSONB NOT NULL,
		output JSONB,
		error TEXT,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		completed_at TIMESTAMPTZ
	);

	CREATE INDEX IF NOT EXISTS idx_environments_project ON environments(project_id);
	CREATE INDEX IF NOT EXISTS idx_deployments_environment ON deployments(environment_id);
	CREATE INDEX IF NOT EXISTS idx_ai_jobs_project ON ai_jobs(project_id);
	`

	_, err := p.db.Exec(ctx, schema)
	return err
}

// HealthHandler returns health status
func (p *Platform) HealthHandler(c *gin.Context) {
	ctx := c.Request.Context()

	healthy := true
	checks := map[string]string{}

	if err := p.db.Ping(ctx); err != nil {
		healthy = false
		checks["database"] = "unhealthy: " + err.Error()
	} else {
		checks["database"] = "healthy"
	}

	if p.nats.IsConnected() {
		checks["nats"] = "healthy"
	} else {
		healthy = false
		checks["nats"] = "unhealthy"
	}

	if err := p.redis.Ping(ctx).Err(); err != nil {
		healthy = false
		checks["redis"] = "unhealthy: " + err.Error()
	} else {
		checks["redis"] = "healthy"
	}

	status := "healthy"
	if !healthy {
		status = "unhealthy"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  status,
		"checks":  checks,
		"version": "1.0.0",
	})
}

// ReadyHandler returns readiness status
func (p *Platform) ReadyHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ready": true})
}

// AuthMiddleware validates JWT tokens
func (p *Platform) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			return
		}

		claims, err := auth.ValidateToken(parts[1], p.config.Auth.JWTSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_roles", claims.Roles)
		c.Next()
	}
}

// Project handlers
func (p *Platform) CreateProject(c *gin.Context) {
	var req models.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")
	project := &models.Project{
		Name:        req.Name,
		Description: req.Description,
		RepoURL:     req.RepoURL,
		OwnerID:     userID,
	}

	ctx := c.Request.Context()
	if err := p.createProject(ctx, project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, project)
}

func (p *Platform) createProject(ctx context.Context, project *models.Project) error {
	query := `
		INSERT INTO projects (name, description, repo_url, owner_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	return p.db.QueryRow(ctx, query, project.Name, project.Description, project.RepoURL, project.OwnerID).
		Scan(&project.ID, &project.CreatedAt, &project.UpdatedAt)
}

func (p *Platform) ListProjects(c *gin.Context) {
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	query := `SELECT id, name, description, repo_url, owner_id, created_at, updated_at FROM projects WHERE owner_id = $1 ORDER BY created_at DESC`
	rows, err := p.db.Query(ctx, query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.OwnerID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		projects = append(projects, p)
	}

	c.JSON(http.StatusOK, projects)
}

func (p *Platform) GetProject(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	query := `SELECT id, name, description, repo_url, owner_id, created_at, updated_at FROM projects WHERE id = $1 AND owner_id = $2`
	var project models.Project
	err := p.db.QueryRow(ctx, query, id, userID).
		Scan(&project.ID, &project.Name, &project.Description, &project.RepoURL, &project.OwnerID, &project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	c.JSON(http.StatusOK, project)
}

func (p *Platform) UpdateProject(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	var req models.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	query := `UPDATE projects SET name = COALESCE($1, name), description = COALESCE($2, description), repo_url = COALESCE($3, repo_url), updated_at = NOW() WHERE id = $4 AND owner_id = $5 RETURNING updated_at`

	var updatedAt time.Time
	err := p.db.QueryRow(ctx, query, req.Name, req.Description, req.RepoURL, id, userID).Scan(&updatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	uid, _ := uuid.Parse(id)
	var pName, pDesc, pRepo string
	if req.Name != nil {
		pName = *req.Name
	}
	if req.Description != nil {
		pDesc = *req.Description
	}
	if req.RepoURL != nil {
		pRepo = *req.RepoURL
	}
	project := models.Project{ID: uid, Name: pName, Description: pDesc, RepoURL: pRepo, UpdatedAt: updatedAt}
	c.JSON(http.StatusOK, project)
}

func (p *Platform) DeleteProject(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	result, err := p.db.Exec(ctx, `DELETE FROM projects WHERE id = $1 AND owner_id = $2`, id, userID)
	if err != nil || result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Environment handlers
func (p *Platform) CreateEnvironment(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.GetString("user_id")

	var req models.CreateEnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify project ownership
	ctx := c.Request.Context()
	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	pUUID, _ := uuid.Parse(projectID)
	env := &models.Environment{
		ProjectID: pUUID,
		Name:      req.Name,
		Namespace: req.Namespace,
		Cluster:   req.Cluster,
		Config:    req.Config,
	}

	if err := p.createEnvironment(ctx, env); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, env)
}

func (p *Platform) createEnvironment(ctx context.Context, env *models.Environment) error {
	configJSON, _ := json.Marshal(env.Config)
	query := `
		INSERT INTO environments (project_id, name, namespace, cluster, config)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	return p.db.QueryRow(ctx, query, env.ProjectID, env.Name, env.Namespace, env.Cluster, configJSON).
		Scan(&env.ID, &env.CreatedAt, &env.UpdatedAt)
}

func (p *Platform) ListEnvironments(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	// Verify ownership
	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	query := `SELECT id, project_id, name, namespace, cluster, config, created_at, updated_at FROM environments WHERE project_id = $1 ORDER BY created_at DESC`
	rows, err := p.db.Query(ctx, query, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var environments []models.Environment
	for rows.Next() {
		var e models.Environment
		var configJSON []byte
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Namespace, &e.Cluster, &configJSON, &e.CreatedAt, &e.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		json.Unmarshal(configJSON, &e.Config)
		environments = append(environments, e)
	}

	c.JSON(http.StatusOK, environments)
}

func (p *Platform) GetEnvironment(c *gin.Context) {
	projectID := c.Param("id")
	envID := c.Param("envId")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	query := `SELECT id, project_id, name, namespace, cluster, config, created_at, updated_at FROM environments WHERE id = $1 AND project_id = $2`
	var env models.Environment
	var configJSON []byte
	err = p.db.QueryRow(ctx, query, envID, projectID).
		Scan(&env.ID, &env.ProjectID, &env.Name, &env.Namespace, &env.Cluster, &configJSON, &env.CreatedAt, &env.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "environment not found"})
		return
	}
	json.Unmarshal(configJSON, &env.Config)

	c.JSON(http.StatusOK, env)
}

func (p *Platform) DeleteEnvironment(c *gin.Context) {
	projectID := c.Param("id")
	envID := c.Param("envId")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	result, err := p.db.Exec(ctx, `DELETE FROM environments WHERE id = $1 AND project_id = $2`, envID, projectID)
	if err != nil || result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "environment not found"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (p *Platform) DeployEnvironment(c *gin.Context) {
	projectID := c.Param("id")
	envID := c.Param("envId")
	userID := c.GetString("user_id")

	var req models.DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Verify ownership
	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	eUUID, _ := uuid.Parse(envID)
	// Create deployment record
	deployment := &models.Deployment{
		EnvironmentID: eUUID,
		CommitSHA:     req.CommitSHA,
		Branch:        req.Branch,
		Status:        "pending",
		TriggeredBy:   userID,
	}

	if err := p.createDeployment(ctx, deployment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Queue deployment job
	jobData := map[string]interface{}{
		"deployment_id": deployment.ID,
		"project_id":    projectID,
		"environment_id": envID,
		"commit_sha":    req.CommitSHA,
		"branch":        req.Branch,
	}
	jobBytes, _ := json.Marshal(jobData)

	if err := p.nats.Publish("deployments.create", jobBytes); err != nil {
		log.Printf("Failed to publish deployment job: %v", err)
	}

	c.JSON(http.StatusAccepted, deployment)
}

func (p *Platform) createDeployment(ctx context.Context, d *models.Deployment) error {
	query := `
		INSERT INTO deployments (environment_id, commit_sha, branch, status, triggered_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	return p.db.QueryRow(ctx, query, d.EnvironmentID, d.CommitSHA, d.Branch, d.Status, d.TriggeredBy).
		Scan(&d.ID, &d.CreatedAt)
}

func (p *Platform) ListDeployments(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	query := `
		SELECT d.id, d.environment_id, d.commit_sha, d.branch, d.status, d.triggered_by, d.logs, d.created_at, d.started_at, d.completed_at
		FROM deployments d
		JOIN environments e ON d.environment_id = e.id
		WHERE e.project_id = $1
		ORDER BY d.created_at DESC
		LIMIT 50
	`
	rows, err := p.db.Query(ctx, query, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var deployments []models.Deployment
	for rows.Next() {
		var d models.Deployment
		if err := rows.Scan(&d.ID, &d.EnvironmentID, &d.CommitSHA, &d.Branch, &d.Status, &d.TriggeredBy, &d.Logs, &d.CreatedAt, &d.StartedAt, &d.CompletedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		deployments = append(deployments, d)
	}

	c.JSON(http.StatusOK, deployments)
}

func (p *Platform) GetDeployment(c *gin.Context) {
	projectID := c.Param("id")
	deploymentID := c.Param("deploymentId")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	query := `
		SELECT d.id, d.environment_id, d.commit_sha, d.branch, d.status, d.triggered_by, d.logs, d.created_at, d.started_at, d.completed_at
		FROM deployments d
		JOIN environments e ON d.environment_id = e.id
		WHERE d.id = $1 AND e.project_id = $2
	`
	var d models.Deployment
	err = p.db.QueryRow(ctx, query, deploymentID, projectID).
		Scan(&d.ID, &d.EnvironmentID, &d.CommitSHA, &d.Branch, &d.Status, &d.TriggeredBy, &d.Logs, &d.CreatedAt, &d.StartedAt, &d.CompletedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	c.JSON(http.StatusOK, d)
}

func (p *Platform) GetDeploymentLogs(c *gin.Context) {
	// Stream logs via WebSocket or return recent logs
	projectID := c.Param("id")
	deploymentID := c.Param("deploymentId")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// Get logs from Redis stream
	logs, err := p.redis.XRange(ctx, "deployment:logs:"+deploymentID, "-", "+").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var logEntries []gin.H
	for _, msg := range logs {
		logEntries = append(logEntries, gin.H{
			"id":        msg.ID,
			"timestamp": msg.Values["timestamp"],
			"level":     msg.Values["level"],
			"message":   msg.Values["message"],
		})
	}

	c.JSON(http.StatusOK, gin.H{"logs": logEntries})
}

// AI Handlers
func (p *Platform) AIReviewPR(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.GetString("user_id")

	var req models.AIReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Verify ownership
	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	pjUUID, _ := uuid.Parse(projectID)
	// Create AI job
	job := &models.AIJob{
		ProjectID: pjUUID,
		Type:      "pr_review",
		Status:    "pending",
		Input:     req,
	}

	if err := p.createAIJob(ctx, job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Queue AI job
	jobData := map[string]interface{}{
		"job_id":     job.ID,
		"type":       "pr_review",
		"project_id": projectID,
		"input":      req,
	}
	jobBytes, _ := json.Marshal(jobData)
	p.nats.Publish("ai.jobs", jobBytes)

	c.JSON(http.StatusAccepted, job)
}

func (p *Platform) AIGenerateDocs(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.GetString("user_id")

	var req models.AIDocsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	pjUUID, _ := uuid.Parse(projectID)
	job := &models.AIJob{
		ProjectID: pjUUID,
		Type:      "generate_docs",
		Status:    "pending",
		Input:     req,
	}

	if err := p.createAIJob(ctx, job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	jobData := map[string]interface{}{
		"job_id":     job.ID,
		"type":       "generate_docs",
		"project_id": projectID,
		"input":      req,
	}
	jobBytes, _ := json.Marshal(jobData)
	p.nats.Publish("ai.jobs", jobBytes)

	c.JSON(http.StatusAccepted, job)
}

func (p *Platform) AIGenerateTests(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.GetString("user_id")

	var req models.AITestsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	pjUUID, _ := uuid.Parse(projectID)
	job := &models.AIJob{
		ProjectID: pjUUID,
		Type:      "generate_tests",
		Status:    "pending",
		Input:     req,
	}

	if err := p.createAIJob(ctx, job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	jobData := map[string]interface{}{
		"job_id":     job.ID,
		"type":       "generate_tests",
		"project_id": projectID,
		"input":      req,
	}
	jobBytes, _ := json.Marshal(jobData)
	p.nats.Publish("ai.jobs", jobBytes)

	c.JSON(http.StatusAccepted, job)
}

func (p *Platform) AIMigrateCode(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.GetString("user_id")

	var req models.AIMigrateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	pjUUID, _ := uuid.Parse(projectID)
	job := &models.AIJob{
		ProjectID: pjUUID,
		Type:      "migrate_code",
		Status:    "pending",
		Input:     req,
	}

	if err := p.createAIJob(ctx, job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	jobData := map[string]interface{}{
		"job_id":     job.ID,
		"type":       "migrate_code",
		"project_id": projectID,
		"input":      req,
	}
	jobBytes, _ := json.Marshal(jobData)
	p.nats.Publish("ai.jobs", jobBytes)

	c.JSON(http.StatusAccepted, job)
}

func (p *Platform) createAIJob(ctx context.Context, job *models.AIJob) error {
	inputJSON, _ := json.Marshal(job.Input)
	query := `
		INSERT INTO ai_jobs (project_id, type, status, input)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`
	return p.db.QueryRow(ctx, query, job.ProjectID, job.Type, job.Status, inputJSON).
		Scan(&job.ID, &job.CreatedAt)
}

// WebSocket Handler
func (p *Platform) WebSocketHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	conn, err := p.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		p.logger.Error().Err(err).Msg("WebSocket upgrade failed")
		return
	}

	clientID := generateClientID()
	p.wsClients[clientID] = conn
	defer func() {
		delete(p.wsClients, clientID)
		conn.Close()
	}()

	p.logger.Info().Str("client_id", clientID).Str("user_id", userID).Msg("WebSocket connected")

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				p.logger.Error().Err(err).Msg("WebSocket error")
			}
			break
		}

		// Handle incoming messages (ping/pong, subscriptions)
		var wsMsg models.WSMessage
		if err := json.Unmarshal(msg, &wsMsg); err != nil {
			continue
		}

		switch wsMsg.Type {
		case "ping":
			conn.WriteJSON(models.WSMessage{Type: "pong", Data: map[string]interface{}{"time": time.Now()}})
		case "subscribe":
			// Handle subscription to deployment logs, etc.
		}
	}
}

func generateClientID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// BroadcastToProject sends a message to all clients watching a project
func (p *Platform) BroadcastToProject(projectID string, msg models.WSMessage) {
	// In a real implementation, track which clients watch which projects
	data, _ := json.Marshal(msg)
	for _, conn := range p.wsClients {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}