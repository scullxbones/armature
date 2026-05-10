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

## Resolved Questions

- The exact DoR and DoE signals, owners, run timing, outcome classes, and
  initial policy routes are defined in
  `docs/design/orchestrate-readiness-and-executability.md`.

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
