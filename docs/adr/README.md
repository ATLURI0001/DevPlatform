# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for DevPlatform.

## What is an ADR?

An ADR is a document that captures a decision made in the project, including:
- The context and forces that led to the decision
- The decision itself
- The consequences (positive, negative, neutral)

## ADR List

| # | Title | Status | Date |
|---|-------|--------|------|
| 0000 | [Template](0000-template.md) | Draft | - |
| 0001 | [Architecture Overview](0001-architecture-overview.md) | Proposed | - |
| 0002 | [Technology Stack](0002-technology-stack.md) | Proposed | - |
| 0003 | [Data Model](0003-data-model.md) | Proposed | - |
| 0004 | [API Design](0004-api-design.md) | Proposed | - |
| 0005 | [Deployment Strategy](0005-deployment-strategy.md) | Proposed | - |

## Creating a New ADR

```bash
make adr title="Your Decision Title"
```

This generates a new numbered ADR following the MADR format.

## Format

We follow [MADR](https://adr.github.io/madr/) (Markdown Any Decision Records).

### Status Options

- **Proposed**: Under consideration
- **Accepted**: Decision made and being implemented
- **Deprecated**: No longer recommended
- **Superseded**: Replaced by another ADR (link it)

## Contributing ADRs

1. Create new ADR via `make adr`
2. Fill in context, decision, consequences
3. Submit PR with ADR
4. Review and acceptance by maintainers
5. Status → **Accepted** when merged

---

**Note**: ADRs are immutable once **Accepted**. To change a decision, create a new ADR that supersedes it.