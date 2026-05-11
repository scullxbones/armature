# Orchestrate Runtime Phase 3 Reconciliation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete Phase 3 of the orchestrate runtime roadmap as a docs-only reconciliation pass: compare the Phase 2 runtime model against existing Armature surfaces, review embedded Go workflow-library options, and update the runtime index and roadmap status.

**Architecture:** This is a documentation-only implementation slice. The gap analysis maps the proposed runtime model onto current commands, packages, skills, ops, policy seams, and audit seams. The OSS review evaluates whether future runtime implementation should build directly, selectively borrow library patterns, or integrate a Go workflow library. No production code, command behavior, op schema, or policy format changes are made in this phase.

**Tech Stack:** Markdown, Go source reading, git, ripgrep, sed, official project/repository documentation for OSS review

**Inputs:**
- `docs/design/orchestrate-runtime-index.md`
- `docs/design/orchestrate-runtime-roadmap.md`
- `docs/design/orchestrate-runtime-direction.md`
- `docs/design/orchestrate-runtime-decisions.md`
- `docs/design/orchestrate-readiness-and-executability.md`
- `docs/design/orchestrate-policy-subworkflow-specification.md`
- `docs/design/orchestrate-worker-runtime-state-machine.md`
- `docs/design/orchestrate-runtime-policy-model.md`
- `docs/design/orchestrate-audit-model.md`
- `docs/commands.md`
- `docs/design/architecture.md`
- `cmd/armature/ready.go`
- `cmd/armature/claim.go`
- `cmd/armature/orchestrate.go`
- `cmd/armature/doctor.go`
- `cmd/armature/validate.go`
- `cmd/armature/dagsum.go`
- `internal/ready/compute.go`
- `internal/claim/claim.go`
- `internal/orchestrate/engine.go`
- `internal/orchestrate/state.go`
- `internal/ops/types.go`
- `internal/skillsembed/skills/armature-planner/SKILL.md`
- `internal/skillsembed/skills/armature-auditor/SKILL.md`

---

## File Map

### Created Files

- `docs/design/orchestrate-runtime-gap-analysis.md` — Phase 3 architecture and command gap review organized by reuse, change, add, deprecate, contradictions, and open decisions
- `docs/design/orchestrate-runtime-oss-review.md` — embedded Go workflow-library review with recommendation for the future runtime implementation path

### Modified Files

- `docs/design/orchestrate-runtime-index.md` — add both Phase 3 documents to the brainstorm index and update current position/open follow-ups
- `docs/design/orchestrate-runtime-roadmap.md` — mark Phase 3 as completed docs-only work and clarify that v1 slicing remains Phase 4

### No-Change Files

- `cmd/armature/**` — do not add or modify runtime commands in Phase 3
- `internal/**` — do not add or modify runtime implementation in Phase 3
- `internal/ops/types.go` — do not add new op types or audit schema fields in Phase 3
- `docs/design/orchestrate-runtime-direction.md` — leave the Phase 1 boundary note unchanged
- `docs/design/orchestrate-policy-subworkflow-specification.md` — leave Phase 2 control-model semantics unchanged
- `docs/design/orchestrate-worker-runtime-state-machine.md` — leave the Phase 2 state-machine model unchanged
- `docs/design/orchestrate-runtime-policy-model.md` — leave policy surface semantics unchanged
- `docs/design/orchestrate-audit-model.md` — leave audit-event model semantics unchanged

---

## Chunk 1: Reconfirm Phase 3 Scope

### Task 1: Read The Required Context

**Files:**
- Read: `docs/design/orchestrate-runtime-index.md`
- Read: `docs/design/orchestrate-runtime-roadmap.md`
- Read: `docs/design/orchestrate-runtime-direction.md`
- Read: `docs/design/orchestrate-runtime-decisions.md`
- Read: `docs/design/orchestrate-readiness-and-executability.md`
- Read: `docs/design/orchestrate-policy-subworkflow-specification.md`
- Read: `docs/design/orchestrate-worker-runtime-state-machine.md`
- Read: `docs/design/orchestrate-runtime-policy-model.md`
- Read: `docs/design/orchestrate-audit-model.md`
- Read: `docs/commands.md`
- Read: `docs/design/architecture.md`

- [ ] **Step 1: Read the Phase 3 roadmap and index entries**

Run:

```bash
sed -n '1,220p' docs/design/orchestrate-runtime-index.md
sed -n '1,260p' docs/design/orchestrate-runtime-roadmap.md
```

