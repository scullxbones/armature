# Orchestrate Runtime Phase 1 Decision Record Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create the standalone Phase 1 orchestrate runtime decision ledger and link it from the runtime brainstorm index.

**Architecture:** This is a documentation-only implementation slice. The new decision ledger becomes the compact canonical handoff for roadmap step 2, while the existing source notes remain the detailed rationale. The index gets a short pointer so future readers can discover the ledger without changing the roadmap or reopening design scope.

**Tech Stack:** Markdown, git, ripgrep, sed

**Spec:** `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md`

---

## File Map

### Created Files

- `docs/design/orchestrate-runtime-decisions.md` — standalone decision ledger with locked decisions, derived decisions, assumptions, open questions, deferred questions, and source notes

### Modified Files

- `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md` — update status from `Draft` to `Approved`
- `docs/design/orchestrate-runtime-index.md` — add the decision ledger to the indexed document list

### No-Change Files

- `docs/design/orchestrate-runtime-roadmap.md` — leave unchanged; step 2 completion should not rewrite the roadmap
- `docs/design/orchestrate-runtime-direction.md` — leave unchanged; the boundary note already landed in step 1
- `docs/design/orchestrate-readiness-and-executability.md` — leave unchanged; step 3 will refine DoR/DoE checks later
- `docs/design/orchestrate-exception-taxonomy.md` — leave unchanged; later work will define exact recovery entry cases
- `docs/design/orchestrate-policy-subworkflows.md` — leave unchanged; phase 2 will specify each workflow in detail

---

## Chunk 1: Approve The Design Spec

### Task 1: Mark The Decision Record Spec As Approved

**Files:**
- Modify: `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md`

- [ ] **Step 1: Change the spec status line from `Draft` to `Approved`**

Replace this line near the top of the file:

```markdown
**Status:** Draft
```

with:

```markdown
**Status:** Approved
```

- [ ] **Step 2: Verify the status change**

Run:

```bash
rg -n "^\*\*Status:\*\* " docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md
```

Expected:

```text
3:**Status:** Approved
```

- [ ] **Step 3: Commit the spec status update**

```bash
git add docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md
git commit -m "docs(spec): mark runtime decision record design approved"
```

---

## Chunk 2: Create The Standalone Decision Ledger

### Task 2: Add The Runtime Decision Ledger

**Files:**
- Create: `docs/design/orchestrate-runtime-decisions.md`

- [ ] **Step 1: Create `docs/design/orchestrate-runtime-decisions.md` with this exact content**

```markdown
# Orchestrate Runtime Decision Record

## Purpose

This document is the compact decision ledger for the `arm orchestrate` runtime
brainstorming after Phase 1 step 1. It summarizes the current design state so
future roadmap work can proceed without re-reading every source note.

The deeper design notes remain the rationale source. This ledger distinguishes
decisions already locked in the source documents from conservative derived
decisions that synthesize those notes.

## Locked Decisions

### Embedded deterministic worker runtime

Armature should move toward an embedded deterministic worker runtime that owns
normal queue draining.

Source: `docs/design/orchestrate-runtime-direction.md`

### Runtime wraps the single-task orchestrator

The worker runtime should invoke and wrap the existing single-task orchestrator
in `v1`; it should not replace that orchestrator.

Source: `docs/design/orchestrate-runtime-direction.md`;
`docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

### CLI surface remains undecided

Phase 1 locks runtime semantics first. Future CLI work may explore
`arm worker run`, `arm orchestrate --loop`, or another command shape, but the
name is not decided yet.

Source: `docs/design/orchestrate-runtime-direction.md`;
`docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

### `v1` is a value-first bounded hybrid runtime

The `v1` runtime should deliver practical queue-draining value while keeping
routine control flow deterministic and allowing only narrow bounded recovery
through the exception-agent lane.

Source: `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

### Routine control-plane behavior remains deterministic

Queue polling, claim attempts, claim win/loss handling, retry and backoff
behavior, worker routing, provenance refresh, and safe Definition of
Executability fallback choice remain outside exception-agent discretion.

Source: `docs/design/orchestrate-runtime-direction.md`;
`docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

### Definition of Ready and Definition of Executability are separate

Definition of Ready describes whether a task is well formed and governable
enough to enter the executable pool. Definition of Executability describes
whether a worker class can execute the task right now in the current runtime
environment.

Source: `docs/design/orchestrate-readiness-and-executability.md`

### DoR and DoE use four outcome classes

DoR and DoE checks should classify outcomes as `pass`, `informational`,
`policy_evaluable`, or `blocked` instead of collapsing ambiguous conditions into
a generic warning bucket.

Source: `docs/design/orchestrate-readiness-and-executability.md`

### Exception agents are narrow bounded recovery tools

Exception agents are allowed in `v1` only for explicitly enumerated recovery
cases, at most one bounded recovery attempt per triggering condition, and only
inside a policy-approved action envelope.

Source: `docs/design/orchestrate-runtime-direction.md`;
`docs/design/orchestrate-exception-taxonomy.md`;
`docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

### Human escalation is the narrowest lane

