# Quality Gates

## TDD is mandatory

Write failing test → minimum implementation → refactor. No exceptions. No implementation without a test.

## Two-tier gates

Armature uses a two-tier gate model (`docs/design/gate-efficiency.md` D1):

- **Fast gate** (`make check-fast`): deterministic, diff-routed; use this
  during implementation and remediation. Do **not** run the full gate on
  intermediate iterations — it's expensive and most of its checks are
  irrelevant to a small remediation.
- **Publish gate** (`make check`): unchanged in scope and rigor; mandatory
  exactly twice per task lifecycle — once at the final task head, once
  cumulatively at story integration. A green fast gate is sufficient to
  iterate; only a green publish gate confers delivery (Constitution I5).

## `make check` must pass before every commit/push

```bash
make check   # serial full publish: union of check-static + check-core + mutate
```

Fix failures; never suppress. Linters: `govet`, `errcheck`, `ineffassign`, `staticcheck`, `misspell`, `unconvert`, `goimports`, plus `depguard` as the hard modularity hold (`.golangci.yml` fences are the package map; overnight GNHF may shrink them, feature work must not widen an allow-list or remove a deny without an ADR — docs/design/quality-controls.md C7, docs/adr/0004-deep-module-depguard-boundaries.md, docs/adr/0023-depguard-allow-list-widens-are-adr-class.md). No `//nolint` without justification. Current thresholds: statement coverage cmd ≥83% / internal ≥86% (per-tree), mutant-coverage ≥92%, efficacy ≥99%. Ratchet policy, amended by ADR 0015 Decision 3 (docs/adr/0015-recalibrate-mutation-and-coverage-gates.md): statement-coverage thresholds are seeded a point or so below each tree's measured value and ratchet upward from there; `mutant-coverage` is a secondary reachability proxy held at a single repo-wide 92 after the one-time Decision 2 correction (removal of an unintended double gate with per-tree statement coverage), and ratchets upward from there; efficacy is ratchet-only-up, unamended. Lowering any threshold requires a new ADR.

This is the commit/push gate. It's distinct from `arm validate --ci` / `arm doctor`, which is the task-completion sanity check — see [workflow.md](workflow.md).

`check` runs the unit suite exactly once: `coverage` runs `go test -coverprofile=coverage.out`, and `coverage-check` depends on `coverage` then reads that same profile rather than re-running the suite (docs/design/gate-efficiency.md D3). `make check` has no prerequisites; it loops `make check-job` so `make -j check` stays serial. Standalone `make coverage-check` generates a current profile first. `test-coverage-check` exercises the threshold script on fixtures and does not re-run the suite.

## CI required checks

PR wall is the **max** of three required GitHub Actions jobs (no `needs` between them, no aggregator job named `check`). Rigor matches local `make check`; only the schedule changes.

| Context | Runs |
|---|---|
| `check-core` | coverage suite once + thresholds, validate-skills, validate-doc-examples, census-drift, git-test hermetic |
| `check-static` | lint (pinned golangci-lint), crosscompile, census/coverage/hermetic self-tests, check-job drift test |
| `mutate` | gremlins on `./internal` then `./cmd` (full, every PR) |

CI calls `make check-job JOB=<id>` only — the membership table in the Makefile is the only step list. Expected wall is ~max of the two ~4m poles plus setup (~6m), not the former ~13m serial `check` job.

Keep `e2eharness`, `validate-graph`, and the `test-os` matrix as they are. Ubuntu `test-os` skips `make test-ci` on pull_request/push (check-core already ran the suite) and still runs it when this workflow is called with `os-matrix-only` (tag releases skip check-core). macos and windows keep their matrix tests. Default ruleset must require the three new context names (copy the same string form already stored for `e2eharness`) and must drop `check`.

## `make check-fast` — diff-routed fast gate

```bash
make check-fast          # routes off git diff against merge-base HEAD origin/main
BASE=<ref> make check-fast   # override the diff base (e.g. in a worktree or detached HEAD)
```

`scripts/check-fast.sh` computes the changed files against `BASE` (default
`git merge-base HEAD origin/main`) and runs only the steps implied by the
changed surfaces:

| Changed surface | Steps |
|---|---|
| `**/*.go` | lint + build + `go test` on changed packages **plus reverse importers** (`go list`) |
| `skills/**`, `docs/skills/**`, `internal/skillsembed/skills/**`, `.agents/skills/**` | `validate-skills`, `validate-doc-examples` |
| `cmd/**`, `docs/design/surface-census.md`, `docs/commands.md` | `census-drift-check` |
| docs only | `adr-principles` lint only |

The fast gate never runs mutation testing, full coverage, or crosscompile —
those stay exclusive to `make check`. `scripts/test-check-fast.sh` (wired via
`make test-check-fast`) exercises the routing logic itself: for each surface
it asserts both that the right steps run and that the wrong ones (mutate,
coverage, crosscompile, unrelated validators) do not.

## Commands

```bash
make build           # build ./bin/arm
make install         # build → ~/.local/bin/arm
make test            # run all tests
make check-fast      # diff-routed fast gate (see above)
make check-job JOB=check-core  # one CI partition (also check-static, mutate)
make test-check-jobs # drift test: ci.yml job ids vs Makefile table
make test-check-fast # test check-fast.sh routing itself
make lint            # golangci-lint run ./...
make coverage        # run unit suite once with -coverprofile, generate coverage.html
make coverage-check  # run coverage, then fail if cmd < 83% or internal < 86%
make mutate          # gremlins mutation testing on ./internal then ./cmd
make validate-skills # validate embedded skill source
make skill           # deploy skills to .claude/skills/, .gemini/skills/, .codex/skills/
make clean           # remove bin/, dist/, *.out, coverage.html, mutesting-report/, .claude/skills/

go test ./internal/materialize/... -run TestApplyOp   # single test
go test -v ./internal/materialize/...                 # verbose
```

Dependencies: `golangci-lint` and `gremlins` must be on PATH.
