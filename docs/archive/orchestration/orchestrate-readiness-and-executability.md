# Orchestrate Readiness And Executability

## Summary

The brainstorming identified a useful separation between:

- **Definition of Ready (DoR)**: is the task well-formed and governable enough
  to enter the executable pool?
- **Definition of Executability (DoE)**: can this worker class execute the task
  right now in the current environment?

This avoids overloading "readiness" with transient runtime conditions such as
 provider quota exhaustion or harness outages.

## Outcome Classes

The internal workflow model should prefer these outcomes over a generic
 "warning" bucket:

- `pass`
- `informational`
- `policy_evaluable`
- `blocked`

### Meaning

- `pass`: no action needed
- `informational`: audit/ranking metadata only
- `policy_evaluable`: must enter a deterministic or bounded sub-workflow
- `blocked`: task cannot proceed until fixed

## Enforceable Check Matrix

This matrix turns the brainstormed categories into checks that can be wired into
existing commands and the future worker runtime. `policy_evaluable` checks
should first route to deterministic policy or the named sub-workflow. They only
become exception-agent candidates when deterministic policy cannot choose safely
among policy-permitted actions. A check returns `pass` when its signal is not
present.

### Definition of Ready Checks

DoR focuses on task quality, governance, dependencies, and provenance. These
checks decide whether a task is well formed enough to enter the executable pool.

| ID | Check | Exact signal | Owner | Runs | Outcome | Policy route |
| --- | --- | --- | --- | --- | --- | --- |
| `dor_status_draft` | Draft status | Issue status or provenance confidence indicates `draft` or unconfirmed imported/inferred work. | `ready`, `validate` | Ready queue materialization and validation | `blocked` | None |
| `dor_missing_parent` | Missing parent | Issue references a parent that cannot be resolved. | `validate`, materialization | Materialization and validation | `blocked` | None |
| `dor_missing_dependency` | Missing dependency | Issue has a `blocked_by` or dependency reference that cannot be resolved. | `validate`, materialization | Materialization and validation | `blocked` | None |
| `dor_dependency_cycle` | Dependency cycle | Dependency graph contains a cycle involving the issue. | `validate`, materialization | Materialization and validation | `blocked` | None |
| `dor_missing_dod` | Missing DoD | Task has no `dod` value or the value is empty after trimming. | `validate`, `ready` | Validation and ready filtering | `blocked` | None |
| `dor_missing_acceptance` | Missing acceptance | Task has no acceptance criteria or the criteria list is empty. | `validate`, `ready` | Validation and ready filtering | `blocked` | None |
| `dor_missing_scope` | Missing scope | Task has no scope entry or every scope entry is empty/invalid. | `validate`, `ready` | Validation and ready filtering | `blocked` | None |
| `dor_missing_citation` | Missing citation | Task has neither a source link nor an accepted citation. | `validate`, release gate | Validation and release/readiness review | `blocked` | None |
| `dor_unresolved_source` | Unresolved source | Source link or accepted citation references a source ID that cannot be resolved. | `validate`, traceability | Validation and traceability materialization | `blocked` | None |
| `dor_overlap_without_rationale` | Accepted overlap lacks rationale | Scope overlap is accepted or forced but has no required rationale. | `claim`, `validate` | Claim preflight and validation | `blocked` | None |
| `dor_vague_dod` | Vague DoD | DoD is present but too broad, circular, or not externally verifiable by configured policy. | Planner/release gate | Pre-ready review and release gate | `policy_evaluable` | `task_contract_review` |
| `dor_weak_acceptance` | Weak acceptance | Acceptance criteria are present but do not define observable verification or are too weak for code execution. | Planner/release gate | Pre-ready review and release gate | `policy_evaluable` | `task_contract_review` |
| `dor_scope_overbroad` | Overbroad scope | Scope globs touch unrelated areas or exceed configured breadth thresholds. | Planner/release gate, `validate` | Pre-ready review and validation | `policy_evaluable` | `scope_and_dependency_resolution` |
| `dor_scope_acceptance_mismatch` | Scope/acceptance mismatch | Acceptance asks for behavior outside the declared scope, or scope omits files likely required by acceptance. | Planner/release gate | Pre-ready review | `policy_evaluable` | `task_contract_review` |
| `dor_too_large` | Task may be too large | Estimated files, acceptance items, or context size exceed configured single-cycle thresholds. | Planner/release gate | Pre-ready review | `policy_evaluable` | `execution_lane_routing` |
| `dor_unrelated_deliverables` | Multiple unrelated deliverables | Title, DoD, acceptance, or scope describe more than one unrelated implementation target. | Planner/release gate | Pre-ready review | `policy_evaluable` | `execution_lane_routing` |
| `dor_sibling_overlap` | Sibling overlap needs decision | Active sibling tasks overlap in scope without deterministic serialization or accepted rationale. | `claim`, planner/release gate | Claim preflight and pre-ready review | `policy_evaluable` | `scope_and_dependency_resolution` |
| `dor_conflicting_decisions` | Conflicting decisions affect task | Active decision records disagree about scope, behavior, or implementation constraints for the task. | Planner/release gate | Pre-ready review and context rendering | `policy_evaluable` | `provenance_review` |
| `dor_weak_citation_alignment` | Weak citation alignment | Citation exists but policy cannot verify that it supports the actual task intent. | Traceability, planner/release gate | Traceability review and pre-ready review | `policy_evaluable` | `provenance_review` |
| `dor_worker_tier_pressure` | Worker/model tier pressure | Task quality, risk, or complexity suggests the default worker/model tier may be insufficient. | Planner/release gate | Pre-ready review | `policy_evaluable` | `execution_lane_routing` |
| `dor_new_files_inferable` | New files inferable | Scope references new files without explicit new-file marking, but the target is inferable. | `validate` | Validation | `informational` | None |
| `dor_priority_missing` | Priority missing | Task lacks priority, but default ordering policy can still rank it. | `ready`, planner/release gate | Ready queue ranking and pre-ready review | `informational` | None |
| `dor_ttl_suboptimal` | TTL expectation suboptimal | TTL or scheduling expectation is missing or outside preferred range, but not execution-blocking. | Planner/release gate | Pre-ready review | `informational` | None |
| `dor_nonpreferred_fallback` | Nonpreferred fallback exists | A fallback execution path exists but policy ranks another path higher. | Planner/release gate | Pre-ready review | `informational` | None |

