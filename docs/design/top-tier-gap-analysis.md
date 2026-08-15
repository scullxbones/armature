# Top-Tier Gap Analysis

**Date:** 2026-07-07
**Purpose:** Identify the five biggest technical gaps and five biggest documentation gaps between Armature's current state and a top-tier open-source project, with incremental changes to close each. This document is the citation source for the TOPTIER epic.

**Grounding:** CONTEXT.md glossary, docs/adr/, docs/design/architecture.md, the dogfood findings corpus (docs/dogfood/findings/), CI/release configuration, and the repository code layout as of this date.

**Headline:** The core engine (event-sourced ops, per-tree coverage gate (cmd ≥83%, internal ≥86%), mutation testing, ADRs, domain glossary) is strong for the project's age. The gaps concentrate in the agent-facing workflow layer — skills, schemas, recovery — and in adopter-facing documentation.

---

## Technical Gaps

### T1. Skills are the product's real API surface but the least-tested code in the repo

Dogfood evidence: the coordinator skill omitted the mandatory `--worktree` flag so every wave dispatch failed; its `--grep="feat(...)"` pseudocode silently dropped non-`feat` commits; `arm review record --bundle` was documented taking JSON content when it takes a file path. `make check` passed through all of these because skills are markdown prose with shell pseudocode; semantic bugs only surface during live dogfooding.

Incremental changes:

- **T1.1 Skill lint in CI**: extract every command invocation in the skill files into fenced, tagged blocks and add a CI step that verifies each against the real CLI (flags exist, required flags present) using a fixture repo — a semantic check beyond structural `validate-skills`.
- **T1.2 Golden-transcript tests**: fixture repo plus scripted coordinator/worker command sequences taken from each skill, asserted end-to-end in CI.
- **T1.3 Shrink the pseudocode surface**: where skills contain shell pseudocode encoding real logic (e.g. commit discovery by grep), replace it with real `arm` subcommands (e.g. `arm review commits <task-id>`) so the logic is compiled and testable.

### T2. No end-to-end workflow test harness — the composed multi-agent loop is validated only by dogfooding

Unit coverage is strong, but findings like "JSON string/int mismatch hidden by tests", "worker left task claimed", and "recovery skips worker dispatch" show the composed lifecycle (bootstrap → decompose → claim → work → review → merge detection) has no automated exercise. Only two `*_integration_test.go` files exist.

Incremental changes:

- **T2.1 Test harness package**: spin up a bare "origin" repo plus N clones and drive the full lifecycle with real `git` commands; land one happy-path lifecycle test.
- **T2.2 Failure-class scenario tests**: stale-claim expiry/recovery, two workers racing a claim, coordinator crash recovery, concurrent ops-branch pushes — the known classes from the dogfood themes.
- **T2.3 Strict-decode round-trip suite**: plan.json → decompose-apply → render-context → assessment JSON with `DisallowUnknownFields`, killing the string/int mismatch class of bug.

### T3. Crash/interruption resilience is under-engineered relative to the coordination-system claim

Dogfood themes `session-recovery-gaps` and `parallel-coordination-conflicts` document branch divergence after recovery, parallel checkout races, semantic reverts git does not flag, and orphaned claims. A coordination system's value is precisely how it behaves when a worker dies mid-task.

Incremental changes:

- **T3.1 Recovery state machine doc**: for every issue status × claim liveness combination, define what `arm doctor` / the coordinator should do.
- **T3.2 Deterministic reconciliation**: `arm recover` (or `arm doctor --fix`) that reconciles stale claims, unbound worktrees, and half-recorded transitions — today that logic lives in skill prose.
- **T3.3 Actionable claim expiry**: `arm ready` surfaces expired-claim issues distinctly rather than omitting or silently including them.
- **T3.4 Replay convergence test**: kill the process at each op-append point and assert materialization converges (folds into T2 harness).

### T4. Scope enforcement via the harness hook has demonstrated bypasses

Findings: a worker edited the Makefile out of scope, left a stray binary, and worktree changes leaked into the main worktree. ADR 0007 (path-based binding) is the right direction, but the hook's guarantees are not verified adversarially, and pass-through is broad.

Incremental changes:

- **T4.1 Hook conformance suite**: a matrix of (binding state × tool × path) with expected allow/deny decisions, run in CI.
- **T4.2 Violation visibility**: log scope violations even in pass-through; add a `doctor` check for out-of-scope artifacts after a task completes (stray binaries, dirty main worktree).
- **T4.3 Threat model doc**: state honestly what the hook does and does not guarantee (advisory vs. enforcing), per platform.

