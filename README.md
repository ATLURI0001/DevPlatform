# DevPlatform

[![Build Status](https://github.com/atlur/devplatform/workflows/CI/CD%20Pipeline/badge.svg)](https://github.com/atlur/devplatform/actions)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://golang.org)
[![Next.js](https://img.shields.io/badge/Next.js-14+-000000?logo=next.js)](https://nextjs.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**AI-Native Developer Platform** - A self-hosted developer platform with AI-assisted workflows for code review, documentation generation, test creation, and code migration. Built with GitOps principles for ephemeral preview environments.

## 🎯 Features

- **GitOps Preview Environments**: Create ephemeral environments on every PR like Vercel/Netlify but self-hosted
- **AI-Assisted Workflows**:
  - Automated PR code review with configurable focus areas
  - Documentation generation (README, API docs, architecture)
  - Test generation with target coverage goals
  - Code migration between languages (JS→TS, Python→Go, etc.)
- **Multi-Project Management**: Organize projects, environments, and deployments
- **Real-time Updates**: WebSocket-based live deployment logs and status
- **Full Observability**: OpenTelemetry + Prometheus + Grafana + Tempo + Loki
- **Kubernetes Native**: Deploy with kustomize, GitOps via ArgoCD

## 🏗 Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Frontend  │────▶│ Control     │────▶│  PostgreSQL │
│  (Next.js)  │     │  Plane      │     │  (Primary)  │
└─────────────┘     │  (Go/Gin)   │     └─────────────┘
                    └──────┬──────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
    ┌───────────┐   ┌───────────┐   ┌───────────┐
    │   NATS    │   │   Redis   │   │  Workers  │
    │ (Queue)   │   │ (Cache/   │   │ (Async    │
    │           │   │  Logs)    │   │  Jobs)    │
    └───────────┘   └───────────┘   └───────────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
    ┌───────────┐   ┌───────────┐   ┌───────────┐
    │ Prometheus│   │   Tempo   │   │   Loki    │
    │ (Metrics) │   │ (Traces)  │   │ (Logs)    │
    └───────────┘   └───────────┘   └───────────┘
                           │
                           ▼
                    ┌───────────┐
                    │  Grafana  │
                    │(Dashboards)│
                    └───────────┘
```

## 🚀 Quick Start

### Prerequisites

- Docker & Docker Compose
- Go 1.22+ (for local development)
- Node.js 20+ (for frontend development)
- kubectl & kustomize (for Kubernetes deployment)

### Local Development

```bash
# Clone and enter
git clone https://github.com/atlur/devplatform.git
cd devplatform

# Start all services (PostgreSQL, NATS, Redis, Control Plane, Worker, Frontend, Observability)
make dev

# Or manually with docker-compose
docker-compose up -d

# Access points:
# Frontend:     http://localhost:3000
# API:          http://localhost:8080
# Grafana:      http://localhost:3001 (admin/admin)
# Prometheus:   http://localhost:9090
# NATS Monitor: http://localhost:8222
```

### Configuration

Copy `.env.example` to `.env` and configure:

```env
# Server
SERVER_ADDR=:8080

# Database
DATABASE_URL=postgres://devplatform:devplatform@localhost:5432/devplatform?sslmode=disable

# Message Queue
NATS_URL=nats://localhost:4222

# Cache & Logs
REDIS_ADDR=localhost:6379

# Authentication
JWT_SECRET=your-secure-random-secret

# AI Provider (openai, anthropic, ollama, mock)
AI_PROVIDER=mock
AI_API_KEY=your-api-key

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

## 📁 Project Structure

```
devplatform/
├── cmd/
│   ├── control-plane/     # API server entry point
│   └── worker/            # Async job worker entry point
├── internal/
│   ├── ai/                # AI client (OpenAI, Anthropic, Ollama)
│   ├── auth/              # JWT authentication
│   ├── gitops/            # GitOps manager (kustomize, ArgoCD)
│   ├── platform/          # Core platform logic
│   └── worker/            # Job processor
├── pkg/
│   ├── models/            # Domain models
│   └── telemetry/         # OpenTelemetry + Prometheus
├── frontend/              # Next.js 14 + React + TypeScript
│   ├── src/
│   │   ├── app/           # App Router pages
│   │   ├── components/    # React components
│   │   ├── hooks/         # Custom hooks
│   │   ├── services/      # API client
│   │   └── lib/           # Utilities
├── deploy/
│   ├── docker/            # Dockerfiles
│   ├── k8s/               # Kubernetes manifests (kustomize)
│   │   ├── base/          # Base configuration
│   │   └── overlays/      # Environment overlays
│   └── observability/     # Prometheus, Tempo, Loki, Grafana configs
├── docs/
│   ├── adr/               # Architecture Decision Records
│   ├── api/               # OpenAPI specifications
│   └── runbooks/          # Operational runbooks
├── .github/
│   ├── workflows/         # CI/CD pipelines
│   └── dependabot.yml     # Dependency updates
├── docker-compose.yml     # Local development stack
├── Makefile               # Common commands
├── go.mod / go.sum        # Go dependencies
└── README.md
```

## 🛠 Development

### Common Commands

```bash
# Development
make dev              # Start all services
make dev-logs         # Follow logs
make dev-stop         # Stop services

# Building
make build            # Build all binaries
make build-docker     # Build Docker images

# Testing
make test             # Run all tests
make test-unit        # Unit tests only
make test-integration # Integration tests
make test-coverage    # Coverage report

# Code Quality
make lint             # All linters
make fmt              # Format all code
make vet              # Go vet

# Database
make migrate-up       # Run migrations
make migrate-create NAME=add_table  # Create migration

# Documentation
make adr-new TITLE="Use Redis"      # Create ADR
make docs-serve       # Serve docs locally
```

### API Development

The control plane exposes a REST API with OpenAPI documentation:

```bash
# Health checks
curl http://localhost:8080/health
curl http://localhost:8080/ready

# Authentication
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}'

# Projects
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/projects
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/projects \
  -d '{"name":"my-project","description":"My project"}'
```

### Adding AI Providers

Implement the `AIClient` interface in `internal/ai/client.go`:

```go
type AIClient interface {
    Chat(ctx context.Context, messages []Message) (string, error)
    ChatStream(ctx context.Context, messages []Message) (<-chan string, error)
    ReviewPR(ctx context.Context, req ReviewRequest) (*ReviewResult, error)
    GenerateDocs(ctx context.Context, req DocsRequest) (*DocsResult, error)
    GenerateTests(ctx context.Context, req TestsRequest) (*TestsResult, error)
    MigrateCode(ctx context.Context, req MigrateRequest) (*MigrateResult, error)
}
```

## 📦 Deployment

### Kubernetes (Production)

```bash
# Staging
kubectl apply -k deploy/k8s/overlays/staging

# Production
kubectl apply -k deploy/k8s/overlays/production

# Or with ArgoCD (recommended)
kubectl apply -f deploy/argocd/
```

### Docker Compose (Simple)

```bash
# Production-like with docker-compose
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

## 🔐 Security

- **Authentication**: JWT HS256 with role-based access control
- **Authorization**: Project-scoped permissions
- **Secrets**: Kubernetes secrets / Docker secrets
- **Network**: mTLS via service mesh (optional)
- **Scanning**: Trivy, gosec, CodeQL in CI
- **SBOM**: Syft-generated, cosign-signed
- **Dependency Updates**: Dependabot weekly

See [SECURITY.md](SECURITY.md) for vulnerability reporting.

## 📊 Observability

The platform includes a complete observability stack:

- **Metrics**: Prometheus + Grafana dashboards
- **Traces**: Tempo (OpenTelemetry)
- **Logs**: Loki + Grafana
- **Alerting**: Alertmanager (configured separately)

Key dashboards:
- DevPlatform Overview
- Deployment Pipeline
- AI Job Performance
- System Health

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## 📄 License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).

## 🙏 Acknowledgments

- [Gin](https://github.com/gin-gonic/gin) - HTTP framework
- [NATS](https://nats.io) - Message queue
- [OpenTelemetry](https://opentelemetry.io) - Observability
- [Next.js](https://nextjs.org) - React framework
- [Tailwind CSS](https://tailwindcss.com) - Styling
- [Zustand](https://zustand-demo.pmnd.rs) - State management
- [TanStack Query](https://tanstack.com/query) - Server state