### Definition of Executability Checks

DoE focuses on runtime and worker-class conditions. These checks decide whether
the selected worker can execute the task right now in the current environment.

| ID | Check | Exact signal | Owner | Runs | Outcome | Policy route |
| --- | --- | --- | --- | --- | --- | --- |
| `doe_blockers_unmerged` | Required blockers unmerged | Any `blocked_by` issue is not in a merged/completed state accepted by runtime policy. | `ready`, worker runtime | Ready filtering and pre-claim/pre-execute | `blocked` | None |
| `doe_repo_health_failed` | Repo structural health failed | `validate` or `doctor` reports structural failures that policy marks execution-blocking. | `validate`, `doctor`, worker runtime | Worker startup and pre-execute | `blocked` | None |
| `doe_missing_verification` | Verification commands missing | Task or repo policy requires verification, but no verification command or deterministic substitute is configured. | Worker runtime, transition gate | Pre-execute and pre-complete | `blocked` | None |
| `doe_missing_harness` | Harness adapter missing | Required harness adapter is unavailable and no deterministic fallback harness is configured. | Worker runtime | Worker startup and pre-execute | `blocked` | None |
| `doe_missing_sandbox` | Sandbox prerequisite missing | Required sandbox, tool, permission, or filesystem prerequisite is unavailable and no deterministic fallback is configured. | Worker runtime, `doctor` | Worker startup and pre-execute | `blocked` | None |
| `doe_model_fallback_choice` | Preferred model unavailable and fallback matters | Preferred model is unavailable and policy has more than one plausible permitted fallback. | Worker runtime | Pre-execute and retry routing | `policy_evaluable` | `execution_lane_routing` |
| `doe_harness_credits_exhausted` | Harness credits exhausted | Preferred harness cannot run because its quota or credits are exhausted. | Worker runtime | Pre-execute and retry routing | `policy_evaluable` | `runtime_recovery` |
| `doe_provider_unavailable` | Provider outage or auth failure | Provider API, auth, or network checks fail for the preferred execution path. | Worker runtime | Pre-execute and retry routing | `policy_evaluable` | `runtime_recovery` |
| `doe_verification_persistent_failure` | Verification failures persist | Verification still fails after deterministic retry budget or known deterministic repair paths are exhausted. | Worker runtime, transition gate | Retry handling and pre-complete | `policy_evaluable` | `runtime_recovery` |
| `doe_repeated_timeout` | Repeated timeouts | Execution or verification times out after deterministic timeout/retry budget is exhausted. | Worker runtime | Retry handling | `policy_evaluable` | `runtime_recovery` |
| `doe_provenance_meaning_changed` | Refreshed provenance changes meaning | Source refresh succeeds but changes task intent, scope, or acceptance meaning. | Worker runtime, provenance refresh | Pre-execute and execution-time refresh | `policy_evaluable` | `provenance_review` |
| `doe_execution_quality_mismatch` | Task quality mismatch discovered during execution | Worker discovers contradictory acceptance, unusable scope, or underspecification that earlier DoR checks missed. | Single-task orchestrator, worker runtime | During execution | `policy_evaluable` | `task_contract_review` |
| `doe_safe_model_fallback` | Safe model fallback configured | Preferred model is unavailable, but policy identifies exactly one safe fallback. | Worker runtime | Pre-execute and retry routing | `informational` | None |
| `doe_safe_harness_fallback` | Safe harness fallback configured | Preferred harness is unavailable, but policy identifies exactly one safe fallback. | Worker runtime | Pre-execute and retry routing | `informational` | None |

## Ownership Boundaries

- `ready` owns deterministic queue eligibility and should exclude tasks with
  `blocked` DoR/DoE conditions.
- `claim` owns claim-time contention and scope-overlap checks that can only be
  evaluated safely at the moment of claim.
- `validate`, `doctor`, traceability materialization, and planner/release gates
  own repo, graph, source, and task-contract checks before work enters the pool.
- The worker runtime owns DoE checks tied to current worker capacity, provider
  state, harness state, retries, timeouts, and execution-time provenance refresh.
- The existing single-task orchestrator can surface execution-time DoE signals,
  but the worker runtime owns routing those signals through deterministic policy
  or a bounded sub-workflow.

## Practical Intent

DoR should remove tasks that never should have reached workers.

DoE should handle real-time execution conditions without pretending they are
 properties of the task itself.

## Design Principle

Ruthlessly minimize human acknowledgment. If a deterministic rule or a policy
 can make the choice with high confidence, route there before any human review.