Expected: Phase 3 contains exactly two roadmap outputs: an architecture and command gap review, and a Go OSS review for embedded workflow support.

- [ ] **Step 2: Read the Phase 1 and Phase 2 design inputs**

Run:

```bash
sed -n '1,260p' docs/design/orchestrate-runtime-direction.md
sed -n '1,260p' docs/design/orchestrate-runtime-decisions.md
sed -n '1,260p' docs/design/orchestrate-readiness-and-executability.md
sed -n '1,260p' docs/design/orchestrate-policy-subworkflow-specification.md
sed -n '1,260p' docs/design/orchestrate-worker-runtime-state-machine.md
sed -n '1,260p' docs/design/orchestrate-runtime-policy-model.md
sed -n '1,260p' docs/design/orchestrate-audit-model.md
```

Expected: the worker runtime remains deterministic for normal queue draining, wraps the existing single-task orchestrator, routes policy ambiguity through reusable sub-workflows, and keeps exception agents bounded by deterministic controls.

- [ ] **Step 3: Read the existing Armature surface references**

Run:

```bash
sed -n '1,260p' docs/commands.md
sed -n '1,260p' docs/design/architecture.md
```

Expected: current Armature surfaces are git-native, append-only, single-binary, and command-oriented around materialization, ready queue calculation, claims, validation, doctor checks, skills, and a single-task orchestrator.

- [ ] **Step 4: Confirm this phase remains docs-only**

Run:

```bash
git status --short
```

Expected: no source files are changed before the docs are created. If unrelated user changes exist, leave them untouched.

---

## Chunk 2: Inspect Current Runtime-Relevant Code Surfaces

### Task 2: Map Existing Commands And Packages

**Files:**
- Read: `cmd/armature/ready.go`
- Read: `cmd/armature/claim.go`
- Read: `cmd/armature/orchestrate.go`
- Read: `cmd/armature/doctor.go`
- Read: `cmd/armature/validate.go`
- Read: `cmd/armature/dagsum.go`
- Read: `internal/ready/compute.go`
- Read: `internal/claim/claim.go`
- Read: `internal/orchestrate/engine.go`
- Read: `internal/orchestrate/state.go`
- Read: `internal/ops/types.go`
- Read: `internal/skillsembed/skills/armature-planner/SKILL.md`
- Read: `internal/skillsembed/skills/armature-auditor/SKILL.md`

- [ ] **Step 1: Read command entry points**

Run:

```bash
sed -n '1,240p' cmd/armature/ready.go
sed -n '1,240p' cmd/armature/claim.go
sed -n '1,260p' cmd/armature/orchestrate.go
sed -n '1,220p' cmd/armature/doctor.go
sed -n '1,220p' cmd/armature/validate.go
sed -n '1,220p' cmd/armature/dagsum.go
```

Expected observations to carry into the gap analysis:

```text
ready: materializes state and computes claimable work, including assignment-aware sorting and stale-claim filtering.
claim: writes claim ops, enforces inferred-node guardrails, and performs scope-overlap checks with force override.
orchestrate: runs one issue at a time, resolves model/harness config, invokes internal/orchestrate.Engine, and emits current orchestration result.
doctor: runs repo health checks D1-D6 and can promote warnings to errors with --strict.
validate: checks graph consistency, traceability, coverage, scope warnings, and CI/strict behavior.
dag-summary: supports review and promotion of draft nodes before workers claim them.
```

- [ ] **Step 2: Read runtime-relevant internal packages**

Run:

```bash
sed -n '1,260p' internal/ready/compute.go
sed -n '1,220p' internal/claim/claim.go
sed -n '1,320p' internal/orchestrate/engine.go
sed -n '1,220p' internal/orchestrate/state.go
sed -n '1,180p' internal/ops/types.go
```

Expected observations to carry into the gap analysis:

```text
internal/ready: already contains deterministic ready-queue logic, but not the full Phase 2 DoR/DoE matrix.
internal/claim: already contains scope-overlap primitives used by claim and orchestrate.
internal/orchestrate.Engine: already has a single-task crash-resumable orchestration state machine with dispatch, verify-fail, retry, escalate, and complete phases.
internal/orchestrate/state: derives single-task orchestration state by replaying ops.
internal/ops/types: has current orchestration op names but not Phase 2 worker runtime audit-event catalog fields.
```

- [ ] **Step 3: Read planner and auditor skill surfaces**

Run:

