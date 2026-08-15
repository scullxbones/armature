# Contributing to Armature

Thank you for your interest in contributing to Armature! This document explains how to get started with development and the quality standards we maintain.

## Prerequisites

- Go 1.26 or later (matches go.mod)
- `make` for build and test commands
- `golangci-lint` for linting
- `gremlins` for mutation testing

## Getting Started

1. Clone the repository
2. Run `make install` to build and install the `arm` binary to `~/.local/bin/`
3. Run `arm bootstrap` to initialize the repository and deploy skills
4. Run `arm worker-init` once per clone to register your worker identity

## Development Workflow

Armature uses test-driven development (TDD). Follow this workflow for all contributions:

1. **Write a failing test** — Start with the requirement and write a test that verifies it
2. **Implement the minimum** — Write just enough code to make the test pass
3. **Refactor** — Clean up the implementation
4. **Run `make check`** — Verify your changes pass all quality gates before committing

## Quality Gates

All commits must pass `make check`, which enforces:

- **Lint checks** — `govet`, `errcheck`, `ineffassign`, `staticcheck`, `misspell`, `unconvert`, `goimports`
- **Tests** — All tests must pass
- **Coverage** — Per-tree statement coverage: cmd ≥83%, internal ≥86% (use `make coverage` to check)
- **Mutation testing** — Minimum 92% mcover, 99% efficacy with `gremlins`
- **Skill validation** — All embedded skills must be valid
- **Build** — The project must compile successfully

See [docs/agents/quality-gates.md](docs/agents/quality-gates.md) for detailed requirements and how to fix common issues.

### Running Quality Checks

```bash
make check              # Run all checks (lint + test + coverage + mutate + validate-skills + build)
make lint               # Run linters only
make test               # Run tests only
make coverage           # Generate coverage report (coverage.html)
make coverage-check     # Verify per-tree coverage: cmd ≥83%, internal ≥86%
make mutate             # Run mutation testing
make validate-skills    # Validate embedded skills
make build              # Build the arm binary
```

## Architecture Decisions

Significant changes to Armature require Architecture Decision Records (ADRs). ADRs are append-only documents in `docs/adr/`:

- Constitution amendments require a new ADR (see `docs/adr/0009-ratify-the-armature-constitution.md`)
- Wording-only fixes to ADRs do not require new ADRs
- Use `docs/adr/template.md` as a template for new ADRs
- Reference the ADR in your PR description

See [docs/adr/](docs/adr/) for the full list of existing decisions.

## Documentation

- [AGENTS.md](AGENTS.md) — Agent setup and first-time development commands
- [CONSTITUTION.md](CONSTITUTION.md) — Core invariants and principles
- [CONTEXT.md](CONTEXT.md) — Domain glossary
- [docs/conventions.md](docs/conventions.md) — Commit format, branch naming, test naming conventions
- [docs/commands.md](docs/commands.md) — CLI command reference
- [docs/design/architecture.md](docs/design/architecture.md) — System architecture

## Submitting Changes

1. Create a feature branch from `main`
2. Implement your changes following the workflow above
3. Ensure all commits pass `make check`
4. Submit a pull request with a clear description of what was changed and why
5. Address any review feedback

See [docs/conventions.md](docs/conventions.md) for commit message format requirements.

## Reporting Issues

If you encounter a problem:

1. **Bug report** — Use the [bug issue template](.github/ISSUE_TEMPLATE/bug.yml) to report issues
2. **Dogfood findings** — If you find friction while using Armature tools, report it via the [dogfood finding template](.github/ISSUE_TEMPLATE/dogfood-finding.yml)

## Security

For security vulnerabilities, please see [SECURITY.md](SECURITY.md) for our responsible disclosure policy.

## Questions?

If you have questions:

- Check the documentation in `docs/agents/` and `docs/design/`
- Review existing issues and pull requests
- Open a discussion issue with your question

Thank you for contributing!
