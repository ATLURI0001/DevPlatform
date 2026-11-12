package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Provider   string
	APIKey     string
	Model      string
	BaseURL    string
	MaxTokens  int
	Temperature float32
}

type Client struct {
	config     Config
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.3
	}
	if cfg.BaseURL == "" {
		switch cfg.Provider {
		case "openai":
			cfg.BaseURL = "https://api.openai.com/v1"
		case "anthropic":
			cfg.BaseURL = "https://api.anthropic.com/v1"
		case "ollama":
			cfg.BaseURL = "http://localhost:11434/v1"
		default:
			cfg.BaseURL = "https://api.openai.com/v1"
		}
	} else {
		cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	}

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float32       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (c *Client) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	reqBody := ChatRequest{
		Model:       c.config.Model,
		Messages:    messages,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return "", fmt.Errorf("API error: %d - %v", resp.StatusCode, errResp)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

func (c *Client) ChatStream(ctx context.Context, messages []ChatMessage, onChunk func(string)) error {
	reqBody := ChatRequest{
		Model:       c.config.Model,
		Messages:    messages,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
		Stream:      true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("API error: %d - %v", resp.StatusCode, errResp)
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" || line == "data: [DONE]" {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
			if data == "[DONE]" {
				continue
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err == nil {
				if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" && onChunk != nil {
					onChunk(chunk.Choices[0].Delta.Content)
				}
			}
		}
	}

	return scanner.Err()
}

// ReviewPR performs a code review on a PR
func (c *Client) ReviewPR(ctx context.Context, pr *PRContext) (*ReviewResult, error) {
	systemPrompt := `You are an expert code reviewer. Review the provided code changes and provide:
1. Summary of changes
2. Potential bugs or issues
3. Security concerns
4. Performance considerations
5. Code style and best practices
6. Architecture/design feedback
7. Suggested improvements

Format your response as JSON with the following structure:
{
  "summary": "string",
  "issues": [{"severity": "critical|major|minor", "file": "string", "line": int, "message": "string", "suggestion": "string"}],
  "security": [{"severity": "critical|major|minor", "file": "string", "message": "string"}],
  "performance": [{"file": "string", "message": "string", "suggestion": "string"}],
  "style": [{"file": "string", "message": "string", "suggestion": "string"}],
  "architecture": [{"message": "string", "suggestion": "string"}],
  "overall_score": int  // 1-10
}`

	userPrompt := fmt.Sprintf(`Review this PR:
Repository: %s
PR #%d: %s -> %s
Files changed: %d
Diff:
%s`,
		pr.RepoURL, pr.PRNumber, pr.BaseBranch, pr.HeadBranch, len(pr.Files), pr.Diff)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	content, err := c.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}

	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// Try to extract JSON from markdown code blocks
		content = extractJSON(content)
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return nil, fmt.Errorf("parse review result: %w", err)
		}
	}

	return &result, nil
}

type PRContext struct {
	RepoURL     string
	PRNumber    int
	BaseBranch  string
	HeadBranch  string
	Files       []string
	Diff        string
}

type ReviewResult struct {
	Summary      string          `json:"summary"`
	Issues       []ReviewIssue   `json:"issues"`
	Security     []SecurityIssue `json:"security"`
	Performance  []PerfIssue     `json:"performance"`
	Style        []StyleIssue    `json:"style"`
	Architecture []ArchIssue     `json:"architecture"`
	OverallScore int             `json:"overall_score"`
}