```bash
sed -n '1,240p' internal/skillsembed/skills/armature-planner/SKILL.md
sed -n '1,240p' internal/skillsembed/skills/armature-auditor/SKILL.md
```

Expected observations to carry into the gap analysis:

```text
armature-planner: owns source registration, decomposition, dag-transition, source linking, dependency linking, validate, doctor, and release-to-coordinator workflow.
armature-auditor: owns post-worker verification, citation integrity, source freshness, outcome quality, scope-overlap resolution, and doctor --strict checks.
```

- [ ] **Step 4: Search for all orchestration op and command references**

Run:

```bash
rg -n "OpOrchestrate|orchestrate|worker|ready|claim|validate|doctor|dag-summary" cmd internal docs/design docs/commands.md
```

Expected: enough cross-references to identify direct reuse, required changes, and missing surfaces without modifying code.

---

## Chunk 3: Create The Gap Analysis

### Task 3: Add `orchestrate-runtime-gap-analysis.md`

**Files:**
- Create: `docs/design/orchestrate-runtime-gap-analysis.md`

- [ ] **Step 1: Create the gap analysis document**

Create `docs/design/orchestrate-runtime-gap-analysis.md` with the Markdown below. During execution, only change claims when the Chunk 1 and Chunk 2 evidence contradicts this text; preserve the required sections.

