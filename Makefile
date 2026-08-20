# DevPlatform Makefile
# Provides common development and deployment commands

.PHONY: help dev build test lint fmt vet docker-up docker-down docker-logs docker-ps migrate-up migrate-down migrate-create adr-new

# Default target
help:
	@echo "DevPlatform - AI-Native Developer Platform"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Development:"
	@echo "  dev              Start all services with docker-compose (detached)"
	@echo "  dev-logs         Follow docker-compose logs"
	@echo "  dev-stop         Stop all services"
	@echo "  dev-restart      Restart all services"
	@echo "  dev-ps           Show running containers"
	@echo ""
	@echo "Building:"
	@echo "  build            Build all Go binaries"
	@echo "  build-frontend   Build frontend for production"
	@echo "  build-docker     Build Docker images"
	@echo ""
	@echo "Testing:"
	@echo "  test             Run all tests"
	@echo "  test-unit        Run unit tests only"
	@echo "  test-integration Run integration tests"
	@echo "  test-e2e         Run end-to-end tests"
	@echo "  test-coverage    Run tests with coverage report"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint             Run all linters"
	@echo "  lint-go          Run Go linters (golangci-lint)"
	@echo "  lint-frontend    Run frontend linters (eslint, prettier)"
	@echo "  fmt              Format all code"
	@echo "  fmt-go           Format Go code (gofmt, goimports)"
	@echo "  fmt-frontend     Format frontend code (prettier)"
	@echo "  vet              Run go vet"
	@echo ""
	@echo "Database:"
	@echo "  migrate-up       Run database migrations up"
	@echo "  migrate-down     Run database migrations down"
	@echo "  migrate-create   Create new migration (usage: make migrate-create NAME=add_users_table)"
	@echo "  migrate-status   Show migration status"
	@echo "  db-shell         Open psql shell"
	@echo "  db-reset         Reset database (drop, create, migrate)"
	@echo ""
	@echo "Documentation:"
	@echo "  adr-new          Create new ADR (usage: make adr-new TITLE=\"Use Redis for caching\")"
	@echo "  docs-serve       Serve documentation locally"
	@echo ""
	@echo "Deployment:"
	@echo "  deploy-staging   Deploy to staging (k8s)"
	@echo "  deploy-prod      Deploy to production (k8s)"
	@echo "  k8s-diff         Show kustomize diff for staging"
	@echo ""
	@echo "Utilities:"
	@echo "  clean            Clean build artifacts"
	@echo "  deps             Download dependencies"
	@echo "  generate         Run code generation"
	@echo "  version          Show version info"

# Development
dev: docker-up
	@echo "Starting development environment..."
	@echo "Frontend: http://localhost:3000"
	@echo "API: http://localhost:8080"
	@echo "Grafana: http://localhost:3001 (admin/admin)"
	@echo "Prometheus: http://localhost:9090"

dev-logs:
	docker-compose logs -f

dev-stop:
	docker-compose down

dev-restart: dev-stop dev

dev-ps:
	docker-compose ps

# Building
build: build-go build-frontend

build-go:
	@echo "Building Go binaries..."
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/control-plane ./cmd/control-plane
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/worker ./cmd/worker

build-frontend:
	@echo "Building frontend..."
	cd frontend && npm run build

build-docker:
	@echo "Building Docker images..."
	docker-compose build

# Testing
test: test-unit test-integration

test-unit:
	@echo "Running unit tests..."
	go test -v -race -count=1 ./...
	cd frontend && npm run test -- --run

test-integration:
	@echo "Running integration tests..."
	go test -v -tags=integration -count=1 ./internal/...

test-e2e:
	@echo "Running e2e tests..."
	cd frontend && npm run test:e2e

test-coverage:
	@echo "Running tests with coverage..."
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Code Quality
lint: lint-go lint-frontend

lint-go:
	@echo "Running Go linters..."
	golangci-lint run ./...

lint-frontend:
	@echo "Running frontend linters..."
	cd frontend && npm run lint

fmt: fmt-go fmt-frontend

fmt-go:
	@echo "Formatting Go code..."
	gofmt -w .
	goimports -w .

fmt-frontend:
	@echo "Formatting frontend code..."
	cd frontend && npm run format

vet:
	@echo "Running go vet..."
	go vet ./...

# Database
migrate-up:
	@echo "Running migrations up..."
	go run ./cmd/migrate up

migrate-down:
	@echo "Running migrations down..."
	go run ./cmd/migrate down

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=migration_name"; exit 1; fi
	@echo "Creating migration: $(NAME)"
	go run ./cmd/migrate create $(NAME)

migrate-status:
	@echo "Migration status:"
	go run ./cmd/migrate status

db-shell:
	docker-compose exec postgres psql -U devplatform -d devplatform

db-reset:
	@echo "Resetting database..."
	docker-compose exec postgres psql -U devplatform -d devplatform -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	$(MAKE) migrate-up

# Documentation
adr-new:
	@if [ -z "$(TITLE)" ]; then echo "Usage: make adr-new TITLE=\"Your ADR Title\""; exit 1; fi
	@NUM=$$(ls docs/adr/ | wc -l | xargs printf "%04d"); \
	DATE=$$(date +%Y-%m-%d); \
	FILE=docs/adr/$$NUM-$$(echo $(TITLE) | tr '[:upper:]' '[:lower:]' | tr ' ' '-').md; \
	cp docs/adr/0000-template.md $$FILE; \
	sed -i "s/Title: .*/Title: $(TITLE)/" $$FILE; \
	sed -i "s/Date: .*/Date: $$DATE/" $$FILE; \
	sed -i "s/Status: .*/Status: Proposed/" $$FILE; \
	echo "Created $$FILE"

docs-serve:
	@echo "Serving docs at http://localhost:8081"
	docker run --rm -p 8081:8080 -v $(PWD)/docs:/docs squidfunk/mkdocs-material

# Deployment
deploy-staging:
	@echo "Deploying to staging..."
	kubectl apply -k deploy/k8s/overlays/staging
	kubectl rollout status deployment/control-plane -n devplatform-staging
	kubectl rollout status deployment/worker -n devplatform-staging
	kubectl rollout status deployment/frontend -n devplatform-staging

deploy-prod:
	@echo "Deploying to production..."
	kubectl apply -k deploy/k8s/overlays/production
	kubectl rollout status deployment/control-plane -n devplatform
	kubectl rollout status deployment/worker -n devplatform
	kubectl rollout status deployment/frontend -n devplatform

k8s-diff:
	@echo "Showing kustomize diff for staging..."
	kubectl diff -k deploy/k8s/overlays/staging || true

# Utilities
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf frontend/.next/
	rm -rf frontend/node_modules/
	rm -f coverage.out coverage.html
	go clean -cache -modcache -testcache

deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod verify
	cd frontend && npm ci

generate:
	@echo "Running code generation..."
	go generate ./...

version:
	@echo "DevPlatform Version Info"
	@echo "Git Commit: $$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
	@echo "Git Branch: $$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'unknown')"
	@echo "Build Date: $$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	@echo "Go Version: $$(go version)"

# Install development tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	cd frontend && npm ci