# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## TDD is mandatory

Write failing tests before writing implementation code. No exceptions.

1. Write a failing test that captures the requirement.
2. Write the minimum code to make it pass.
3. Refactor if needed.

Never commit implementation code without a corresponding test.

## `make check` must be green before every commit and push

```bash
make check   # lint + test + coverage-check (≥80%) + mutate + validate-skills + build
```

All stages must be green. Do not ignore or suppress failures — fix them.

Linters enabled: `govet`, `errcheck`, `ineffassign`, `staticcheck`, `misspell`, `unconvert`, `goimports`.

Do not add `//nolint` unless the suppression is genuinely justified. Do not lower the 80% coverage threshold or the mutation testing thresholds (90% mcover, 75% efficacy).

## Commands

```bash
make build           # build ./bin/arm
make install         # build and install to ~/.local/bin/arm
make test            # run all tests (summarized via scripts/summarize_test_json.py)
make lint            # golangci-lint run ./...
make coverage        # generate coverage.html
make coverage-check  # fail if total coverage < 80%
make mutate          # gremlins mutation testing on ./internal
make validate-skills # validate embedded skill source
make skill           # build + deploy skills to .claude/skills/, .gemini/skills/, .codex/skills/
make clean           # remove bin/, dist/, *.out, coverage.html, mutesting-report/, .claude/skills/

# Run a single test
go test ./internal/materialize/... -run TestApplyOp

# Run tests with verbose output
go test -v ./internal/materialize/...
```

Dependencies: `golangci-lint` and `gremlins` must be on PATH (`go install ...@latest`).

## Working Model

Armature is a git-native work orchestration system. It stores coordination state
as append-only ops and materialized task state under `.armature/`, with no
external database or server.

Current operational boundary:

- Armature coordinates work; it does **not** execute or supervise external
  harnesses.
- Do not introduce, restore, or document `arm orchestrate` or `arm worker run`
  as active commands.
- The primary workflow is `arm ready` -> `arm claim` -> `arm render-context` ->
  external worker execution -> `arm transition`.
- `arm harness-hook` is the retained integration surface for harness-native
  policy enforcement.

Important invariants:

- Ops are append-only JSONL entries in `.armature/ops/<worker-id>.log`.
- Each worker writes only to its own log file.
- Materialized state is derived from ops, not source of truth.
- `done` means worker-complete; `merged` means confirmed on the main branch.

## Canonical References

Prefer linking to the canonical docs instead of re-explaining them here:

- `docs/design/architecture.md` — architecture and repo model
- `docs/commands.md` — current CLI surface
- `docs/harness-hook.md` — harness-native integration
- `internal/skillsembed/skills/` — accompanying skill source code
- `CONTEXT.md` - domain glossary

## Skills

Bundled skills are deployed via `arm install-skills` or `make skill` to local
agent directories. The current bundled set is:

- `armature`
- `armature-coordinator`
- `armature-worker`
- `armature-planner`
- `armature-auditor`

`make validate-skills` enforces that skill bodies do not reference `make install`.