```markdown
# Orchestrate Runtime Phase 3 Gap Analysis

## Purpose

This document reconciles the Phase 2 orchestrate worker runtime model with
Armature's current commands, packages, skills, and git-native architecture. It
does not choose the final `v1` implementation slice or make code changes.

Phase 3 asks what the future runtime can reuse, what must change, what must be
added, and what should be deprecated or avoided before Phase 4 chooses the
thinnest valuable `v1`.

## Scope

Included:

- `ready`
- `claim`
- `orchestrate`
- `doctor`
- `validate`
- `dag-summary`
- planner and auditor skills
- existing orchestrator engine and retry logic
- existing op and materialization architecture where relevant to runtime audit

Excluded:

- final runtime CLI naming
- final on-disk policy syntax
- final audit serialization schema
- implementation task decomposition for `v1`
- production code changes

## Executive Summary

The proposed worker runtime fits Armature's current architecture if it is treated
as a deterministic queue-control layer above existing surfaces rather than a
replacement for them. The strongest reuse path is to keep `ready`, `claim`, and
the existing single-task `orchestrate` engine as the first implementation
substrate, then add explicit worker-runtime gates, policy-result types, and
runtime audit records around them.

The main gap is not missing execution machinery; it is missing runtime ownership.
Current commands can be run manually in a loop, but no embedded runtime owns
polling, claim contention, cooldowns, pause/resume, reroutes, bounded recovery,
or state-machine auditability end to end.

## Direct Reuse

| Surface | Current responsibility | Phase 2 runtime fit | Reuse posture |
| --- | --- | --- | --- |
| `arm ready` / `internal/ready` | Deterministically computes claimable work from materialized state. | Maps to `poll_gate` candidate discovery and no-ready-work behavior. | Reuse as the initial poll substrate; extend later with explicit DoR/DoE check IDs. |
| `arm claim` / `internal/claim` | Writes claim ops and checks scope overlap. | Maps to `claim_gate`, claim contention, and scope/dependency policy routing. | Reuse claim mechanics; split reusable claim-gate logic from CLI presentation when implementing runtime. |
| `arm orchestrate` / `internal/orchestrate.Engine` | Runs one claimed task through dispatch, verification, retry, escalation, and completion. | Maps to `executing` state as the single-task executor the worker runtime wraps. | Reuse as execution handoff; do not replace in `v1`. |
| `arm validate` | Checks graph consistency, traceability, coverage, and scope warnings. | Supplies evidence for DoR, provenance review, and release/audit gates. | Reuse as validation evidence; do not make runtime call broad validation blindly on every idle loop without cost controls. |
| `arm doctor` | Checks repo health and stale claims. | Supplies operational health evidence for poll, resume, and escalation decisions. | Reuse selected checks as evidence; separate human diagnostics from runtime gates later. |
| `arm dag-summary` | Reviews and promotes draft nodes. | Supports readiness governance before tasks enter the executable pool. | Reuse as human governance surface; runtime should not auto-promote draft work. |
| Planner skill | Creates cited, validated, dependency-aware work DAGs. | Upstream producer of strong DoR inputs. | Reuse as pre-runtime planning lane; runtime should not absorb planner duties in `v1`. |
| Auditor skill | Verifies completed work before sign-off. | Downstream governance and quality gate. | Reuse as post-execution audit lane; runtime can emit evidence but should not replace auditor judgment in `v1`. |

## Required Changes

| Area | Current behavior | Required change before runtime implementation |
| --- | --- | --- |
| Ready classification | Ready queue mostly returns eligible entries or explain strings. | Introduce explicit DoR/DoE check IDs and outcome classes: `pass`, `informational`, `policy_evaluable`, `blocked`. |
| Claim gate | Claim CLI owns overlap presentation and force behavior. | Extract reusable claim-gate decision logic that can return structured outcomes without CLI-only strings. |
| Orchestrator execution | Single-task engine has its own phases and retry budget. | Keep it as the execution engine, but wrap it with worker runtime states: `claim_won`, `executing`, `recovering`, `paused`, `escalated`, `stopped`. |
| Retry and escalation | Retry budget is command-level and verify-failure oriented. | Align retry classes with runtime policy groups: verification persistence, timeout, quota, harness, model, provenance, and task contract classes. |
| Audit surface | Existing orchestration ops record dispatch, check results, retry, escalate, and complete. | Add runtime audit records or op payloads for policy evaluation, cooldown, reroute, bounded exception-agent invocation, pause/resume, and human escalation resolution. |
| Policy surface | Current config has orchestrator defaults, but not Phase 2 runtime policy groups. | Add an explicit runtime policy model later, with conservative built-in defaults and repo overrides. |
| Command docs | Commands document manual surfaces. | Add future runtime command docs only after Phase 4 chooses CLI shape. |

## Required Additions

| Addition | Why it is needed | Likely owner |
| --- | --- | --- |
| Worker runtime package | Owns deterministic queue draining and the Phase 2 state machine above existing commands. | New `internal/workerruntime` or similarly named package in Phase 4/implementation. |
| Structured gate result types | Lets `poll_gate`, `claim_gate`, `execute_gate`, `recovery_gate`, `resume_gate`, and `stop_gate` share result semantics. | Runtime package plus supporting internal packages. |
| Policy evaluation types | Represents built-in defaults, tunables, selected policy clauses, and sub-workflow results. | New policy-focused package or runtime subpackage. |
| Runtime audit writer | Emits append-only, repo-visible runtime audit events aligned with the Phase 2 audit model. | Existing ops layer or a new audit adapter, depending on Phase 4 scope. |
| Cooldown and pause state materialization | Allows the worker to avoid aggressive loops and resume safely. | Materialization plus runtime state derivation. |
| Bounded recovery envelope type | Encodes allowed exception-agent actions outside prompts. | Runtime policy and audit integration. |

## Deprecate Or Avoid

| Candidate | Recommendation | Rationale |
| --- | --- | --- |
| Manual shell loops around `arm ready`, `arm claim`, and `arm orchestrate` | Avoid as the long-term runtime strategy. | Shell loops cannot reliably own pause/resume, policy evaluation, cooldowns, and audit correlation. |
| Treating `policy_evaluable` as agent-worthy by default | Deprecate as a design posture. | Phase 2 requires deterministic and policy reasoning before any bounded exception-agent lane. |
| Broad exception-agent supervisors | Keep out of `v1`. | Violates the Phase 1 bounded hybrid runtime boundary. |
| Runtime-driven autonomous decomposition | Keep out of `v1`. | Planner owns decomposition; runtime may detect pressure but should escalate or require upstream scope change. |
| Hidden service state | Avoid. | Armature remains git-native and repo-visible; runtime state must be reconstructable from materialized state and audit records. |

## Contradictions And Tensions

| Tension | Why it matters | Phase 3 conclusion |
| --- | --- | --- |
| `ready` currently knows less than the Phase 2 DoR/DoE matrix. | Runtime needs structured check outcomes, not just queue inclusion. | This is an implementation gap, not an architecture contradiction. |
| `claim --force` allows human override of overlap warnings. | Runtime must not silently choose risky overlap behavior. | Runtime should route overlap ambiguity through policy/sub-workflow or escalation, not copy CLI force semantics. |
| Existing `orchestrate` escalation is retry-budget exhaustion. | Phase 2 human escalation is broader and more structured. | Preserve current engine escalation as one evidence source; add runtime-level escalation semantics later. |
| `doctor` and `validate` are human/CI-facing commands. | Runtime needs cheap, targeted gates and cannot run expensive diagnostics indiscriminately. | Reuse check logic selectively instead of shelling out to every command on every loop. |
| Current orchestration ops are task-execution oriented. | Phase 2 audit model needs worker-run and policy-decision correlation. | Add runtime audit events in a future implementation slice. |

## Command Surface Gap

| Option | Fit | Tradeoff |
| --- | --- | --- |
| New `arm worker run` command | Cleanly separates queue-draining runtime from single-task `arm orchestrate`. | Adds a new top-level concept and docs burden. |
| Expanded `arm orchestrate --loop` mode | Reuses the existing command name users already know. | Risks conflating single-task execution with worker lifecycle ownership. |
| Temporary internal runtime with no public command | Lets implementation evolve behind tests before CLI choice. | Delays dogfooding and may hide product-shape decisions too long. |

Phase 3 does not choose among these. Phase 4 should choose based on the thinnest
valuable `v1` and the desired user mental model.

## Recommended Phase 4 Inputs

Phase 4 should use this gap analysis to decide:

- whether `v1` exposes a new command or expands `arm orchestrate`
- which DoR/DoE checks become structured runtime gates first
- whether runtime audit events are new op types or structured payloads on an existing audit lane
- which policy groups are needed in the first implementation slice
- whether bounded exception-agent execution is included in `v1` or deferred behind deterministic hooks
- how much of `doctor` and `validate` becomes reusable internal check logic versus human-facing command output
```

