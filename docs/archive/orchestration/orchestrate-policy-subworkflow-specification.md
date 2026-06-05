# Orchestrate Policy Sub-Workflow Specification

## Purpose

This document defines the reusable policy sub-workflows that resolve
`policy_evaluable` conditions for the orchestrate runtime. It turns the Phase 1
boundary into explicit control-model surfaces without choosing the final CLI
shape or implying that the runtime implementation is complete.

These sub-workflows are shared decision surfaces. They let Armature apply a
consistent pattern when deterministic rules identify ambiguity that is still
inside runtime policy.

## Phase 1 Boundary Carried Forward

This specification preserves the existing runtime boundary:

- the worker runtime wraps the existing single-task orchestrator; it does not
  replace it
- final CLI naming remains out of scope
- normal queue draining remains deterministic
- `policy_evaluable` does not automatically mean agent-worthy
- exception agents are narrow bounded recovery tools
- exception agents may run only from explicitly enumerated entry conditions
- at most one bounded exception-agent recovery attempt is allowed per
  triggering condition before escalation or cooldown
- exception-agent safety is enforced by deterministic runtime controls, not
  prompt-only guidance
- exception agents may not broaden scope, rewrite task intent, or decompose
  work autonomously
- human escalation remains the narrowest lane

## Control Principle

Every sub-workflow follows the same decision order:

1. deterministic rules classify the condition and remove impermissible actions
2. runtime policy narrows, ranks, or chooses among the remaining safe actions
3. an exception-agent lane is considered only when more than one
   policy-permitted action remains plausible and deterministic policy cannot
   choose safely
4. if no safe bounded recovery remains, the runtime escalates or cools down

This ordering is mandatory. A sub-workflow never invokes an exception agent
before deterministic and policy reasoning finish.

## Shared Invocation Model

Each sub-workflow accepts a common invocation frame so runtime integration,
audit capture, and future implementation wiring stay uniform.

### Common Inputs

- `triggering_check_id`
  - the DoR or DoE check ID that routed the condition here
- `task_metadata`
  - issue ID, title, scope, acceptance, dependencies, status, and any relevant
    execution annotations
- `current_assessment`
  - the current DoR or DoE result that triggered the workflow, including
    outcome class, summary, and any precomputed evidence
- `policy_profile`
  - the effective built-in defaults plus repo-specific overrides when present
- `evidence_bundle`
  - citations, validation output, runtime signals, retry history, provenance
    refresh details, and any other evidence referenced by deterministic rules
- `worker_context`
  - worker tier, model, harness, quota, runtime state, or execution attempt
    details when the condition is runtime-scoped
- `runtime_context`
  - queue state, cooldown state, claim state, or transition context when the
    decision is made above the single-task orchestrator
- `prior_attempts`
  - prior sub-workflow results, retry counts, cooldown counts, and any prior
    bounded exception-agent attempt for this triggering condition
- `audit_history`
  - relevant prior audit events, including policy evaluations, reroutes,
    retries, cooldowns, and human escalations

### Common Preconditions

- the triggering condition has already been classified as `policy_evaluable`
- the invoking runtime gate has already gathered the minimum evidence required
  by the triggering check
- the invoking gate has already ruled out any direct `blocked` or direct `pass`
  path
- the invoking runtime has identified the current attempt number and remaining
  retry or cooldown budget

## Shared Result Shape

Every sub-workflow returns the same result object so runtime transitions and
audit recording do not need workflow-specific adapters.

| Field | Meaning |
| --- | --- |
| `workflow` | sub-workflow name |
| `outcome` | selected decision outcome |
| `reason_code` | stable explanation code for why the outcome was chosen |
| `confidence` | deterministic or policy confidence level for the chosen outcome |
| `evidence` | evidence references and summaries supporting the decision |
| `constraints` | runtime-enforced constraints attached to the outcome |
| `next_action` | next runtime action, such as retry, reroute, pause, or escalate |
| `policy_decision` | effective policy clause or ranking logic that shaped the outcome |
| `agent_allowed` | whether bounded exception-agent invocation is allowed |
| `agent_action_envelope` | the exact permitted actions if `agent_allowed` is true |
| `human_required` | whether human escalation is required now |
| `audit_events` | audit event stubs the runtime must emit for this result |
| `retry_or_recheck_after` | retry delay, recheck window, or cooldown duration when applicable |
| `terminal_for_current_attempt` | whether this result ends the current execution attempt |

