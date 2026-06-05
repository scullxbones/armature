# Orchestrate Audit Model

## Purpose

This document defines the audit records needed for the Phase 2 orchestrate
runtime control model. The goal is to preserve traceability for policy
evaluations, bounded exception recovery, retries, reroutes, cooldowns, and
human escalations without introducing hidden service state.

## Audit Principles

- audit records are repo-visible artifacts, not hidden service state
- records are append-only by default
- corrections use superseding records rather than mutation
- materialized runtime state can be rebuilt from ordered audit events
- records are human-readable and machine-parseable
- exception-agent records must prove the runtime enforced a bounded action
  envelope outside the agent prompt

## Scope

Phase 2 audit design covers:

- policy evaluations
- exception-agent invocations
- exception-agent action selections
- retries
- reroutes
- cooldowns
- human escalations
- human escalation resolutions
- rationale notes

It does not define the final on-disk file format, storage layout, or schema
serialization details.

## Shared Audit Record Fields

Every runtime audit event should carry the following shared fields.

| Field | Meaning |
| --- | --- |
| `event_id` | stable unique identifier for the event |
| `event_type` | event catalog name |
| `schema_version` | audit schema version for this event shape |
| `created_at` | event creation timestamp |
| `actor_type` | origin actor type such as runtime, exception_agent, or human |
| `actor_id` | stable identifier for the specific runtime worker, agent, or human actor |
| `issue_id` | issue or task identifier tied to the event |
| `worker_id` | worker identity when runtime-scoped |
| `run_id` | runtime run instance identifier |
| `attempt` | execution or recovery attempt number for the current condition |
| `correlation_id` | groups events belonging to the same task execution or recovery chain |
| `causation_event_id` | points at the event that directly caused this event |
| `policy_ref` | effective policy clause, profile, or decision reference |
| `inputs_digest` | digest of the normalized input set used for the decision |
| `evidence` | machine-parseable evidence references plus readable summary |
| `outcome` | selected result or recorded status |
| `next_action` | deterministic next runtime action implied by the record |

## Shared Field Semantics

- `correlation_id` groups the broader chain; `causation_event_id` identifies
  the immediate parent event
- `policy_ref` should identify both built-in defaults and any repo-specific
  override that materially shaped the decision
- `inputs_digest` lets the runtime prove which evidence set a decision used
  without copying every artifact inline
- `outcome` and `next_action` should align with the shared sub-workflow result
  contract

## Event Catalog

The minimum Phase 2 event catalog is:

- `policy_evaluation_recorded`
- `retry_scheduled`
- `reroute_selected`
- `cooldown_started`
- `exception_agent_invoked`
- `exception_agent_action_selected`
- `human_escalation_created`
- `human_escalation_resolved`
- `rationale_note_added`

The worker state machine also references additional transition-level event stubs
such as claim, poll, pause, resume, and stop events. Those stubs may expand
later, but the catalog above is the minimum control-model set for Phase 2.

## Event Shapes

### `policy_evaluation_recorded`

Captures the result of deterministic policy evaluation or a reusable
sub-workflow.

Additional fields:

- `workflow`
- `triggering_check_id`
- `reason_code`
- `confidence`
- `constraints`
- `human_required`
- `terminal_for_current_attempt`

This event should align directly with the shared sub-workflow result shape in
`orchestrate-policy-subworkflow-specification.md`.

### `retry_scheduled`

Captures a deterministic or policy-directed retry.

Additional fields:

- `retry_class`
- `retry_budget_before`
- `retry_budget_after`
- `retry_after`
- `retry_strategy`

### `reroute_selected`

Captures a policy-approved change in execution lane.

Additional fields:

- `route_from`
- `route_to`
- `route_reason`
- `selected_by`

`selected_by` should identify whether the choice was deterministic policy or a
validated bounded exception-agent action.

### `cooldown_started`

Captures a pause in execution or polling due to a known cooldown posture.

Additional fields:

- `cooldown_class`
- `cooldown_scope`
- `cooldown_until`
- `cooldown_budget_after`

### `exception_agent_invoked`

Captures bounded exception-agent invocation.

Additional fields:

- `workflow`
- `triggering_check_id`
- `allowed_actions`
- `action_envelope`
- `prior_bounded_attempts`

This record is the proof that the runtime enforced the permission boundary
before agent execution.

### `exception_agent_action_selected`

Captures the validated action chosen by the bounded exception agent.

Additional fields:

- `selected_action`
- `selection_rationale`
- `validated_constraints`
- `validated_by`

`validated_by` should identify the deterministic runtime validation step that
accepted the action.

### `human_escalation_created`

Captures creation of a bounded human escalation artifact.

Additional fields:

- `escalation_reason`
- `escalation_class`
- `required_human_decision`
- `attached_context_refs`

### `human_escalation_resolved`

Captures the human resolution of an escalation.

Additional fields:

- `resolution`
- `resolution_actor`
- `resolution_notes`
- `resume_action`

### `rationale_note_added`

Captures supporting rationale that should remain visible even when no state
transition occurs.

Additional fields:

- `note_category`
- `note_text`
- `related_event_ids`

## Shared Result Alignment

The audit model must preserve the shared sub-workflow result contract:

| Result field | Audit representation |
| --- | --- |
| `workflow` | `policy_evaluation_recorded.workflow` or `exception_agent_invoked.workflow` |
| `outcome` | shared audit `outcome` field |
| `reason_code` | `policy_evaluation_recorded.reason_code` |
| `confidence` | `policy_evaluation_recorded.confidence` |
| `evidence` | shared audit `evidence` field |
| `constraints` | `policy_evaluation_recorded.constraints` |
| `next_action` | shared audit `next_action` field |
| `policy_decision` | shared audit `policy_ref` plus rationale summary |
| `agent_allowed` | inferred from invocation result and `exception_agent_invoked` presence |
| `agent_action_envelope` | `exception_agent_invoked.action_envelope` |
| `human_required` | `policy_evaluation_recorded.human_required` or escalation event |
| `audit_events` | emitted event set |
| `retry_or_recheck_after` | `retry_after` or `cooldown_until` |
| `terminal_for_current_attempt` | `policy_evaluation_recorded.terminal_for_current_attempt` |

## State-Machine Coverage

The Phase 2 audit model must cover or leave stubs for the state-machine
transition classes:

- poll lifecycle
- claim lifecycle
- execution lifecycle
- recovery lifecycle
- pause and resume lifecycle
- stop lifecycle
- escalation lifecycle

The control-model minimum event catalog already covers the recovery and
escalation core. The remaining transition classes may initially use event stubs
that later phases expand into more detailed runtime records.

## Actor Types

Use a constrained actor vocabulary:

- `runtime`
- `exception_agent`
- `human`
- `system_materializer`

This keeps audit filters and materialized state rebuilding simple.

## Append-Only Correction Model

If an audit record is wrong:

- do not mutate the original event
- append a superseding event with its own `event_id`
- set `causation_event_id` to the corrected event when appropriate
- include rationale in `rationale_note_added` or the replacement event fields

This preserves traceability and matches Armature's git-native direction.

## Cross-Document Contract

- `orchestrate-policy-subworkflow-specification.md` defines the shared workflow
  result shape recorded here
- `orchestrate-worker-runtime-state-machine.md` defines the transition classes
  that emit these records
- `orchestrate-runtime-policy-model.md` defines the policy references captured
  by `policy_ref`