- [ ] **Step 2: Verify the gap analysis has all required sections**

Run:

```bash
rg -n "^## (Purpose|Scope|Executive Summary|Direct Reuse|Required Changes|Required Additions|Deprecate Or Avoid|Contradictions And Tensions|Command Surface Gap|Recommended Phase 4 Inputs)" docs/design/orchestrate-runtime-gap-analysis.md
```

Expected:

```text
<line>:## Purpose
<line>:## Scope
<line>:## Executive Summary
<line>:## Direct Reuse
<line>:## Required Changes
<line>:## Required Additions
<line>:## Deprecate Or Avoid
<line>:## Contradictions And Tensions
<line>:## Command Surface Gap
<line>:## Recommended Phase 4 Inputs
```

- [ ] **Step 3: Verify the gap analysis mentions all roadmap-required surfaces**

Run:

```bash
rg -n "`arm ready`|`arm claim`|`arm orchestrate`|`arm doctor`|`arm validate`|`arm dag-summary`|Planner skill|Auditor skill|single-task" docs/design/orchestrate-runtime-gap-analysis.md
```

Expected: every roadmap-required surface appears at least once.

- [ ] **Step 4: Commit the gap analysis**

```bash
git add docs/design/orchestrate-runtime-gap-analysis.md
git commit -m "docs(design): add orchestrate runtime phase 3 gap analysis"
```

---

## Chunk 4: Perform The Go OSS Review

### Task 4: Add `orchestrate-runtime-oss-review.md`

**Files:**
- Create: `docs/design/orchestrate-runtime-oss-review.md`

- [ ] **Step 1: Review active Go workflow/state-machine candidates**

Use official project documentation or repository READMEs for the candidates below. Capture source links in the document so future readers can distinguish current maintenance evidence from architectural inference.

Evaluate at minimum:

```text
Temporal Go SDK
Cadence Go client
go-workflows
River
Watermill
stateless
looplab/fsm
```

Expected: the review compares each candidate against embedded fit, distributed-first posture, deterministic state-machine friendliness, auditability, operational complexity, maintenance, and fit with Armature's git-native architecture.

- [ ] **Step 2: Create the OSS review document**

Create `docs/design/orchestrate-runtime-oss-review.md` with the Markdown below. During execution, update the source links and maintenance notes from current official/repository sources, but keep the Phase 4 recommendation unless current evidence materially changes the architectural fit.

