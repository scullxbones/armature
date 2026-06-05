# Orchestrate Runtime Policy Model

## Purpose

This document defines the configurable runtime policy surface for the
orchestrate worker runtime and its reusable policy sub-workflows. It separates
built-in defaults from tunables while leaving the final on-disk policy format
for a later phase.

## Design Constraints

- policy config shapes runtime choices; it does not replace deterministic
  transition mechanics
- the final CLI and on-disk policy syntax remain out of scope
- policy may narrow or rank safe choices, but it may not authorize autonomous
  decomposition, broad task rewriting, or unbounded agent behavior
- the runtime uses conservative built-in defaults when repo-specific policy is
  absent

## Default Runtime Posture

The runtime should behave as follows when repo-specific policy is absent:

- use conservative built-in defaults
- choose exactly one safe policy-approved fallback deterministically when such a
  fallback exists
- route ambiguous permitted choices through the relevant reusable sub-workflow
- allow at most one bounded exception-agent recovery attempt for selected `v1`
  cases
- escalate or cooldown when recovery remains ambiguous or exhausted

This posture keeps queue draining deterministic by default while preserving a
narrow bounded recovery lane.

## Policy Group Catalog

The top-level policy groups are:

- `retry`
- `cooldown`
- `workers`
- `models`
- `harnesses`
- `quota`
- `decomposition`
- `escalation`
- `subworkflows`

## Shared Policy Semantics

- built-in defaults are the safe behavior floor used when no repo-specific
  override exists
- tunables are repo- or environment-specific adjustments that may narrow,
  reorder, or cap policy-approved actions
- policy must always resolve to deterministic runtime inputs, not free-form
  prose
- if policy ranking still leaves multiple plausible safe choices, the runtime
  routes to the relevant sub-workflow instead of improvising

## `retry`

### Built-In Defaults

- maintain retry budgets by failure class instead of one global retry count
- allow deterministic retry before any bounded exception-agent lane is
  considered
- treat verification persistence and repeated timeout as separate retry classes
- record each retry attempt through audit events

### Tunables

- max retries by failure class
- whether retry may tighten execution constraints
- whether timeout increase is allowed before reroute
- whether retry counts reset after human resolution or policy-driven reroute

### Consumed By

- `runtime_recovery` in
  `orchestrate-policy-subworkflow-specification.md`
- `executing -> recovering` and `recovering -> executing` transitions in
  `orchestrate-worker-runtime-state-machine.md`

## `cooldown`

### Built-In Defaults

- prefer deterministic cooldown for known provider, harness, or quota
  exhaustion patterns
- make cooldown visible in audit history
- requeue after cooldown instead of looping aggressively

### Tunables

- cooldown durations by failure class
- backoff strategy
- maximum consecutive cooldowns before escalation
- whether cooldown is worker-scoped, provider-scoped, or task-scoped

### Consumed By

- `runtime_recovery`
- `poll_gate` and `recovery_gate` in the worker state machine

## `workers`

### Built-In Defaults

- define a default worker tier suitable for ordinary single-task execution
- allow stronger tiers only when policy or deterministic thresholds justify the
  cost or risk
- do not downgrade below the minimum safe worker tier for the task class

### Tunables

- worker tier catalog
- tier-specific eligibility rules
- concurrency limits by tier
- tier preference by task risk or complexity class

### Consumed By

- `execution_lane_routing`
- `poll_gate`, `claim_gate`, and `execute_gate`

## `models`

### Built-In Defaults

- maintain an ordered preferred model list per worker tier
- use deterministic fallback only when exactly one safe model choice remains
- route model ambiguity through `execution_lane_routing`

### Tunables

- preferred model order
- fallback model order
- model minimums by task class
- downgrade permission thresholds

### Consumed By

- `execution_lane_routing`
- `runtime_recovery`
- `execute_gate` and `recovery_gate`

## `harnesses`

### Built-In Defaults

- maintain an ordered preferred harness list when multiple supported harnesses
  exist
- use deterministic fallback only when exactly one safe harness choice remains
- treat harness unavailability and harness quota exhaustion as policy-visible
  runtime events

### Tunables

- preferred harness order
- fallback harness order
- harness capability requirements by task class
- harness switch restrictions after partial execution

### Consumed By

- `execution_lane_routing`
- `runtime_recovery`
- `execute_gate` and `recovery_gate`

## `quota`

### Built-In Defaults