### Shared Result Semantics

- `confidence` is about runtime decision confidence, not model confidence
- `constraints` are runtime-enforced controls, not prompt suggestions
- `next_action` must be implementable by deterministic runtime code
- `agent_action_envelope` may enumerate actions, parameter bounds, required
  evidence, and mandatory validation steps
- `human_required` is narrower than `agent_allowed`; some results allow neither
  agents nor humans because deterministic retry or cooldown is sufficient
- `terminal_for_current_attempt` may be true even when the task is requeued for
  a later attempt

## Outcome Classes Used Across Workflows

The following families keep wording consistent across documents:

- allow-style outcomes
  - the task or runtime may proceed, possibly with constraints
- reroute-style outcomes
  - the work should continue in a different worker tier, model, or harness lane
- retry-style outcomes
  - the current work should retry under deterministic or bounded conditions
- cooldown-style outcomes
  - the runtime should wait before another attempt
- escalation-style outcomes
  - the runtime should raise a bounded human escalation
- block-style outcomes
  - the current attempt cannot safely continue from the current control point

## 1. `task_contract_review`

### Purpose

Determine whether task intent, scope, acceptance, and Definition of Done remain
strong enough for execution or must be constrained, rewritten by a human, or
escalated.

### Feeds From DoR And DoE

- `dor_vague_dod`
- `dor_weak_acceptance`
- `dor_scope_acceptance_mismatch`
- `doe_execution_quality_mismatch`

### Inputs

- common invocation model inputs
- rendered task contract, including title, DoD, acceptance, scope, and issue
  metadata
- relevant planner, validator, or orchestrator findings that explain the
  mismatch or weakness

### Deterministic Rules

- missing or empty required task fields are not handled here; they remain
  `blocked` outside this workflow
- if acceptance and scope are contradictory in a way that changes product
  intent, require escalation instead of guessing
- if the issue can proceed safely with narrower enforcement than originally
  written, prefer `allow_with_constraints`
- if the issue clearly asks for multiple unrelated deliverables, hand off to
  `execution_lane_routing` for decomposition pressure rather than inventing
  decomposition here
- if the current worker discovered an execution-time mismatch that invalidates
  the current attempt, mark the result terminal for the current attempt

### Policy Knobs

- minimum acceptance strength
- maximum tolerated ambiguity in DoD phrasing
- allowed scope inference level
- whether constrained execution is permitted for borderline task contracts
- escalation threshold for repeated task-quality mismatches

### Outputs

- `allow`
- `allow_with_constraints`
- `require_rewrite`
- `require_human_clarification`
- `route_to_exception_agent`
- `block`

### Exception-Agent Entry Conditions

An exception agent may run only when all of the following are true:

- the task contract is internally weak or mismatched but not missing core
  required fields
- deterministic rules and policy agree that more than one bounded
  clarification-preserving recovery action is permissible
- the action envelope can be limited to choices such as selecting among
  policy-approved verification constraints or choosing which already-supported
  acceptance slice to prioritize for the current attempt
- no action would broaden scope, rewrite task intent, or autonomously decompose
  the task
- no prior exception-agent attempt has already been used for this triggering
  condition

### Human Escalation Conditions

- acceptance and scope imply materially different product intent
- the only safe next step would require changing task intent or governance state
- repeated execution-time quality mismatch exhausted the bounded recovery budget
- provenance conflict also affects interpretation and cannot be separated from
  contract ambiguity

## 2. `scope_and_dependency_resolution`

### Purpose

Resolve ambiguous scope overlaps, serialization needs, and dependency concerns
without allowing autonomous task decomposition or broad task rewriting.

### Feeds From DoR And DoE

- `dor_scope_overbroad`
- `dor_sibling_overlap`

### Inputs

- common invocation model inputs
- active sibling issue metadata and overlap rationale when present
- dependency graph snapshot and any claim-time overlap findings
- configured breadth thresholds and serialization policy

### Deterministic Rules

- missing dependencies, missing parents, and dependency cycles remain `blocked`
  outside this workflow
