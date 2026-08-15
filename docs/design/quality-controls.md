# Quality Controls for Armature

**Audience:** Contributors and AI workers **Status:** Active **Scope:** All Go code under `cmd/` and `internal/`

Derived from `docs/design/deterministic-quality-guardrails.md`, adapted to Go and to armature's architecture. Each control maps to one or more guardrails (R1–R9) from that doc. Status is **ACTIVE** (enforced in `make check`), **PARTIAL** (partially in place, gap noted), or **GAP** (not yet enforced).

---

## Controls in `make check`

### C1 — Lint (R3, R4, R5)

**Status: ACTIVE**

`golangci-lint run ./...` with the linters in `.golangci.yml`:

| Linter | What it catches |
|---|---|
| `govet` | Suspicious Go constructs, shadowed variables |
| `errcheck` | Unchecked errors and unchecked type assertions |
| `staticcheck` | Misuse of sync primitives, deprecated APIs, unreachable code |
| `ineffassign` | Assignments whose values are never used |
| `misspell` | Spelling in identifiers and comments |
| `unconvert` | Unnecessary type conversions |
| `goimports` | Import grouping and formatting |

**Gap (R3 — purity):** No lint rule currently bans `time.Now()`, `rand.*`, or `os.Getenv` in domain packages. Six live violations exist in `internal/ready`, `internal/decompose`, and `internal/doctor`. See C4 below for the target state.

**Gap (R1 — mock ban):** No explicit ban on mock libraries. Go's stdlib lacks a mock framework, so this is low-risk today, but `gomock` or `testify/mock` could be introduced. Add `no-restricted-imports` equivalent if a mock package appears in `go.mod`.

### C2 — Tests (R1, R2, R7)

**Status: PARTIAL**

`go test -count=1 ./...` — all tests must pass.

What is in place:
- Port interfaces defined for every external dependency (`GitCommitter`, `GitPusher`, `PendingPushTracker`, `MergeChecker`, `GitConfigPort`, `Provider`, `Screen`).
- Test fakes defined per port in `*_test.go` files adjacent to the package under test.
- No mock framework in `go.mod`.

**Gap (R2 — contract verification):** Fakes are not run against a shared abstract test suite. Each fake is written once and trusted. If a fake's behavior drifts from the real adapter (e.g., error types, ordering, edge-case handling), every unit test built on it tests the wrong contract. The fix is a shared test function per port, parameterized to run against both the fake and the real implementation.

**Gap (R7 — hermeticity):** No CI enforcement that unit tests do not make network calls, sleep, or access the real filesystem outside temp dirs. Currently relies on convention and code review.

### C3 — Coverage (R6)

**Status: ACTIVE**

`go test -coverprofile=coverage.out ./...` with a hard, per-tree threshold: `cmd/**` statement coverage must be **≥ 83%**, `internal/**` must be **≥ 86%** (docs/adr/0015-recalibrate-mutation-and-coverage-gates.md Decision 1).

This measures line execution. It is a floor, not a quality signal — see C5 for the mutation gate that measures assertion strength.

### C4 — Mutation testing (R6)

**Status: ACTIVE**

`gremlins unleash` on `./internal` and `./cmd`, configured in `.gremlins.yaml`:

| Threshold | Value | Meaning |
|---|---|---|
| `threshold-mutant-coverage` | 92% | ≥92% of mutation sites must be reachable by tests |
| `threshold-efficacy` | 99% | ≥99% of executed mutants must be killed |

These thresholds are ratchets, amended by ADR 0015 Decision 3
(docs/adr/0015-recalibrate-mutation-and-coverage-gates.md): statement-coverage
thresholds are seeded a point or so below each tree's measured value and
ratchet upward from there; `mutant-coverage` is a secondary reachability
proxy held at a single repo-wide 92 after the one-time Decision 2 correction
(removal of an unintended double gate with per-tree statement coverage), and
ratchets upward from there. `efficacy` remains ratchet-only-up, unamended.
Lowering any threshold requires a new ADR.

Gremlins is the end-to-end check on test quality. If fakes plus state-based assertions genuinely verify behavior, mutants die. High coverage + low efficacy is the fingerprint of tautological or interaction-theater tests.

### C5 — Build (R5)

**Status: ACTIVE**

`go build ./cmd/armature` — the binary must compile cleanly. Go's type system is strict by default (no implicit conversions, no nil dereferences through the type system, exhaustive switch enforcement available via `staticcheck`).

**Gap (R5 — domain typing):** Task IDs, worker IDs, and story IDs are currently `string`. Wrapping them in `NewType`-equivalent named types (`type TaskID string`) would catch accidental ID cross-assignment at compile time. Not enforced.

---

## Controls not yet in `make check`

### C6 — Clock/randomness purity in domain code (R3)

**Status: GAP**