- prefer cooldown or reroute over repeated failed attempts when quotas or
  credits are exhausted
- distinguish temporary quota exhaustion from structural permission failure
- make quota posture visible to runtime recovery decisions

### Tunables

- exhaustion thresholds
- cooldown-versus-reroute posture
- provider-switch permission
- worker pause thresholds triggered by quota pressure

### Consumed By

- `runtime_recovery`
- `poll_gate` and `recovery_gate`

## `decomposition`

### Built-In Defaults

- treat autonomous decomposition as out of scope for `v1`
- allow the runtime to detect decomposition pressure without creating new tasks
- require escalation or upstream scope change when single-task thresholds are
  exceeded

### Tunables

- breadth thresholds
- file-count or acceptance-count thresholds
- context-size thresholds
- unrelated-deliverable detection sensitivity

### Consumed By

- `task_contract_review`
- `scope_and_dependency_resolution`
- `execution_lane_routing`

## `escalation`

### Built-In Defaults

- human escalation remains the narrowest lane
- escalate after bounded recovery is exhausted
- escalate immediately when product intent, governance semantics, or provenance
  meaning cannot be preserved safely

### Tunables

- escalation thresholds by failure class
- required evidence bundle for escalation creation
- whether cooldown must precede escalation for selected runtime failures
- resume policy after human resolution

### Consumed By

- all sub-workflows when `human_required` becomes true
- `recovering -> escalated` and `escalated -> ...` transitions

## `subworkflows`

### Built-In Defaults

- every reusable sub-workflow shares the same result-shape contract
- deterministic rules run before policy ranking and before any bounded
  exception-agent lane
- exception agents are allowed only for explicitly enumerated cases and only one
  bounded attempt per triggering condition

### Tunables

- sub-workflow-specific thresholds
- confidence thresholds for deterministic selection
- allowed outcome subsets by repo or environment
- agent-action envelope details for selected runtime-recovery cases

### Consumed By

- `task_contract_review`
- `scope_and_dependency_resolution`
- `execution_lane_routing`
- `provenance_review`
- `runtime_recovery`

## Defaults Versus Tunables Summary

| Policy group | Built-in defaults define | Tunables may adjust |
| --- | --- | --- |
| `retry` | safe retry posture by failure class | counts, constraint tightening, reset rules |
| `cooldown` | when deterministic wait is preferred | durations, backoff, scope of cooldown |
| `workers` | minimum safe execution tiers | tier catalog, concurrency, preference rules |
| `models` | preferred and fallback ranking behavior | exact order, downgrade thresholds |
| `harnesses` | preferred and fallback harness behavior | exact order, restrictions, capability mapping |
| `quota` | exhaustion response posture | switch, pause, and escalation thresholds |
| `decomposition` | no autonomous decomposition in `v1` | threshold sensitivity and detection parameters |
| `escalation` | narrow human lane | evidence requirements and thresholds |
| `subworkflows` | shared decision order and bounded agent posture | per-workflow thresholds and action envelopes |

## Policy-To-Control-Model Mapping

| Policy group | Primary sub-workflow consumers | Primary state-machine consumers |
| --- | --- | --- |
| `retry` | `runtime_recovery` | `executing`, `recovering` |
| `cooldown` | `runtime_recovery` | `polling`, `recovering`, `idle` |
| `workers` | `execution_lane_routing` | `polling`, `claim_won`, `recovering` |
| `models` | `execution_lane_routing`, `runtime_recovery` | `claim_won`, `recovering` |
| `harnesses` | `execution_lane_routing`, `runtime_recovery` | `claim_won`, `recovering` |
| `quota` | `runtime_recovery` | `poll_gate`, `recovery_gate` |
| `decomposition` | `task_contract_review`, `scope_and_dependency_resolution`, `execution_lane_routing` | `claim_gate`, `execute_gate` |
| `escalation` | all sub-workflows | `recovering`, `escalated` |
| `subworkflows` | all sub-workflows | any gate that routes `policy_evaluable` outcomes |

## Out Of Scope

This document does not define:

- the final on-disk policy file format
- the final CLI that loads or inspects policy
- exact implementation structs or parsing rules
- the complete `v1` implementation slice

## Cross-Document Contract

- `orchestrate-policy-subworkflow-specification.md` defines the workflows that
  consume this policy surface
- `orchestrate-worker-runtime-state-machine.md` defines where policy influences
  transitions and gates
- `orchestrate-audit-model.md` defines how policy references are recorded in
  audit events
