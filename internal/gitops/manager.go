package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type Config struct {
	GitURL      string
	GitToken    string
	BasePath    string
	ArgoCDAddr  string
	ArgoCDToken string
}

type Manager struct {
	config Config
}

func NewManager(cfg Config) (*Manager, error) {
	if cfg.BasePath == "" {
		cfg.BasePath = "/tmp/gitops"
	}
	os.MkdirAll(cfg.BasePath, 0755)

	return &Manager{config: cfg}, nil
}

func (m *Manager) CloneRepo(ctx context.Context, repoURL, branch, destDir string) error {
	auth := &http.BasicAuth{
		Username: "token",
		Password: m.config.GitToken,
	}

	_, err := git.PlainCloneContext(ctx, destDir, false, &git.CloneOptions{
		URL:           repoURL,
		ReferenceName: plumbing.ReferenceName(branch),
		Auth:          auth,
		SingleBranch:  true,
		Depth:         1,
	})
	return err
}

func (m *Manager) CreateEnvironmentManifests(projectID, envName, namespace, cluster string, config map[string]interface{}) error {
	envPath := filepath.Join(m.config.BasePath, projectID, envName)
	os.MkdirAll(envPath, 0755)

	// Create kustomization.yaml
	kustomization := fmt.Sprintf(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: %s

resources:
  - deployment.yaml
  - service.yaml
  - ingress.yaml

commonLabels:
  project: %s
  environment: %s

configMapGenerator:
  - name: app-config
    literals:
      - ENVIRONMENT=%s
      - CLUSTER=%s
`,
		namespace, projectID, envName, envName, cluster)

	if err := os.WriteFile(filepath.Join(envPath, "kustomization.yaml"), []byte(kustomization), 0644); err != nil {
		return err
	}

	// Create deployment.yaml
	deployment := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: app
  template:
    metadata:
      labels:
        app: app
    spec:
      containers:
        - name: app
          image: PLACEHOLDER_IMAGE
          ports:
            - containerPort: 8080
          envFrom:
            - configMapRef:
                name: app-config
          resources:
            requests:
              cpu: "100m"
              memory: "128Mi"
            limits:
              cpu: "500m"
              memory: "512Mi"
`)
	if err := os.WriteFile(filepath.Join(envPath, "deployment.yaml"), []byte(deployment), 0644); err != nil {
		return err
	}

	// Create service.yaml
	service := `apiVersion: v1
kind: Service
metadata:
  name: app
spec:
  selector:
    app: app
  ports:
    - port: 80
      targetPort: 8080
  type: ClusterIP
`
	if err := os.WriteFile(filepath.Join(envPath, "service.yaml"), []byte(service), 0644); err != nil {
		return err
	}

	// Create ingress.yaml
	ingress := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
    - host: %s.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: app
                port:
                  number: 80
`, envName)
	if err := os.WriteFile(filepath.Join(envPath, "ingress.yaml"), []byte(ingress), 0644); err != nil {
		return err
	}

	return nil
}

func (m *Manager) UpdateDeploymentImage(projectID, envName, image string) error {
	deploymentPath := filepath.Join(m.config.BasePath, projectID, envName, "deployment.yaml")
	// In practice, use a proper YAML parser
	// This is simplified
	return nil
}

func (m *Manager) CommitAndPush(ctx context.Context, projectID, envName, message string) error {
	repoPath := filepath.Join(m.config.BasePath, projectID)
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Add all files
	_, err = wt.Add(".")
	if err != nil {
		return err
	}

	// Commit
	_, err = wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "DevPlatform Bot",
			Email: "bot@devplatform.io",
			When:  time.Now(),
		},
	})
	if err != nil {
		return err
	}

	// Push
	auth := &http.BasicAuth{
		Username: "token",
		Password: m.config.GitToken,
	}
	return repo.PushContext(ctx, &git.PushOptions{Auth: auth})
}

func (m *Manager) SyncArgoCD(projectID, envName string) error {
	// Call ArgoCD API to sync application
	// Implementation depends on ArgoCD API client
	return nil
}