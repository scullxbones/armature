# Orchestrate Runtime Phase 2 Control Model Design

**Date:** 2026-05-10
**Status:** Draft

---

## Overview

This document captures the approved Phase 2 design shape for the Armature
orchestrate runtime work. Phase 2 defines the runtime control model as one
coherent documentation chunk, implemented as four focused design notes plus
index and roadmap updates.

The purpose is to turn the Phase 1 boundary into explicit runtime design
surfaces without choosing the final CLI shape or starting implementation of the
worker runtime itself.

---

## Phase 2 Intent

Phase 2 should answer:

- how reusable policy sub-workflows resolve `policy_evaluable` conditions
- how the worker runtime moves between states above the existing single-task
  orchestrator
- which runtime policy settings are defaults and which are tunable
- which audit events and shared fields are required for traceability

Phase 2 is a single chunk of work, but its outputs should be split into focused
documents so each surface remains readable.

---

## Phase 1 Constraints

The Phase 2 control model must preserve the Phase 1 boundary:

- the runtime wraps the existing single-task orchestrator; it does not replace
  it
- final CLI naming remains out of scope
- normal queue draining remains deterministic
- `policy_evaluable` does not automatically mean agent-worthy
- exception agents are narrow bounded recovery tools
- exception agents may run only from explicitly enumerated entry conditions
- at most one bounded exception-agent recovery attempt is allowed per triggering
  condition before escalation or cooldown
- exception-agent safety must be enforced by deterministic runtime controls, not
  prompt-only guidance
- exception agents may not broaden scope, rewrite task intent, or decompose work
  autonomously
- human escalation remains the narrowest lane

---

## Documentation Outputs

Phase 2 should create four focused design notes:

1. `docs/design/orchestrate-policy-subworkflow-specification.md`
2. `docs/design/orchestrate-worker-runtime-state-machine.md`
3. `docs/design/orchestrate-runtime-policy-model.md`
4. `docs/design/orchestrate-audit-model.md`

It should also update:

- `docs/design/orchestrate-runtime-index.md`
- `docs/design/orchestrate-runtime-roadmap.md`

The index update should add the four new documents to the runtime design index.
The roadmap update should show Phase 2 outputs as defined, while leaving later
implementation and `v1` scope choices for subsequent phases.

---

## Policy Sub-Workflow Specification

`docs/design/orchestrate-policy-subworkflow-specification.md` should fully
define the five reusable sub-workflows from the roadmap:

- `task_contract_review`
- `scope_and_dependency_resolution`
- `execution_lane_routing`
- `provenance_review`
- `runtime_recovery`

The document should define a shared invocation model with common inputs:

- triggering check ID
- task metadata
- current DoR or DoE result
- policy profile
- evidence bundle
- worker or runtime context when applicable
- prior attempts and audit history

The shared result shape should include:

- `workflow`
- `outcome`
- `reason_code`
- `confidence`
- `evidence`
- `constraints`
- `next_action`
- `policy_decision`
- `agent_allowed`
- `agent_action_envelope`
- `human_required`
- `audit_events`
- `retry_or_recheck_after`
- `terminal_for_current_attempt`

For each sub-workflow, the note should specify:

- purpose
- feeds from DoR or DoE checks
- inputs
- deterministic rules
- policy knobs
- outputs
- exception-agent entry conditions
- human escalation conditions

The document should make clear that deterministic rules run first, policy may
narrow or rank outcomes, and exception-agent entry is considered only when
multiple policy-permitted actions remain plausible and deterministic policy
cannot choose safely.

---

## Worker Runtime State Machine

`docs/design/orchestrate-worker-runtime-state-machine.md` should define the
deterministic state machine above the existing single-task orchestrator.

The state catalog should include:

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

Each transition row should define:

- `from_state`
- trigger
- required checks
- policy route, when applicable
- audit event
- side effects
- next state
- retry or cooldown budget impact
- terminality

The state-machine note should cover:

- normal queue-control transitions
- claim win and claim loss handling
- execution handoff to the existing single-task orchestrator
- deterministic recovery transitions
- bounded exception-agent recovery transitions
- pause and resume behavior
- human escalation behavior
- graceful and checkpointed stop behavior
- required checks by gate
- audit event stubs emitted by each transition class

The note should preserve the distinction between normal control flow and real
exceptions. Outcomes such as `claim_lost`, `no_ready_work`, `idle_timeout`,
`stale_claim_observed`, `task_already_complete`, and
`task_already_escalated` remain deterministic runtime outcomes.

---

## Runtime Policy Model

`docs/design/orchestrate-runtime-policy-model.md` should define the configurable
policy surface that the runtime and sub-workflows rely on.