Human escalation should be reserved for unresolved intent conflict, material
provenance disputes, meaningful governance ambiguity, or exhausted recovery
budget.

Source: `docs/design/orchestrate-exception-taxonomy.md`

### Policy-evaluable conditions use reusable sub-workflows

Many `policy_evaluable` conditions should flow through a small set of reusable
decision-oriented sub-workflows rather than each creating bespoke workflow
logic.

Source: `docs/design/orchestrate-policy-subworkflows.md`

### The initial reusable sub-workflow set has five members

The recommended target set is `task_contract_review`,
`scope_and_dependency_resolution`, `execution_lane_routing`,
`provenance_review`, and `runtime_recovery`.

Source: `docs/design/orchestrate-policy-subworkflows.md`

## Derived Decisions

### `policy_evaluable` does not automatically mean agent-worthy

A `policy_evaluable` condition should first enter deterministic policy
resolution or a bounded sub-workflow. It should reach an exception agent only
when multiple policy-permitted actions remain plausible and deterministic policy
cannot choose safely.

Source: derived from `docs/design/orchestrate-readiness-and-executability.md`;
`docs/design/orchestrate-exception-taxonomy.md`;
`docs/design/orchestrate-policy-subworkflows.md`

### Normal runtime control-plane outcomes are not exceptions

Outcomes such as `claim_lost`, `no_ready_work`, `idle_timeout`,
`stale_claim_observed`, `task_already_complete`, and `task_already_escalated`
belong to deterministic runtime control flow rather than the exception lane.

Source: derived from `docs/design/orchestrate-runtime-direction.md`;
`docs/design/orchestrate-exception-taxonomy.md`

### Deterministic recovery precedes exception-agent recovery

Retryable verification failures, timeouts within budget, deterministic harness
fallback, provider cooldown, deterministic repo repair, and stale source refresh
without semantic change should be handled deterministically before any
exception-agent path is considered.

Source: derived from `docs/design/orchestrate-exception-taxonomy.md`;
`docs/design/orchestrate-runtime-direction.md`

### Runtime controls are the policy boundary

Prompts may restate policy, but exception-agent safety depends on deterministic
permission profiles, allowed-action contracts, entry conditions, and validation
outside the agent.

Source: derived from `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

### Shared sub-workflow results are part of runtime integration

Using a shared result shape across policy sub-workflows is a runtime integration
constraint, not just a documentation preference, because it keeps outcomes,
constraints, next actions, and escalation signals consistent.

Source: derived from `docs/design/orchestrate-policy-subworkflows.md`

## Assumptions

- Existing `ready`, `claim`, and `orchestrate` surfaces can provide enough
  reuse for a thin `v1` runtime.
- Existing git-native coordination remains the right persistence and audit
  foundation for the runtime.
- DoR and DoE checks can become enforceable without turning every ambiguous task
  quality concern into human review.
- A small set of reusable policy sub-workflows is sufficient for the first
  runtime planning pass.
- One bounded recovery attempt is enough for selected `v1` exception-agent
  cases before escalation or cooldown.
- Deterministic rules and policy evaluation can identify safe fallback choices
  for at least some model, harness, quota, and provenance conditions.

## Open Questions

- What exact signals make up the enforceable DoR and DoE matrix?
- Which command or subsystem owns each DoR and DoE check?
- When does each DoR and DoE check run?
- Which DoR and DoE outcomes are `pass`, `informational`,
  `policy_evaluable`, or `blocked`?
- What are the exact inputs, deterministic rules, policy knobs, outputs,
  exception-agent entry conditions, and human escalation conditions for each
  reusable policy sub-workflow?
- What is the worker runtime state machine?
- Which transitions emit which audit records?
- What is the complete audit event schema?
- What is the detailed policy configuration surface?
- Which exception-agent recovery cases are included in `v1`?
- Which existing command surfaces can be reused directly, and which need new
  flags or operations?

## Deferred Questions

- What final CLI command shape should expose the runtime?
- Should Armature ever support persistent agent supervisors?
- Should autonomous decomposition or replanning be allowed in a later version?
- Should broad semantic task rewriting ever be allowed, and under what
  governance model?
- Should Armature integrate distributed workflow infrastructure?
- Should Armature build the worker runtime directly, selectively borrow from a
  Go workflow library, or integrate a library?
- What is the thinnest valuable `v1` implementation slice?

## Source Notes

- `docs/design/orchestrate-runtime-direction.md` defines the runtime direction,
  deterministic ownership model, exception-agent posture, and Phase 1 boundary
  note.
- `docs/design/orchestrate-readiness-and-executability.md` separates DoR from
  DoE and defines the preferred outcome classes.
- `docs/design/orchestrate-exception-taxonomy.md` distinguishes normal control
  flow, deterministic recoverables, agent-worthy exceptions, and human
  escalations.
- `docs/design/orchestrate-policy-subworkflows.md` proposes the reusable
  policy sub-workflow set and shared result shape.
- `docs/design/orchestrate-runtime-roadmap.md` sequences the remaining design
  work and identifies this decision record as Phase 1 step 2.
- `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`
  records the approved Phase 1 boundary design that this ledger must preserve.
```