Domain packages (`ready`, `decompose`, `materialize`, `dag`, `ops`) should not call `time.Now()` directly. Non-determinism in core logic causes flaky tests and makes fakes harder to write.

Current violations:
- `internal/ready/compute.go:35,113` — `time.Now().Unix()`
- `internal/decompose/apply.go:160,185` — `time.Now().Unix()`
- `internal/decompose/revert.go:52` — `time.Now().Unix()`
- `internal/doctor/doctor.go:213` — `time.Now()` (passed to `ready.StaleClaims`)

Target state: introduce a `Clock` port (a function type `type Clock func() int64` or a one-method interface), inject it at the composition root, and add a `staticcheck` or custom vet rule banning bare `time.Now()` in domain packages.

### C7 — Architecture conformance (R4)

**Status: GAP**

No automated check enforces the intended import boundary: adapters (`platform`, `ops`, `sources`) must not be imported by the pure domain (`materialize`, `dag`, `ready`). Currently relies on code review and convention.

Go equivalent of ArchUnit is either:
- A small `TestArchitecture` test in each package using `golang.org/x/tools/go/packages` to assert allowed import sets.
- `depguard` linter (addable to `.golangci.yml`) with explicit deny-lists per package.

### C8 — Contract verification for fakes (R2)

**Status: GAP**

For each port interface that has both a fake (used in unit tests) and a real implementation (used in production), a shared `ContractTest(t, impl Port)` function should exist and be called with both. Ports that currently need this:

| Port | Fake | Real adapter |
|---|---|---|
| `GitCommitter` | `fakeCommitter` in `ops/commit_test.go` | `ops.RealCommitter` (git subprocess) |
| `GitPusher` | `fakePusher` in `ops/pusher_test.go` | `ops.RealPusher` (git subprocess) |
| `MergeChecker` | `stubMergeChecker` in `sync/sync_test.go` | git-backed implementation |
| `GitConfigPort` | inline fake in `platform` tests | `platform.RealGitConfig` |

Contract tests for git-backed ports would run in integration scope (tagged `//go:build integration`) and require a real git repo fixture.

### C9 — Spec traceability (R8)

**Status: GAP — blocked on armature feature**

No requirement IDs are assigned to specs and no test tagging convention exists.

**Dependency:** Full enforcement of C9 requires the requirement-traceability feature described in `docs/superpowers/specs/2026-06-13-deterministic-quality-guardrails-design.md`. That design extends armature's existing source-link and traceability model to parse stable requirement IDs from registered source documents (e.g. `REQ-123` in a PRD or design doc), materialize them as first-class requirement references, and allow issues to link to specific requirement references rather than only whole documents. The completed feature would then let the armature repo itself register its own design docs as sources, link stories to `REQ-*` IDs from those docs, and run `arm validate` as the CI gate — using armature to trace armature's own requirements (the dogfood path described in §"Guardrail Adoption In The Armature Repo" of that spec).

Until that feature ships, the interim path is:
- adopt a test naming convention (`TestClaim_REQ_claimRace`) for new stories
- a small CI script that parses IDs from design docs and checks test names for orphaned references

The `internal/traceability` package is the right home for that interim check, and it should be written to be superseded cleanly by the full `arm validate` gate.

---

## Summary table

| Control | Guardrail | Status | `make check` stage |
|---|---|---|---|
| C1 Lint | R3, R4, R5 | PARTIAL | `make lint` |
| C2 Tests + fakes | R1, R2, R7 | PARTIAL | `make test` |
| C3 Coverage (per-tree: cmd ≥83%, internal ≥86%) | R6 | ACTIVE | `make coverage-check` |
| C4 Mutation (gremlins) | R6 | ACTIVE | `make mutate` |
| C5 Build / type system | R5 | ACTIVE | `make build` |
| C6 Clock purity in domain | R3 | GAP | — |
| C7 Architecture conformance | R4 | GAP | — |
| C8 Fake contract tests | R2 | GAP | — |
| C9 Spec traceability | R8 | GAP | — |

R9 (boundary contracts / breaking-change detection) does not apply: armature is a CLI binary with no service API boundary.

---

## Adoption order for gaps

Following the ratchet strategy from the guardrails doc — apply to new and modified code first, then backfill:

1. **C6 (clock purity)** — six localized call sites; inject `Clock` port, add a lint ban. High signal-to-effort ratio.
2. **C7 (architecture conformance)** — add `depguard` config to `.golangci.yml`; zero false positives if rules are written from the current import graph.
3. **C8 (fake contracts)** — start with `GitCommitter` and `MergeChecker`; add integration test tag to CI.
4. **C9 (traceability)** — adopt test naming convention for new stories; script check is a small CI addition.
5. **C1 extensions** — add mock-library ban to lint config if `testify/mock` or `gomock` appears in `go.mod`.
