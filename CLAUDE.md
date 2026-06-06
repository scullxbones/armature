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

## Architecture

Armature is a git-native work orchestration system. The single binary (`arm`) coordinates human and AI workers through append-only ops logs stored on a dedicated `_armature` orphan git branch, with no external database or server.

### Dual-branch model

- **`main`** — code, feature branches, PRs (protected)
- **`_armature`** — `.armature/` coordination data, direct push by all workers (unprotected orphan)

Workers operate in a secondary git worktree (`.arm/`) pointed at `_armature` with sparse checkout limited to `.armature/ops/` and `.armature/state/`. All op writes go to the ops worktree; code writes go to the main worktree. These are never mixed within a single phase.

Single-branch fallback: if `main` is directly pushable, all state lives on `main`.

### Op log and materialization

All state changes are **append-only JSONL ops** in `.armature/ops/<worker-id>.log`. Each worker writes exclusively to its own log file — this is the MRDT invariant that makes merge conflicts architecturally impossible.

`state/` files (`index.json`, `ready.json`, `issues/<uuid>.json`, etc.) are **materialized locally by each worker** and never committed to the ops branch. They are derived caches, not source of truth.

The incremental materializer (`internal/materialize`) replays only new ops since `checkpoint.json`, making each invocation O(new ops) rather than O(all ops).

### Key internal packages

| Package | Responsibility |
|---|---|
| `internal/ops` | Op log parsing, appending, push/retry loop, rate limiting |
| `internal/materialize` | Incremental materialization: ops → state files |
| `internal/claim` | Claim race resolution (timestamp + worker-ID tiebreak) |
| `internal/ready` | Ready-task computation and queue |
| `internal/context` | 7-layer context assembly algorithm (`arm render-context`) |
| `internal/dag` | DAG structure, cycle detection, bottom-up rollup |
| `internal/decompose` | `decompose-context` / `decompose-apply` workflow |
| `internal/sources` | Source document registration, MCP fetch, cache management |
| `internal/validate` | Semantic validation (E1–E12 errors, W1–W11 warnings) |
| `internal/doctor` | Structural pre-work gate (D1–D6 checks) |
| `internal/adapters` | Git and shell adapters; all `os/exec` calls live here |
| `internal/tui` | Bubble Tea TUI components (dag-summary, stale-review, ready) |
| `internal/skillsembed` | Embedded skill files (SKILL.md per role) deployed via `arm install-skills` |
| `cmd/armature` | Cobra command wiring; one file per subcommand |

### Context assembly (`internal/context`)

`arm render-context` assembles a 7-layer context slice for a task within a token budget (default 1600, proxy: `chars/4`). Fixed layers (core spec, snippets) are never truncated. Truncatable layers (blocker outcomes, parent chain, decisions, notes, sibling outcomes) are dropped lowest-priority-first when over budget.

### Two-phase task completion

`done` = worker believes work is complete (self-reported).  
`merged` = code confirmed on main (auto-detected during materialization via commit-message scan, branch-name check, scope-file heuristic, or `arm merged` manual command).  
Downstream tasks require `merged`, not `done`, to unblock.

### Skills

Bundled skills (`internal/skillsembed/skills/`) are deployed to `.claude/skills/`, `.gemini/skills/`, and `.codex/skills/` via `make skill`. Skills cover four roles: `armature` (quick reference), `armature-coordinator`, `armature-worker`, `armature-planner`, `armature-auditor`. The `validate-skills` make target enforces that skill bodies do not reference `make install`.