```markdown
# Orchestrate Runtime Go OSS Review

## Purpose

This document evaluates Go libraries that could influence or support the future
orchestrate worker runtime. The review is intentionally scoped to Phase 3: it
recommends an implementation posture, but does not select the `v1` runtime slice
or introduce dependencies.

## Evaluation Criteria

| Criterion | Meaning |
| --- | --- |
| Embedded fit | Can it run inside the `arm` binary without requiring a server, daemon, database, or distributed control plane? |
| Deterministic state-machine friendliness | Does it make explicit state transitions and replayable decisions straightforward? |
| Auditability | Can Armature keep repo-visible, append-only audit records as the source of truth? |
| Operational complexity | Does it preserve Armature's single-binary, git-native deployment posture? |
| Maintenance | Is the project active and credible enough to depend on or borrow from? |
| Architecture fit | Does it complement Armature's existing commands, op logs, materialization, and worker model? |

## Sources Reviewed

Record the official project or repository sources checked during this review.
Use one bullet per candidate, with source title and URL. Prefer official docs,
GitHub repository READMEs, and release/activity pages over secondary summaries.

## Candidate Summary

| Candidate | Category | Embedded fit | Operational complexity | Recommendation |
| --- | --- | --- | --- | --- |
| Temporal Go SDK | Distributed workflow engine | Low | High | Do not integrate for `v1`; borrow workflow-history and retry vocabulary where useful. |
| Cadence Go client | Distributed workflow engine | Low | High | Do not integrate; similar mismatch to Temporal with weaker strategic fit. |
| go-workflows | Go-native workflow engine | Medium | Medium | Consider only as pattern research unless Phase 4 demands durable replay semantics beyond Armature ops. |
| River | Durable job queue | Low to medium | Medium | Do not integrate for runtime control; database-backed jobs conflict with git-native state. |
| Watermill | Event/message workflow toolkit | Low to medium | Medium | Do not integrate for `v1`; messaging abstractions are broader than Armature needs. |
| stateless | In-process finite state machine | High | Low | Selectively borrow state-machine structure if it is maintained and dependency risk is acceptable. |
| looplab/fsm | In-process finite state machine | High | Low | Selectively borrow concepts or evaluate as a small dependency; avoid if hand-rolled transitions stay clearer. |

## Detailed Notes

### Temporal Go SDK

Temporal is strong for distributed durable workflows, retries, activity
execution, workflow history, and operational observability. It is not a natural
fit for Armature `v1` because it expects external service infrastructure and a
workflow-server mental model that conflicts with the single-binary, git-native
constraint.

Phase 4 posture: do not integrate. Borrow concepts such as durable event
history, replay-safe workflow decisions, retry policy vocabulary, and activity
boundaries if useful.

### Cadence Go Client

Cadence has similar distributed-workflow strengths and similar fit problems for
Armature. It adds operational weight and does not align with the repo-visible
op-log source of truth.

Phase 4 posture: do not integrate.

### go-workflows

`go-workflows` is closer to Go-native workflow execution and may be useful as
pattern research for workflow replay and activity boundaries. It still risks
introducing a workflow runtime abstraction before Armature has proven it needs
one beyond append-only ops and deterministic state derivation.

Phase 4 posture: selectively borrow ideas only unless the `v1` slice explicitly
requires a workflow engine abstraction.

### River

River is a durable job queue. It may be operationally attractive in ordinary Go
services, but Armature's coordination state lives in git rather than a database.

Phase 4 posture: do not integrate for `v1`.

### Watermill

Watermill is useful for message-driven systems, but the Phase 2 model is a
local deterministic worker runtime, not a distributed message topology.

Phase 4 posture: do not integrate for `v1`.

### stateless

`stateless` is an in-process state-machine library. Its shape is closer to the
Phase 2 worker state machine than distributed workflow engines are. The main
question is whether a dependency improves clarity enough to justify adding it.

Phase 4 posture: consider as a pattern or small dependency only after comparing
against a direct typed transition table.

### looplab/fsm

`looplab/fsm` is a small in-process finite-state-machine library. It may fit the
runtime state catalog, transition validation, and callbacks, but Armature may
still be clearer with direct typed transitions because audit records and op-log
state derivation are domain-specific.

Phase 4 posture: consider as a pattern or small dependency; prefer direct code
unless the implementation becomes transition-boilerplate heavy.

## Recommendation

Build the Phase 4 `v1` runtime directly, while selectively borrowing patterns
from small in-process state-machine libraries and distributed workflow systems.

Do not integrate a distributed workflow engine for `v1`. Temporal, Cadence,
River, and Watermill solve larger service-infrastructure problems than Armature
currently has, and they conflict with the single-binary, git-native, repo-visible
state model.

The most promising implementation path is:

1. encode the Phase 2 worker states and transitions directly in Go
2. persist runtime decisions through Armature ops or runtime audit records
3. keep materialized state derivable from repo-visible artifacts
4. revisit a small FSM dependency only if direct transition code becomes noisy
5. borrow retry, replay, and activity-boundary language from mature workflow
   engines without inheriting their infrastructure model

## Phase 4 Decision Inputs

Phase 4 should decide:

- whether direct typed transitions are sufficient for `v1`
- whether a small FSM dependency is worth adding
- whether runtime audit events require a new package or can sit beside existing ops
- whether durable replay needs remain satisfied by existing op materialization
- whether any library should be vendored, depended on, or only cited as pattern research
```