- [ ] **Step 2: Verify all required section headings exist**

Run:

```bash
rg -n "^## (Purpose|Locked Decisions|Derived Decisions|Assumptions|Open Questions|Deferred Questions|Source Notes)$" docs/design/orchestrate-runtime-decisions.md
```

Expected:

```text
<line>:## Purpose
<line>:## Locked Decisions
<line>:## Derived Decisions
<line>:## Assumptions
<line>:## Open Questions
<line>:## Deferred Questions
<line>:## Source Notes
```

- [ ] **Step 3: Verify the ledger preserves the Phase 1 boundary**

Run:

```bash
rg -n "CLI surface remains undecided|Routine control-plane behavior remains deterministic|Exception agents are narrow bounded recovery tools|Human escalation is the narrowest lane" docs/design/orchestrate-runtime-decisions.md
```

Expected:

```text
<line>:### CLI surface remains undecided
<line>:### Routine control-plane behavior remains deterministic
<line>:### Exception agents are narrow bounded recovery tools
<line>:### Human escalation is the narrowest lane
```

- [ ] **Step 4: Commit the new decision ledger**

```bash
git add docs/design/orchestrate-runtime-decisions.md
git commit -m "docs(design): add orchestrate runtime decision record"
```

---

## Chunk 3: Link The Ledger From The Index

### Task 3: Add The Decision Record To The Runtime Brainstorm Index

**Files:**
- Modify: `docs/design/orchestrate-runtime-index.md`

- [ ] **Step 1: Add the decision record entry to the `Documents` list**

In `docs/design/orchestrate-runtime-index.md`, add this entry after the
`orchestrate-runtime-direction.md` entry:

```markdown
- `orchestrate-runtime-decisions.md`
  - Compact decision ledger for locked decisions, derived decisions,
    assumptions, open questions, deferred questions, and source notes.
```

- [ ] **Step 2: Verify the index includes the new document**

Run:

```bash
rg -n "orchestrate-runtime-decisions.md|Compact decision ledger" docs/design/orchestrate-runtime-index.md
```

Expected:

```text
<line>:- `orchestrate-runtime-decisions.md`
<line>:  - Compact decision ledger for locked decisions, derived decisions,
```

- [ ] **Step 3: Commit the index update**

```bash
git add docs/design/orchestrate-runtime-index.md
git commit -m "docs(design): index orchestrate runtime decision record"
```

---

## Chunk 4: Final Consistency Pass

### Task 4: Verify Step 2 Stayed Inside Scope

**Files:**
- Verify: `docs/design/orchestrate-runtime-decisions.md`
- Verify: `docs/design/orchestrate-runtime-index.md`
- Verify: `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md`

- [ ] **Step 1: Confirm the final diff touches only the approved files**

Run:

```bash
git diff --name-only HEAD~3..HEAD
```

Expected:

```text
docs/design/orchestrate-runtime-decisions.md
docs/design/orchestrate-runtime-index.md
docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md
```

- [ ] **Step 2: Confirm the roadmap and source notes were not modified**

Run:

```bash
git diff --name-only HEAD~3..HEAD | rg "orchestrate-runtime-(roadmap|direction)|orchestrate-readiness-and-executability|orchestrate-exception-taxonomy|orchestrate-policy-subworkflows"
```

Expected: no output

- [ ] **Step 3: Run the lightweight Markdown diff sanity check**

Run:

```bash
git diff --check HEAD~3..HEAD
```

Expected: no output

- [ ] **Step 4: Verify the decision ledger does not contain unfinished-work markers**

Run:

```bash
rg -n "TB[D]|TO[D]O|FIX[M]E" docs/design/orchestrate-runtime-decisions.md docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md
```

Expected: no output

- [ ] **Step 5: Summarize the completed step 2 commits**

Run:

```bash
git log --oneline -3
```

Expected:

```text
<sha> docs(design): index orchestrate runtime decision record
<sha> docs(design): add orchestrate runtime decision record
<sha> docs(spec): mark runtime decision record design approved
```

---

## Spec Coverage Check

- `Standalone artifact` is implemented by Task 2 creating `docs/design/orchestrate-runtime-decisions.md`.
- `Locked decisions` are implemented by Task 2 `Locked Decisions`.
- `Derived decisions` are implemented by Task 2 `Derived Decisions` and kept separate from locked decisions.
- `Assumptions` are implemented by Task 2 `Assumptions`.
- `Open questions` are implemented by Task 2 `Open Questions`.
- `Deferred questions` are implemented by Task 2 `Deferred Questions`.
- `Source notes` are implemented by Task 2 `Source Notes`.
- `Discoverability through the index` is implemented by Task 3.
- `No CLI, state-machine, audit-schema, or policy-model design creep` is protected by Task 4 final consistency checks and the no-change file list.

## Unfinished Marker Scan

This plan intentionally contains:

- exact file paths
- exact Markdown content for the new decision ledger
- exact index entry text
- exact verification commands
- expected outputs for each verification step

It intentionally does not contain:

- runtime implementation tasks
- CLI naming decisions
- runtime state-machine design
- audit schema design
- detailed policy model design