- if an explicit dependency already forces ordering, choose deterministic
  serialization instead of semantic debate
- if scope overlap is accepted and rationale satisfies policy, allow or
  serialize without invoking an agent
- if breadth exceeds configured thresholds in a way that obviously breaks the
  single-task envelope, require scope change rather than letting the runtime
  invent decomposition
- if overlap threatens conflicting writes and no policy-approved serialization
  choice exists, escalate rather than guess

### Policy Knobs

- maximum scope breadth for a single execution attempt
- accepted overlap categories
- required rationale strength for overlap exceptions
- serialization bias versus concurrency bias
- dependency confidence threshold

### Outputs

- `allow`
- `allow_with_rationale`
- `serialize_with_dependency`
- `require_scope_change`
- `require_dependency_fix`
- `block`

### Exception-Agent Entry Conditions

An exception agent may run only when:

- the overlap or dependency scenario is `policy_evaluable`, not structurally
  invalid
- deterministic policy leaves multiple safe bounded actions, such as choosing
  between already-approved serialization orders or selecting among
  policy-approved overlap rationales
- the agent action envelope does not allow creating new work items, changing
  task ownership, or decomposing the task
- the condition has not already consumed its one bounded exception-agent
  attempt

### Human Escalation Conditions

- overlap resolution requires changing governance intent
- dependency semantics appear materially wrong or incomplete
- no policy-approved serialization choice is clearly safer than the others
- the only apparent fix would require decomposition or replanning outside the
  current task boundary

## 3. `execution_lane_routing`

### Purpose

Route work to the safest execution lane when complexity, worker fit, model
availability, or harness choice is ambiguous but still within runtime policy.

### Feeds From DoR And DoE

- `dor_too_large`
- `dor_unrelated_deliverables`
- `dor_worker_tier_pressure`
- `doe_model_fallback_choice`

### Inputs

- common invocation model inputs
- worker tier catalog, model fallback order, and harness fallback order
- execution-cost estimates, scope breadth estimates, and any complexity score
- available worker and model inventory when known

### Deterministic Rules

- if policy defines exactly one safe fallback, do not invoke this workflow; use
  the deterministic fallback directly
- if the issue exceeds single-task thresholds because it contains multiple
  unrelated deliverables, return a decomposition-oriented outcome rather than
  letting the runtime split work autonomously
- if a stronger tier is required by policy floor, select it deterministically
- if capacity is the only blocker and policy requires waiting for a specific
  tier, prefer defer or cooldown over risky downgrade
- routing never changes product intent; it only changes execution lane

### Policy Knobs

- worker tier thresholds
- preferred and fallback model order
- preferred and fallback harness order
- decomposition thresholds
- cost and latency tolerance
- downgrade safety policy

### Outputs

- `route_to_default_tier`
- `route_to_stronger_tier`
- `route_to_cheaper_tier`
- `retry_with_model`
- `retry_with_harness`
- `require_decomposition`
- `defer_until_capacity`
- `block_release`

### Exception-Agent Entry Conditions

An exception agent may run only when:

- more than one policy-permitted lane remains plausible
- deterministic ranking cannot safely break the tie
- each candidate lane already has a bounded validation and retry profile
- the action envelope is limited to choosing among already-approved worker,
  model, or harness lanes
- the triggering condition has not already used its bounded exception-agent
  attempt

### Human Escalation Conditions

- the task appears to require decomposition or replanning beyond runtime
  authority
- the available lanes imply materially different delivery risk with no safe
  deterministic tie-break
- policy gaps prevent the runtime from knowing whether downgrade is acceptable

## 4. `provenance_review`

### Purpose

Determine whether the evidence basis for task execution remains trustworthy when
citations, source refreshes, or decision records conflict or change meaning.

### Feeds From DoR And DoE

- `dor_conflicting_decisions`
- `dor_weak_citation_alignment`
- `doe_provenance_meaning_changed`

### Inputs

- common invocation model inputs
- relevant citations, source links, and decision records
- provenance materialization summaries and refresh diffs
- any execution-time evidence showing semantic drift

### Deterministic Rules

- unresolved citations or missing source IDs remain `blocked` outside this
  workflow
