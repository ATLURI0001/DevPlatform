# Contributing to DevPlatform

Thank you for your interest in contributing! This document provides guidelines for contributing to the project.

## 📋 Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Documentation](#documentation)
- [Pull Request Process](#pull-request-process)
- [Release Process](#release-process)

## 🤝 Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By participating, you agree to uphold this code.

## 🚀 Getting Started

1. **Fork** the repository
2. **Clone** your fork: `git clone https://github.com/YOUR_USERNAME/devplatform.git`
3. **Add upstream**: `git remote add upstream https://github.com/atlur/devplatform.git`
4. **Install dependencies**: `make deps`
5. **Start development environment**: `make dev`
6. **Run tests**: `make test`

## 🔄 Development Workflow

### Branch Naming

- Features: `feature/short-description`
- Bug fixes: `fix/short-description`
- Docs: `docs/short-description`
- Refactoring: `refactor/short-description`
- Chores: `chore/short-description`

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`, `build`, `ci`

Examples:
```
feat(ai): add Ollama provider support
fix(auth): handle token refresh race condition
docs(readme): update quick start guide
```

### Keeping Your Fork Updated

```bash
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
```

## 💻 Coding Standards

### Go

- Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Run `make fmt` before committing (gofmt, goimports)
- Run `make lint` before committing (golangci-lint)
- Run `make vet` before committing
- Use meaningful variable names, avoid single-letter names except in tight loops
- Write table-driven tests
- Document public APIs with godoc comments

```go
// CreateProject creates a new project with the given configuration.
// Returns the created project or an error if validation fails.
func (p *Platform) CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error) {
    // implementation
}
```

### TypeScript/React

- Use TypeScript strict mode
- Follow [React TypeScript Cheatsheet](https://react-typescript-cheatsheet.netlify.app/)
- Use functional components with hooks
- Prefer `interface` over `type` for object shapes
- Use Zod for runtime validation
- Run `make fmt-frontend` (Prettier) and `make lint-frontend` (ESLint) before committing

```tsx
interface ProjectCardProps {
  project: Project;
  onClick: () => void;
  onDelete: () => void;
}

export function ProjectCard({ project, onClick, onDelete }: ProjectCardProps) {
  // implementation
}
```

### General

- **Line length**: 100 characters (Go), 120 characters (TypeScript)
- **Indentation**: Tabs (Go), 2 spaces (TypeScript)
- **No trailing whitespace**
- **No commented-out code** - use version control instead

## 🧪 Testing

### Go Tests

```bash
# Unit tests
make test-unit

# Integration tests
make test-integration

# With coverage
make test-coverage
```

### Frontend Tests

```bash
# Unit tests
cd frontend && npm run test

# E2E tests
cd frontend && npm run test:e2e
```

### Test Guidelines

- **Unit tests**: Test individual functions/methods in isolation
- **Integration tests**: Test component interactions (database, NATS, Redis)
- **Contract tests**: Test API contracts
- **E2E tests**: Test critical user flows
- **Target**: 80%+ code coverage
- **Naming**: `TestFunctionName_Scenario_ExpectedResult` (Go), `describe/it` (JS)

## 📚 Documentation

### API Documentation

- Update OpenAPI spec in `docs/api/openapi.yaml` for API changes
- Run `make docs-serve` to preview

### Architecture Decision Records (ADRs)

Create an ADR for significant architectural decisions:

```bash
make adr-new TITLE="Use Redis for distributed locking"
```

ADR template in `docs/adr/0000-template.md`

### Code Comments

- Public APIs: Required
- Complex logic: Explain *why*, not *what*
- TODO comments: Include issue reference: `// TODO(#123): Optimize query`

## 🔀 Pull Request Process

1. **Create PR** from your feature branch to `main`
2. **Fill out PR template** completely
3. **Ensure CI passes**:
   - All linters
   - All tests (unit, integration)
   - Security scans
   - Build succeeds
4. **Request review** from maintainers
5. **Address feedback** - push new commits (don't force push)
6. **Squash and merge** after approval

### PR Requirements

- [ ] Descriptive title and description
- [ ] Related issue linked (`Fixes #123`)
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] No merge conflicts
- [ ] CI passes
- [ ] At least 1 approval

### Review Checklist

- [ ] Code follows style guides
- [ ] Tests are meaningful and pass
- [ ] No security vulnerabilities introduced
- [ ] Performance impact considered
- [ ] Backward compatibility maintained
- [ ] Logging and observability added
- [ ] Error handling appropriate

## 🚢 Release Process

Releases are automated via GitHub Actions on version tags (`v*`).

### Creating a Release

1. Ensure `main` is up to date
2. Create and push a version tag:
   ```bash
   git tag -a v1.2.3 -m "Release v1.2.3"
   git push origin v1.2.3
   ```
3. GitHub Actions will:
   - Run full CI pipeline
   - Build and push multi-arch Docker images
   - Generate SBOMs
   - Sign images with cosign
   - Deploy to staging (on `develop` branch)
   - Deploy to production (on version tags)
   - Create GitHub Release with changelog

### Versioning

Follow [Semantic Versioning](https://semver.org/):
- `MAJOR`: Breaking changes
- `MINOR`: New features (backward compatible)
- `PATCH`: Bug fixes (backward compatible)

### Changelog

Maintained in `CHANGELOG.md` following [Keep a Changelog](https://keepachangelog.com/).

## 🏷️ Labels

Common labels used in issues and PRs:

| Label | Description |
|-------|-------------|
| `bug` | Something isn't working |
| `enhancement` | New feature or request |
| `documentation` | Documentation improvements |
| `good first issue` | Good for newcomers |
| `help wanted` | Extra attention needed |
| `priority: high` | High priority |
| `priority: low` | Low priority |
| `status: blocked` | Blocked by external dependency |
| `status: in progress` | Currently being worked on |
| `status: needs review` | Ready for review |

## 💬 Getting Help

- **Discussions**: [GitHub Discussions](https://github.com/atlur/devplatform/discussions)
- **Issues**: [GitHub Issues](https://github.com/atlur/devplatform/issues)
- **Security**: See [SECURITY.md](SECURITY.md)

## 🙏 Recognition

Contributors are recognized in:
- GitHub Contributors graph
- Release notes
- Annual contributor appreciation

---

*Thank you for contributing to DevPlatform!* 🚀