The document should distinguish built-in defaults from tunables for:

- retry budgets
- cooldown and backoff rules
- worker tiers
- model fallback order
- harness fallback order
- quota exhaustion behavior
- decomposition thresholds
- escalation thresholds

The policy note should define top-level policy groups such as:

- `retry`
- `cooldown`
- `workers`
- `models`
- `harnesses`
- `quota`
- `decomposition`
- `escalation`
- `subworkflows`

It should state the default runtime posture:

- use conservative built-in defaults when repo-specific policy is absent
- choose exactly one safe policy-approved fallback deterministically
- route ambiguous permitted choices through the relevant sub-workflow
- allow at most one bounded exception-agent recovery attempt for selected
  `v1` cases
- escalate or cooldown when recovery remains ambiguous or exhausted

The policy model should link its settings to the sub-workflows and state-machine
transitions that consume them, but it should not define transition mechanics or
the final on-disk policy format.

---

## Audit Model

`docs/design/orchestrate-audit-model.md` should define the audit records needed
for Phase 2 runtime control.

The audit model should cover:

- policy evaluations
- exception-agent invocations
- exception-agent action selections
- retries
- reroutes
- cooldowns
- human escalations
- human escalation resolutions
- rationale notes

The shared audit record fields should include:

- `event_id`
- `event_type`
- `schema_version`
- `created_at`
- `actor_type`
- `actor_id`
- `issue_id`
- `worker_id`
- `run_id`
- `attempt`
- `correlation_id`
- `causation_event_id`
- `policy_ref`
- `inputs_digest`
- `evidence`
- `outcome`
- `next_action`

The event catalog should include at least:

- `policy_evaluation_recorded`
- `retry_scheduled`
- `reroute_selected`
- `cooldown_started`
- `exception_agent_invoked`
- `exception_agent_action_selected`
- `human_escalation_created`
- `human_escalation_resolved`
- `rationale_note_added`

The audit model should preserve Armature's git-native direction:

- audit records are repo-visible artifacts, not hidden service state
- records are append-only by default
- corrections use superseding records rather than mutation
- materialized runtime state can be rebuilt from ordered audit events
- records are human-readable and machine-parseable
- exception-agent records prove the runtime enforced a bounded action envelope
  outside the agent prompt

---

## Cross-Document Consistency

The four Phase 2 documents should use shared terminology for:

- DoR and DoE outcome classes
- `policy_evaluable` routing
- sub-workflow result fields
- exception-agent entry conditions
- human escalation conditions
- state transition audit events
- retry and cooldown budget language

The policy sub-workflow specification should define the shared result shape.
The audit model should define how those results are recorded. The policy model
should define the default and tunable inputs that shape those results. The state
machine should define when those results affect runtime transitions.

No Phase 2 document should claim that the runtime implementation is ready, that
the final CLI surface is chosen, or that `v1` scope has been finalized.

---

## Parallel Work Strategy

The four design notes are suitable for parallel drafting because they have
separate write scopes:

- one worker drafts the policy sub-workflow specification
- one worker drafts the state-machine note
- one worker drafts the policy-model note
- one worker drafts the audit-model note

The integration pass should remain centralized. It should reconcile terminology,
cross-references, event names, shared field names, and Phase 1 constraints across
all four documents before the Phase 2 chunk is considered complete.

---

## Acceptance Criteria

Phase 2 is complete when:

- all four focused design notes exist
- each design note covers the fields required by this spec
- `docs/design/orchestrate-runtime-index.md` lists the four new design notes
- `docs/design/orchestrate-runtime-roadmap.md` reflects the Phase 2 outputs
- shared result fields are consistent between the sub-workflow and audit notes
- state-machine audit events are represented in the audit model or explicitly
  identified as stubs for later expansion
- policy defaults and tunables are clearly separated
- exception-agent entry conditions preserve the Phase 1 boundary
- human escalation remains narrower than general recovery
- no document chooses the final runtime CLI shape
- no document allows autonomous decomposition or broad task rewriting in `v1`
- a placeholder scan finds no `TBD`, `TODO`, or incomplete sections
- `git diff --check` reports no whitespace problems

---

## Deferred Questions

Phase 2 intentionally leaves these questions open for later phases:

- the final runtime CLI command shape
- the exact on-disk policy configuration format
- the exact on-disk audit event format
- durable checkpoint representation
- whether `claim_lost` should remain a state or become only an audited
  transition outcome
- which exception-agent recovery cases are included in the thinnest valuable
  `v1`
- whether to build the runtime directly, selectively borrow from a Go workflow
  library, or integrate a library
- the final `v1` implementation slice