type ReviewIssue struct {
	Severity   string `json:"severity"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type SecurityIssue struct {
	Severity string `json:"severity"`
	File     string `json:"file"`
	Message  string `json:"message"`
}

type PerfIssue struct {
	File       string `json:"file"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type StyleIssue struct {
	File       string `json:"file"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type ArchIssue struct {
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// GenerateDocs generates documentation for code
func (c *Client) GenerateDocs(ctx context.Context, req *DocsRequest) (*DocsResult, error) {
	systemPrompt := `You are a technical writer. Generate documentation for the provided code.
Format as JSON:
{
  "overview": "string",
  "api": [{"endpoint": "string", "method": "string", "description": "string", "params": [], "response": {}}],
  "architecture": "string",
  "readme": "string",
  "adrs": [{"title": "string", "context": "string", "decision": "string", "consequences": "string"}]
}`

	userPrompt := fmt.Sprintf(`Generate %s documentation for:
Repository: %s
Paths: %v
Format: %s`,
		req.DocTypes, req.RepoURL, req.Paths, req.Format)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	content, err := c.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}

	var result DocsResult
	content = extractJSON(content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse docs result: %w", err)
	}

	return &result, nil
}

type DocsRequest struct {
	RepoURL string
	Paths   []string
	DocTypes []string
	Format  string
}

type DocsResult struct {
	Overview    string          `json:"overview"`
	API         []APIEndpoint   `json:"api"`
	Architecture string         `json:"architecture"`
	Readme      string          `json:"readme"`
	ADRs        []ADR           `json:"adrs"`
}

type APIEndpoint struct {
	Endpoint    string            `json:"endpoint"`
	Method      string            `json:"method"`
	Description string            `json:"description"`
	Params      []Param           `json:"params"`
	Response    map[string]interface{} `json:"response"`
}

type Param struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type ADR struct {
	Title       string `json:"title"`
	Context     string `json:"context"`
	Decision    string `json:"decision"`
	Consequences string `json:"consequences"`
}

// GenerateTests generates tests for code
func (c *Client) GenerateTests(ctx context.Context, req *TestsRequest) (*TestsResult, error) {
	systemPrompt := `You are a test engineer. Generate comprehensive tests for the provided code.
Format as JSON:
{
  "test_files": [{"path": "string", "content": "string", "type": "unit|integration|e2e"}],
  "coverage_target": int,
  "setup_instructions": "string"
}`

	userPrompt := fmt.Sprintf(`Generate %s tests for:
Repository: %s
Paths: %v
Framework: %s
Target coverage: %d%%
`,
		req.TestTypes, req.RepoURL, req.Paths, req.Framework, req.Coverage)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	content, err := c.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}

	var result TestsResult
	content = extractJSON(content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse tests result: %w", err)
	}

	return &result, nil
}

type TestsRequest struct {
	RepoURL   string
	Paths     []string
	TestTypes []string
	Framework string
	Coverage  int
}

type TestsResult struct {
	TestFiles        []TestFile `json:"test_files"`
	CoverageTarget   int        `json:"coverage_target"`
	SetupInstructions string     `json:"setup_instructions"`
}

type TestFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

// MigrateCode migrates code from one language to another
func (c *Client) MigrateCode(ctx context.Context, req *MigrateRequest) (*MigrateResult, error) {
	systemPrompt := `You are a code migration expert. Migrate the provided code from %s to %s.
Preserve functionality, tests, and comments.
Format as JSON:
{
  "files": [{"path": "string", "content": "string"}],
  "notes": "string",
  "warnings": ["string"]
}`

	userPrompt := fmt.Sprintf(`Migrate code from %s to %s:
Repository: %s
Paths: %v
Preserve tests: %v`,
		req.SourceLang, req.TargetLang, req.RepoURL, req.Paths, req.PreserveTests)

	messages := []ChatMessage{
		{Role: "system", Content: fmt.Sprintf(systemPrompt, req.SourceLang, req.TargetLang)},
		{Role: "user", Content: userPrompt},
	}

	content, err := c.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}

	var result MigrateResult
	content = extractJSON(content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse migrate result: %w", err)
	}

	return &result, nil
}

type MigrateRequest struct {
	RepoURL       string
	SourceLang    string
	TargetLang    string
	Paths         []string
	PreserveTests bool
}

type MigrateResult struct {
	Files    []MigratedFile `json:"files"`
	Notes    string         `json:"notes"`
	Warnings []string       `json:"warnings"`
}

type MigratedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func extractJSON(content string) string {
	// Try to extract JSON from markdown code blocks
	start := -1
	for i := 0; i < len(content)-6; i++ {
		if content[i:i+7] == "```json" {
			start = i + 7
			break
		}
		if content[i:i+3] == "```" {
			start = i + 3
			break
		}
	}
	if start == -1 {
		return content
	}
	end := -1
	for i := start; i < len(content)-2; i++ {
		if content[i:i+3] == "```" {
			end = i
			break
		}
	}
	if end == -1 {
		return content[start:]
	}
	return content[start:end]
}