# Orchestrate Runtime Roadmap

## Purpose

This roadmap captures the recommended next steps after the orchestrator runtime
brainstorming. It is intended to bridge the current design notes into a more
formal implementation planning phase without forcing all open questions to be
resolved up front.

The roadmap is sequenced to:

1. lock the product boundaries
2. turn the current brainstorm into explicit design decisions
3. define the minimum viable runtime model
4. defer deeper implementation planning until the design surface is stable

## Phase 1: Stabilize The Design Surface

### 1. Lock product boundaries

Decide and document:

- whether the runtime will surface as `arm worker run`, an expanded
  `arm orchestrate --loop`, or remain temporarily undecided
- what remains deterministic-only
- what is allowed into the exception-agent lane
- what is explicitly out of scope for v1

Output:

- a short boundary decision note appended to
  `orchestrate-runtime-direction.md`

### 2. Create a compact decision record

For the current design notes, extract:

- decisions made
- assumptions
- open questions
- deferred questions

Output:

- a single concise decision record document or a short decision appendix in the
  index

### 3. Refine DoR and DoE into enforceable checks

For each identified check, define:

- exact signal
- owning command or subsystem
- when it runs
- resulting outcome class:
  - `pass`
  - `informational`
  - `policy_evaluable`
  - `blocked`

Output:

- an enforceable DoR/DoE matrix suitable for implementation planning

## Phase 2: Define The Runtime Control Model

### 4. Specify the reusable policy sub-workflows

Fully define the five recommended sub-workflows:

- `task_contract_review`
- `scope_and_dependency_resolution`
- `execution_lane_routing`
- `provenance_review`
- `runtime_recovery`

For each, specify:

- inputs
- deterministic rules
- policy knobs
- outputs
- exception-agent entry conditions
- human escalation conditions

Output:

- `docs/design/orchestrate-policy-subworkflow-specification.md`

### 5. Define the worker runtime state machine

Design the state machine above the existing single-task orchestrator.

Candidate states:

- `idle`
- `polling`
- `claim_pending`
- `claim_lost`
- `claim_won`
- `executing`
- `recovering`
- `paused`
- `escalated`
- `stopped`

For each state transition, define:

- trigger
- required checks
- emitted audit record
- next state

Output:

- `docs/design/orchestrate-worker-runtime-state-machine.md`

### 6. Define the policy model

Write down the configurable policy surface, including:

- retry budgets
- cooldown rules
- worker tiers
- model and harness fallback order
- quota exhaustion behavior
- decomposition thresholds
- escalation thresholds

Output:

- `docs/design/orchestrate-runtime-policy-model.md`

### 7. Define the audit model

Specify the audit records needed for:

- policy evaluations
- exception-agent actions
- retries
- reroutes
- cooldowns
- human escalations
- rationale notes

Output:

- `docs/design/orchestrate-audit-model.md`

### Phase 2 Status

Phase 2 now defines the control-model documentation set as four focused notes
plus index updates. These notes describe control semantics, policy surfaces,
audit expectations, and runtime gates without claiming that the worker runtime
implementation, final CLI shape, or final `v1` slice is decided.

## Phase 3: Reconcile With Existing Armature Surfaces

### 8. Perform an architecture and command gap review

Compare the proposed runtime against what already exists:

- `ready`
- `claim`
- `orchestrate`
- `doctor`
- `validate`
- `dag-summary`
- planner and auditor skills
- existing orchestrator engine and retry logic

Goals:

- identify direct reuse opportunities
- find contradictions with current architecture
- identify new commands, flags, or ops that would be required

Output:

- a gap analysis document with:
  - reuse
  - change
  - add
  - deprecate

### 9. Perform a Go OSS review for embedded workflow support

Once the requirements are sharp enough, review active, well-regarded Go
libraries for embedded workflow execution.

Evaluation criteria:

- embedded, not distributed-first
- deterministic state-machine friendliness
- auditability
- low operational complexity
- active maintenance
- fit with Armature's git-native architecture

Output:

- an OSS review note with recommendation:
  - build directly
  - selectively borrow
  - integrate a library

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

## Phase 4: Choose And Plan A V1 Slice

### 10. Define the thinnest valuable v1

Recommended bias for v1:

- deterministic worker runtime
- existing single-task orchestrator retained
- no persistent agent supervisor per worker
- a narrow exception lane
- strong audit hooks
- human escalation available as fallback

Questions to answer:

- which sub-workflows are required in v1?
- which are deferred?
- does v1 include exception-agent execution or only the deterministic hooks for
  it?

Output:

- a v1 scope note

### 11. Write the implementation plan

Break the chosen v1 into:

- epics
- stories
- tasks
- acceptance criteria

Output:

- implementation plan suitable for decomposition into Armature work items

### Phase 4 Status

Phase 4 is completed as a docs-first v1 slicing pass:

- `docs/design/orchestrate-runtime-v1-scope.md` selects the thinnest valuable
  v1 runtime slice.
- `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md` decomposes the
  selected slice into implementation work.

Phase 4 intentionally does not add production runtime code, command behavior,
policy parsing, or audit op schemas. Those belong to the v1 implementation
plan.

## Phase 5: Resume Targeted Deep Dives

After the above is stable, resume deeper brainstorming only where needed.

Most likely targets:

- `runtime_recovery`
- `execution_lane_routing`
- audit schema
- runtime CLI shape
- exception-agent action envelope

The key shift is from broad ideation to focused problem-solving.

## Recommended Order

If only a small amount of time is available, prioritize:

1. lock product boundaries
2. refine DoR/DoE into enforceable checks
3. define the five sub-workflows
4. define the runtime state machine
5. define the audit model

This sequence should produce enough clarity to decide whether the runtime is
ready for implementation planning.
