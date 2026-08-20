# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| devplatform >= 1.0.0 | ✅ Yes |
| < 1.0.0 | ❌ No |

## Reporting a Vulnerability

**Please do NOT open a public issue for security vulnerabilities.**

Instead, report them via:

1. **GitHub Security Advisories** (preferred): Go to the Security tab → "Report a vulnerability"
2. **Email**: atlur@example.com with subject "SECURITY: devplatform"

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We aim to:
- Acknowledge within 48 hours
- Provide initial assessment within 7 days
- Release fix within 30 days (depending on severity)

## Security Practices

### Dependency Management

- All dependencies scanned via Dependabot (weekly)
- CodeQL analysis on every push
- Trivy container scanning on every build
- SBOM generated for each release (SPDX + CycloneDX)

### Supply Chain

- Artifacts signed with cosign (keyless via GitHub OIDC)
- SLSA Level 2+ build provenance
- Reproducible builds enforced
- Dependency pinning with lockfiles

### Runtime

- Non-root containers
- Read-only root filesystem
- Dropped capabilities
- Seccomp/AppArmor profiles
- Network policies in Kubernetes

### Secrets

- No secrets in code or config
- Use secret managers (Vault, AWS Secrets Manager, etc.)
- `.env.example` for template only
- Git-secrets pre-commit hook recommended

## Disclosure Timeline

| Severity | Target Fix |
| -------- | ---------- |
| Critical | 7 days |
| High | 14 days |
| Medium | 30 days |
| Low | 90 days |

## Hall of Fame

Thanks to researchers who responsibly disclosed:

<!-- Add names here after first disclosure -->

---

**PGP Key** (for encrypted reports):
```
Atlur <atlur@example.com>
```