- [ ] **Step 3: Verify the OSS review has all required candidates**

Run:

```bash
rg -n "Temporal|Cadence|go-workflows|River|Watermill|stateless|looplab/fsm" docs/design/orchestrate-runtime-oss-review.md
```

Expected: each candidate appears in the candidate summary and detailed notes.

- [ ] **Step 4: Verify the recommendation is explicit**

Run:

```bash
rg -n "Build the Phase 4 `v1` runtime directly|Do not integrate a distributed workflow engine|small FSM dependency" docs/design/orchestrate-runtime-oss-review.md
```

Expected: the recommendation states build directly, borrow patterns selectively, and avoid distributed workflow-engine integration for `v1`.

- [ ] **Step 5: Commit the OSS review**

```bash
git add docs/design/orchestrate-runtime-oss-review.md
git commit -m "docs(design): add orchestrate runtime go oss review"
```

---

## Chunk 5: Update The Index And Roadmap

### Task 5: Link Phase 3 Documents From The Index

**Files:**
- Modify: `docs/design/orchestrate-runtime-index.md`

- [ ] **Step 1: Add the two Phase 3 documents to the `Documents` list**

In `docs/design/orchestrate-runtime-index.md`, add these entries after `orchestrate-audit-model.md`:

```markdown
- `orchestrate-runtime-gap-analysis.md`
  - Phase 3 reconciliation of the proposed runtime model against current
    Armature commands, packages, skills, ops, policy seams, and audit seams.
- `orchestrate-runtime-oss-review.md`
  - Phase 3 review of Go workflow and state-machine libraries, with a
    recommendation to build the `v1` runtime directly while selectively
    borrowing patterns.
```

- [ ] **Step 2: Update the `Current Position` list**

Replace the current numbered list with:

```markdown
1. Queue draining should not depend on the user manually running a loop.
2. Queue draining should not spend LLM tokens on routine polling, claiming, or
   idle behavior.
3. A deterministic embedded worker runtime should own normal execution.
4. Exception agents are allowed, but only for bounded recovery within policy and
   with audit traceability.
5. Phase 3 found that the runtime can wrap and reuse existing `ready`, `claim`,
   and single-task `orchestrate` surfaces, but needs explicit runtime gates,
   policy-result types, cooldown/pause state, and audit records before
   implementation.
6. The Go OSS review recommends building the `v1` runtime directly while
   selectively borrowing state-machine, replay, retry, and activity-boundary
   patterns instead of integrating a distributed workflow engine.
```

- [ ] **Step 3: Update the `Open Follow-Ups` list**

Replace the current `Open Follow-Ups` list with:

```markdown
- Choose the thinnest valuable Phase 4 `v1` runtime slice.
- Decide whether the runtime surface should be a new command such as
  `arm worker run`, an expanded `arm orchestrate --loop` mode, or a temporary
  internal runtime surface.
- Decide whether `v1` includes bounded exception-agent execution or only the
  deterministic hooks and audit envelope for it.
- Define the final on-disk policy and audit serialization formats.
- Turn the chosen `v1` slice into an implementation plan suitable for Armature
  work items.
```

- [ ] **Step 4: Verify the index links both Phase 3 docs and no longer lists Phase 3 review as open**

Run:

```bash
rg -n "orchestrate-runtime-gap-analysis|orchestrate-runtime-oss-review|Perform the Phase 3 architecture" docs/design/orchestrate-runtime-index.md
```

Expected:

```text
<line>:- `orchestrate-runtime-gap-analysis.md`
<line>:- `orchestrate-runtime-oss-review.md`
```

There should be no match for `Perform the Phase 3 architecture`.

- [ ] **Step 5: Commit the index update**

```bash
git add docs/design/orchestrate-runtime-index.md
git commit -m "docs(design): index orchestrate runtime phase 3 notes"
```

### Task 6: Mark Phase 3 Complete In The Roadmap

**Files:**
- Modify: `docs/design/orchestrate-runtime-roadmap.md`

- [ ] **Step 1: Add a Phase 3 status note after roadmap step 9**

After the output block for step 9, add:

