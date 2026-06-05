# Orchestrate Policy Sub-Workflows

## Summary

Many `policy_evaluable` conditions should not each create a bespoke workflow.
Instead, Armature should reuse a small set of decision-oriented sub-workflows.

The recommended target set is five.

## 1. `task_contract_review`

Purpose:

- decide whether a task specification is strong enough to enter or remain in
  execution

Feeds from:

- vague DoD detection
- weak acceptance criteria
- acceptance not clearly suitable for code execution
- scope/acceptance mismatch
- task-quality mismatch discovered during execution

Typical outcomes:

- `allow`
- `allow_with_constraints`
- `require_rewrite`
- `require_decomposition`
- `route_to_exception_agent`
- `block`

## 2. `scope_and_dependency_resolution`

Purpose:

- decide whether the task's scope envelope is safe and parallelizable

Feeds from:

- overbroad scope detection
- sibling overlap detection
- overlap rationale requirements
- dependency plausibility concerns
- merge-first dependency modeling concerns

Typical outcomes:

- `allow`
- `allow_with_rationale`
- `serialize_with_dependency`
- `require_scope_change`
- `require_dependency_fix`
- `block`

## 3. `execution_lane_routing`

Purpose:

- route work to the right worker/model tier and detect decomposition pressure

Feeds from:

- task may be too large
- task may include multiple unrelated deliverables
- estimated runtime cost unusually high
- worker/model tier fit
- preferred model availability

Typical outcomes:

- `route_to_default_tier`
- `route_to_stronger_tier`
- `route_to_cheaper_tier`
- `require_decomposition`
- `defer_until_capacity`
- `block_release`

## 4. `provenance_review`

Purpose:

- decide whether the evidence basis for the task remains trustworthy

Feeds from:

- conflicting decisions affecting scope
- weak citation-task semantic alignment
- stale source refreshes that change meaning
- provenance changes discovered during execution

Typical outcomes:

- `allow`
- `allow_with_latest_effective_decision`
- `require_provenance_refresh`
- `pause_for_review`
- `route_to_exception_agent`
- `block`

## 5. `runtime_recovery`

Purpose:

- recover from runtime, provider, harness, and repeated execution failures

Feeds from:

- verification failure
- timeout exhaustion
- quota exhaustion
- provider outage
- auth failure
- harness fallback decisions
- ambiguous runtime root-cause scenarios

Typical outcomes:

- `retry_same`
- `retry_with_stronger_constraints`
- `retry_with_model`
- `retry_with_harness`
- `increase_timeout`
- `cooldown_then_requeue`
- `switch_provider`
- `pause_worker`
- `escalate_human`

## Ownership Boundaries

### Planner / release gate should invoke

- `task_contract_review`
- `scope_and_dependency_resolution`
- `execution_lane_routing`
- `provenance_review`

### Worker runtime should invoke

- `execution_lane_routing`
- `runtime_recovery`
- `provenance_review` when refreshed evidence changes meaning

## Shared Result Shape

Each sub-workflow should return a common result structure, for example:

- `outcome`
- `reason_code`
- `evidence`
- `constraints`
- `next_action`
- `agent_allowed`
- `human_required`

Using a shared result shape will keep runtime integration simpler and reduce the
 chance that each sub-workflow invents its own semantics.

## Pending Follow-Up

The next deeper design pass should define, for each sub-workflow:

- inputs
- deterministic rules
- policy knobs
- exception-agent entry conditions
- output schema