### T5. Distribution and compatibility maturity

CI is ubuntu-only while goreleaser ships untested windows/darwin binaries; the project sits at v0.0.1 with no CHANGELOG; the ops log is append-only forever yet there is no documented schema versioning/migration policy or compatibility contract.

Incremental changes:

- **T5.1 CI OS matrix**: run at least `make test build` on ubuntu/macos/windows; gate releases on it.
- **T5.2 Ops-schema compatibility policy**: every op record carries a version; old binaries fail loudly on newer ops; a fixture corpus of v1 ops that every future version must replay identically.
- **T5.3 Real releases**: cut v0.1.0 with a CHANGELOG (Keep a Changelog format), `go install` instructions, then Homebrew tap/scoop incrementally.

---

## Documentation Gaps

### D1. The README quickstart has drift and duplication — the front door doesn't match the product

Step 3 ("Install Skills") repeats `arm bootstrap` from step 1 and admits it; README says the ops worktree is `.arm/` without the `.armature/` state-dir distinction getting-started makes; no visual despite a working TUI.

Incremental changes:

- **D1.1 Quickstart rewrite, executed in CI**: rewrite against a fresh clone running every command verbatim; delete the duplicate step; add a CI job that executes the quickstart script in a scratch repo.
- **D1.2 Visuals**: one screenshot or asciinema of `arm ready` / the TUI in the README.

### D2. Envelope gaps: docs show payloads but omit wrappers, flags, and file-vs-content contracts

The project's own top documented dogfood theme: plan JSON envelope missing from the planner skill, `--worktree` omitted from coordinator examples, `--bundle` path-vs-content confusion, `_REQ_<TASK-ID>` naming convention existing nowhere.

Incremental changes:

- **D2.1 Published JSON Schemas**: in-repo schemas for plan, review bundle, conformance assessment, and activity index; every doc that mentions an artifact links its schema; CI validates all doc examples against the schemas.
- **D2.2 Canonical examples from the CLI**: embed `arm dag apply --example` (and equivalents) output into the skills at build time so examples cannot drift.
- **D2.3 Conventions page**: test naming (`_REQ_<ID>`), commit message format, branch naming — linked from worker/planner/coordinator skills.

### D3. No adopter-facing positioning

README states features but never argues the comparison against GitHub Issues + a prompt, plain markdown task lists, or other agent-orchestration tools; no worked example artifact; no demo. The market analysis exists but is internal.

Incremental changes:

- **D3.1 "Why Armature / Alternatives" doc**: honest comparison table, linked from README.
- **D3.2 Example artifact trail**: an `examples/` dir (or example repository) containing a completed story — sources, plan.json, ops log, PR link — inspectable without running anything.
- **D3.3 Demo recording**: one asciinema/short video of coordinator + two workers end-to-end.

### D4. No community/contribution scaffolding

No CONTRIBUTING.md, CHANGELOG.md, SECURITY.md, issue/PR templates, or code of conduct; `docs/agents/` covers AI contributors thoroughly while human contributors get nothing.

Incremental changes (single slice):

- **D4.1** CONTRIBUTING.md pointing at `make check`, quality-gates, and the ADR process; SECURITY.md with a disclosure contact (relevant because the harness hook mediates agent tool calls); GitHub issue templates including a dogfood-finding template mirroring the internal format.

### D5. The doc corpus is internal-heavy and its authority structure is implicit

`docs/design/` mixes the authoritative architecture doc with superseded artifacts (trellis-prd, gap-resolutions, rename docs) despite an `archive/` dir existing; the ADR dir has a stray un-numbered `adr-context-files.md`; no index states which doc is canonical for what; ~30 `internal/` packages lack package-level godoc.

Incremental changes:

- **D5.1 Archive and index**: move superseded design docs to `docs/archive/` with "superseded by" headers; add `docs/README.md` as a canonical-docs index with audience and status per file.
- **D5.2 ADR hygiene**: renumber/rename the stray ADR; add an ADR template with status field (proposed/accepted/superseded).
- **D5.3 Package godoc pass**: one-paragraph package comment for each `internal/` package — the documentation AI workers hit first when navigating code.

---

## Priority

T1 and D2 first (same root cause: the skill/schema surface is untested and underdocumented, and every dogfood failure originated there), then T2, then D1/D3 for adoption, with T5/D4 as background chores.