```markdown
### Phase 3 Status

Phase 3 is completed as a docs-only reconciliation pass:

- `docs/design/orchestrate-runtime-gap-analysis.md` compares the Phase 2 runtime
  model against current Armature commands, packages, skills, ops, policy seams,
  and audit seams.
- `docs/design/orchestrate-runtime-oss-review.md` recommends building the
  Phase 4 `v1` runtime directly while selectively borrowing library patterns
  rather than integrating a distributed workflow engine.

Phase 3 intentionally does not choose the final CLI shape, final policy/audit
serialization, or thinnest valuable `v1` slice. Those remain Phase 4 decisions.
```

- [ ] **Step 2: Verify the roadmap status note exists**

Run:

```bash
rg -n "Phase 3 Status|docs-only reconciliation|Phase 4 decisions" docs/design/orchestrate-runtime-roadmap.md
```

Expected:

```text
<line>:### Phase 3 Status
<line>:Phase 3 is completed as a docs-only reconciliation pass:
<line>:Phase 3 intentionally does not choose the final CLI shape, final policy/audit
```

- [ ] **Step 3: Commit the roadmap update**

```bash
git add docs/design/orchestrate-runtime-roadmap.md
git commit -m "docs(design): mark orchestrate runtime phase 3 complete"
```

---

## Chunk 6: Final Verification

### Task 7: Verify The Documentation Set

**Files:**
- Verify: `docs/design/orchestrate-runtime-gap-analysis.md`
- Verify: `docs/design/orchestrate-runtime-oss-review.md`
- Verify: `docs/design/orchestrate-runtime-index.md`
- Verify: `docs/design/orchestrate-runtime-roadmap.md`

- [ ] **Step 1: Verify only the approved Phase 3 docs changed in this branch**

Run:

```bash
git diff --name-only HEAD~4..HEAD
```

Expected:

```text
docs/design/orchestrate-runtime-gap-analysis.md
docs/design/orchestrate-runtime-index.md
docs/design/orchestrate-runtime-oss-review.md
docs/design/orchestrate-runtime-roadmap.md
```

- [ ] **Step 2: Search for placeholders**

Run:

```bash
rg -n "TBD|TODO|FIXME|\\[source needed\\]|fill in|placeholder" docs/design/orchestrate-runtime-gap-analysis.md docs/design/orchestrate-runtime-oss-review.md docs/design/orchestrate-runtime-index.md docs/design/orchestrate-runtime-roadmap.md
```

Expected: no matches.

- [ ] **Step 3: Confirm Phase 3 scope did not drift into implementation**

Run:

```bash
git diff --name-only HEAD~4..HEAD | rg -v '^docs/design/orchestrate-runtime-(gap-analysis|oss-review|index|roadmap)\\.md$'
```

Expected: no output.

- [ ] **Step 4: Run repository checks**

Run:

```bash
make check
```

Expected: the repository check suite passes. If failures occur, inspect whether they are unrelated to docs-only changes before modifying anything.

- [ ] **Step 5: Review the final document flow manually**

Run:

```bash
sed -n '1,240p' docs/design/orchestrate-runtime-gap-analysis.md
sed -n '1,240p' docs/design/orchestrate-runtime-oss-review.md
sed -n '1,180p' docs/design/orchestrate-runtime-index.md
sed -n '80,190p' docs/design/orchestrate-runtime-roadmap.md
```

Confirm:

- the gap analysis has clear `reuse`, `change`, `add`, and `deprecate` conclusions
- the OSS review has a clear `build directly` recommendation
- the index points to both Phase 3 docs
- the roadmap marks Phase 3 complete and leaves Phase 4 decisions open

- [ ] **Step 6: Commit any final cleanup**

If final review required edits, commit them:

```bash
git add docs/design/orchestrate-runtime-gap-analysis.md docs/design/orchestrate-runtime-oss-review.md docs/design/orchestrate-runtime-index.md docs/design/orchestrate-runtime-roadmap.md
git commit -m "docs(design): polish orchestrate runtime phase 3 reconciliation"
```

Skip this commit if no cleanup was needed.

---

## Completion Criteria

Phase 3 is complete when:

- `docs/design/orchestrate-runtime-gap-analysis.md` exists and covers reuse, change, add, deprecate, contradictions, and command gaps
- `docs/design/orchestrate-runtime-oss-review.md` exists and recommends build directly, selectively borrow patterns, or integrate a library
- `docs/design/orchestrate-runtime-index.md` links both Phase 3 documents and no longer lists the Phase 3 reviews as open follow-ups
- `docs/design/orchestrate-runtime-roadmap.md` includes a Phase 3 status note
- no production code changed
- placeholder scan is clean
- `make check` passes or any failure is documented as unrelated to docs-only changes