- if refreshed provenance changes wording but not execution meaning, allow with
  the latest effective decision and emit audit rationale
- if conflicting decisions can be ordered by an already-defined precedence rule,
  choose the latest effective governing decision deterministically
- if refreshed provenance changes task intent, mark the current attempt
  terminal and require pause, escalation, or bounded review
- provenance review never fabricates new source meaning

### Policy Knobs

- decision precedence rules
- source staleness thresholds
- citation-alignment confidence threshold
- pause-versus-escalate posture for semantic drift

### Outputs

- `allow`
- `allow_with_latest_effective_decision`
- `require_provenance_refresh`
- `pause_for_review`
- `route_to_exception_agent`
- `block`

### Exception-Agent Entry Conditions

An exception agent may run only when:

- the evidence bundle contains multiple policy-permitted interpretations that
  stay inside the existing task intent
- deterministic precedence and freshness rules cannot safely pick one
- the agent action envelope is limited to selecting among already-supported
  provenance interpretations and recording rationale
- the condition has not already consumed its one bounded exception-agent
  attempt

### Human Escalation Conditions

- provenance conflict materially changes what should be built
- citation alignment remains disputed after deterministic refresh or precedence
  handling
- a governing decision record appears wrong, stale, or contradictory in a way
  the runtime cannot resolve safely

## 5. `runtime_recovery`

### Purpose

Recover from runtime, provider, harness, and repeated execution failures while
keeping deterministic recoverables separate from bounded exception recovery.

### Feeds From DoR And DoE

- `doe_harness_credits_exhausted`
- `doe_provider_unavailable`
- `doe_verification_persistent_failure`
- `doe_repeated_timeout`

### Inputs

- common invocation model inputs
- retry and cooldown budget state
- provider, model, and harness health signals
- execution logs, verification summaries, and timeout history
- any deterministic repair or fallback results already attempted

### Deterministic Rules

- use deterministic retries first when the configured retry budget is not yet
  exhausted
- choose the one safe fallback deterministically when policy supplies exactly
  one permitted fallback
- start cooldown rather than invoke an agent when provider or quota conditions
  match a configured deterministic cooldown profile
- if retries, cooldowns, and deterministic fallbacks are exhausted, only then
  consider bounded exception recovery
- if the failure appears to require scope change, task rewriting, or
  decomposition, escalate rather than widening runtime authority

### Policy Knobs

- retry budgets by failure class
- cooldown durations and backoff rules
- model fallback order
- harness fallback order
- quota exhaustion posture
- escalation thresholds for repeated runtime failure

### Outputs

- `retry_same`
- `retry_with_stronger_constraints`
- `retry_with_model`
- `retry_with_harness`
- `increase_timeout`
- `cooldown_then_requeue`
- `switch_provider`
- `pause_worker`
- `escalate_human`

### Exception-Agent Entry Conditions

An exception agent may run only when:

- deterministic retries and deterministic fallback choices are exhausted
- more than one policy-permitted recovery action remains plausible
- the runtime can enforce a bounded action envelope covering only the candidate
  recovery actions
- the triggering condition has not already consumed its one bounded
  exception-agent attempt
- the runtime can validate the proposed recovery action before execution

### Human Escalation Conditions

- the bounded recovery budget is exhausted
- provider, harness, and repo evidence remain ambiguous after deterministic
  recovery paths complete
- the recovery options would materially change task risk without a safe
  policy-approved tie-break
- the issue appears already escalated or requires a broader governance decision

## Runtime Integration Notes

- planner or release-gate surfaces primarily invoke `task_contract_review`,
  `scope_and_dependency_resolution`, `execution_lane_routing`, and
  `provenance_review`
- the worker runtime primarily invokes `execution_lane_routing`,
  `runtime_recovery`, and `provenance_review`
- the existing single-task orchestrator may surface execution-time signals, but
  the runtime owns sub-workflow routing and final control decisions

## Cross-Document Contract

- this document defines the shared sub-workflow result shape
- `orchestrate-runtime-policy-model.md` defines the defaults and tunables that
  shape these outcomes
- `orchestrate-worker-runtime-state-machine.md` defines when these results
  affect runtime transitions
- `orchestrate-audit-model.md` defines how decisions and outcomes are recorded
