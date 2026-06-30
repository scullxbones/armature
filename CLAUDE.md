# CLAUDE.md

## TDD is mandatory

Write failing test → minimum implementation → refactor. No exceptions. No implementation without a test.

## `make check` must pass before every commit/push

```bash
make check   # lint + test + coverage (≥85%) + mutate + validate-skills + build
```

Fix failures; never suppress. Linters: `govet`, `errcheck`, `ineffassign`, `staticcheck`, `misspell`, `unconvert`, `goimports`. No `//nolint` without justification. Don't lower coverage (85%) or mutation thresholds (95% mcover, 99% efficacy).

## Commands

```bash
make build           # build ./bin/arm
make install         # build → ~/.local/bin/arm
make test            # run all tests
make lint            # golangci-lint run ./...
make coverage        # generate coverage.html
make coverage-check  # fail if coverage < 85%
make mutate          # gremlins mutation testing on ./internal
make validate-skills # validate embedded skill source
make skill           # deploy skills to .claude/skills/, .gemini/skills/, .codex/skills/
make clean           # remove bin/, dist/, *.out, coverage.html, mutesting-report/, .claude/skills/

go test ./internal/materialize/... -run TestApplyOp   # single test
go test -v ./internal/materialize/...                 # verbose
```

Dependencies: `golangci-lint` and `gremlins` must be on PATH.

## Working Model

Armature is a git-native work orchestration system. State stored as append-only ops + materialized task state under `.armature/`. No external DB or server.

- Armature supports task-driven workflows; it does **not** execute or supervise harnesses.
- Primary workflow: `arm ready` → `arm claim` → `arm render-context` → external worker → `arm transition`.
- `arm harness-hook` is the retained harness integration surface.

Invariants:
- Ops are append-only JSONL in `.armature/ops/<worker-id>.log`; each worker writes only its own log.
- Materialized state is derived from ops, not source of truth.
- `done` = worker-complete; `merged` = confirmed on main branch.

## Dogfood Findings

Armature is used to build Armature — every task is a live dogfood run. Capture friction **immediately** before continuing:

```
Skill("capturing-dogfood-findings")
```

Invoke when:
- an `arm` command fails, returns confusing output, or behaves differently than the docs describe
- a skill does not fire when expected, or fires with wrong content
- a workflow step requires knowledge not in CLAUDE.md, AGENTS.md, or the relevant skill
- `make check`, `arm validate`, or `arm doctor` behaves unexpectedly
- a doc or error message is misleading or missing

- **Findings root:** `docs/dogfood/findings/`
- **Writer identity:** `arm worker-init --check` (UUID) + `ARM_LOG_SLOT` if set
- **Area:** `bootstrap` | `hooks` | `skills` | `commands` | `workflow` | `validation` | `coordination` | `tooling` | `documentation` | `other`

Capture, then return to task. Don't turn findings into implementation work unless asked.

## Canonical References

- `docs/design/architecture.md` — architecture and repo model
- `docs/commands.md` — CLI surface
- `docs/harness-hook.md` — harness integration
- `internal/skillsembed/skills/` — skill source
- `CONTEXT.md` — domain glossary

## Skills

Deployed via `arm bootstrap` or `make skill`. Bundled set: `armature`, `armature-coordinator`, `armature-worker`, `armature-planner`, `armature-auditor`.

`make validate-skills` enforces skill bodies don't reference `make install`.
