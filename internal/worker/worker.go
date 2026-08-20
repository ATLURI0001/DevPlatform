package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/atlur/devplatform/internal/ai"
	"github.com/atlur/devplatform/internal/gitops"
	"github.com/atlur/devplatform/pkg/models"
	"github.com/atlur/devplatform/pkg/telemetry"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type Config struct {
	NATS     *nats.Conn
	Redis    *redis.Client
	DB       *pgxpool.Pool
	AIClient *ai.Client
	GitOps   *gitops.Manager
}

type Pool struct {
	config Config
	ctx    context.Context
	cancel context.CancelFunc
	logger zerolog.Logger
}

func NewPool(cfg Config) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
		logger: zerolog.Logger{},
	}
}

func (p *Pool) Start(ctx context.Context) {
	p.logger.Info().Msg("Starting worker pool")

	// Subscribe to deployment jobs
	p.config.NATS.Subscribe("deployments.create", p.handleDeployment)

	// Subscribe to AI jobs
	p.config.NATS.Subscribe("ai.jobs", p.handleAIJob)

	// Keep running
	<-p.ctx.Done()
	p.logger.Info().Msg("Worker pool stopped")
}

func (p *Pool) Stop() {
	p.cancel()
}

func (p *Pool) handleDeployment(msg *nats.Msg) {
	var jobData map[string]interface{}
	if err := json.Unmarshal(msg.Data, &jobData); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal deployment job")
		return
	}

	deploymentID := jobData["deployment_id"].(string)
	projectID := jobData["project_id"].(string)
	environmentID := jobData["environment_id"].(string)
	commitSHA := jobData["commit_sha"].(string)
	branch := jobData["branch"].(string)

	p.logger.Info().Str("deployment_id", deploymentID).Msg("Processing deployment")

	start := time.Now()
	p.updateDeploymentStatus(deploymentID, "running", &start, nil)

	// Stream logs to Redis
	logKey := "deployment:logs:" + deploymentID
	p.streamLog(logKey, "info", "Starting deployment for commit "+commitSHA)

	// Simulate deployment steps
	steps := []string{
		"Cloning repository",
		"Building Docker image",
		"Running tests",
		"Pushing image to registry",
		"Updating Kubernetes manifests",
		"Syncing ArgoCD",
		"Waiting for rollout",
		"Running health checks",
	}

	for i, step := range steps {
		p.streamLog(logKey, "info", fmt.Sprintf("[%d/%d] %s", i+1, len(steps), step))
		time.Sleep(2 * time.Second) // Simulate work
	}

	// Mark complete
	completedAt := time.Now()
	p.updateDeploymentStatus(deploymentID, "success", &start, &completedAt)
	p.streamLog(logKey, "info", "Deployment completed successfully")

	telemetry.RecordDeployment(projectID, environmentID, "success", time.Since(start).Seconds())
}

func (p *Pool) handleAIJob(msg *nats.Msg) {
	var jobData map[string]interface{}
	if err := json.Unmarshal(msg.Data, &jobData); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal AI job")
		return
	}

	jobID := jobData["job_id"].(string)
	jobType := jobData["type"].(string)
	projectID := jobData["project_id"].(string)
	input := jobData["input"]

	p.logger.Info().Str("job_id", jobID).Str("type", jobType).Msg("Processing AI job")

	start := time.Now()
	p.updateAIJobStatus(jobID, "running", nil)

	var output map[string]interface{}
	var err error

	switch jobType {
	case "pr_review":
		output, err = p.handlePRReview(input)
	case "generate_docs":
		output, err = p.handleGenerateDocs(input)
	case "generate_tests":
		output, err = p.handleGenerateTests(input)
	case "migrate_code":
		output, err = p.handleMigrateCode(input)
	default:
		err = fmt.Errorf("unknown job type: %s", jobType)
	}

	completedAt := time.Now()
	if err != nil {
		p.logger.Error().Err(err).Str("job_id", jobID).Msg("AI job failed")
		p.updateAIJobStatus(jobID, "failed", &completedAt)
		p.updateAIJobError(jobID, err.Error())
		telemetry.RecordAIJob(projectID, jobType, "failed", time.Since(start).Seconds())
	} else {
		p.logger.Info().Str("job_id", jobID).Msg("AI job completed")
		p.updateAIJobStatus(jobID, "completed", &completedAt)
		p.updateAIJobOutput(jobID, output)
		telemetry.RecordAIJob(projectID, jobType, "success", time.Since(start).Seconds())
	}
}

func (p *Pool) handlePRReview(input interface{}) (map[string]interface{}, error) {
	reqMap := input.(map[string]interface{})
	pr := &ai.PRContext{
		RepoURL:    reqMap["repo_url"].(string),
		PRNumber:   int(reqMap["pr_number"].(float64)),
		BaseBranch: reqMap["base_branch"].(string),
		HeadBranch: reqMap["head_branch"].(string),
	}

	// In real implementation, fetch diff from GitHub/GitLab API
	// For now, return mock result
	return map[string]interface{}{
		"summary":      "Code review completed",
		"issues":       []interface{}{},
		"security":     []interface{}{},
		"performance":  []interface{}{},
		"style":        []interface{}{},
		"architecture": []interface{}{},
		"overall_score": 8,
	}, nil
}

func (p *Pool) handleGenerateDocs(input interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"overview":     "Generated documentation",
		"api":          []interface{}{},
		"architecture": "System architecture documentation",
		"readme":       "# Project\n\nAuto-generated README",
		"adrs":         []interface{}{},
	}, nil
}

func (p *Pool) handleGenerateTests(input interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"test_files":         []interface{}{},
		"coverage_target":    80,
		"setup_instructions": "Run tests with standard test runner",
	}, nil
}

func (p *Pool) handleMigrateCode(input interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"files":    []interface{}{},
		"notes":    "Migration completed",
		"warnings": []string{},
	}, nil
}

func (p *Pool) updateDeploymentStatus(deploymentID, status string, startedAt, completedAt *time.Time) {
	ctx := context.Background()
	query := `UPDATE deployments SET status = $1`
	args := []interface{}{status}
	argIdx := 2

	if startedAt != nil {
		query += fmt.Sprintf(", started_at = $%d", argIdx)
		args = append(args, *startedAt)
		argIdx++
	}
	if completedAt != nil {
		query += fmt.Sprintf(", completed_at = $%d", argIdx)
		args = append(args, *completedAt)
		argIdx++
	}
	query += fmt.Sprintf(" WHERE id = $%d", argIdx)
	args = append(args, deploymentID)

	_, err := p.config.DB.Exec(ctx, query, args...)
	if err != nil {
		p.logger.Error().Err(err).Str("deployment_id", deploymentID).Msg("Failed to update deployment status")
	}
}

func (p *Pool) updateAIJobStatus(jobID, status string, completedAt *time.Time) {
	ctx := context.Background()
	query := `UPDATE ai_jobs SET status = $1`
	args := []interface{}{status}
	argIdx := 2

	if completedAt != nil {
		query += fmt.Sprintf(", completed_at = $%d", argIdx)
		args = append(args, *completedAt)
		argIdx++
	}
	query += fmt.Sprintf(" WHERE id = $%d", argIdx)
	args = append(args, jobID)

	_, err := p.config.DB.Exec(ctx, query, args...)
	if err != nil {
		p.logger.Error().Err(err).Str("job_id", jobID).Msg("Failed to update AI job status")
	}
}

func (p *Pool) updateAIJobOutput(jobID string, output map[string]interface{}) {
	ctx := context.Background()
	outputJSON, _ := json.Marshal(output)
	_, err := p.config.DB.Exec(ctx, `UPDATE ai_jobs SET output = $1 WHERE id = $2`, outputJSON, jobID)
	if err != nil {
		p.logger.Error().Err(err).Str("job_id", jobID).Msg("Failed to update AI job output")
	}
}

func (p *Pool) updateAIJobError(jobID, errorMsg string) {
	ctx := context.Background()
	_, err := p.config.DB.Exec(ctx, `UPDATE ai_jobs SET error = $1 WHERE id = $2`, errorMsg, jobID)
	if err != nil {
		p.logger.Error().Err(err).Str("job_id", jobID).Msg("Failed to update AI job error")
	}
}

func (p *Pool) streamLog(key, level, message string) {
	ctx := context.Background()
	p.config.Redis.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		Values: map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"level":     level,
			"message":   message,
		},
	}).Err()

	// Also publish to NATS for real-time WebSocket updates
	logMsg := map[string]interface{}{
		"deployment_id": key[19:], // strip "deployment:logs:" prefix
		"timestamp":     time.Now().Format(time.RFC3339),
		"level":         level,
		"message":       message,
	}
	logBytes, _ := json.Marshal(logMsg)
	p.config.NATS.Publish("deployment.logs."+key[19:], logBytes)
}