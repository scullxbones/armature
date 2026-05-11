# Orchestrate Worker Runtime State Machine

## Purpose

This document defines the deterministic worker runtime state machine above the
existing single-task orchestrator. The runtime owns queue control, claim
handling, bounded recovery coordination, pause and stop behavior, and
escalation routing. It does not replace the single-task orchestrator or choose
the final CLI surface.

## Phase 1 Boundary Carried Forward

- the runtime wraps the existing single-task orchestrator
- normal queue draining remains deterministic
- `policy_evaluable` outcomes first route to deterministic policy and the
  reusable sub-workflows
- exception agents are narrow bounded recovery tools, not general orchestrators
- at most one bounded exception-agent recovery attempt is allowed per
  triggering condition before escalation or cooldown
- human escalation remains narrower than general recovery

## State Catalog

| State | Meaning |
| --- | --- |
| `idle` | worker is alive but not currently attempting to poll or execute work |
| `polling` | runtime is materializing ready work and checking claim candidates |
| `claim_pending` | runtime is attempting a claim or waiting for claim outcome confirmation |
| `claim_lost` | runtime observed that another worker won or retained the claim |
| `claim_won` | runtime confirmed that this worker owns the claim and may prepare execution |
| `executing` | runtime is invoking or supervising the existing single-task orchestrator |
| `recovering` | runtime is applying deterministic recovery or a bounded recovery decision |
| `paused` | runtime intentionally stops progressing new work until resume conditions are met |
| `escalated` | runtime has created a bounded human escalation and is waiting for resolution |
| `stopped` | runtime has exited gracefully or after checkpointed stop handling |

## State Invariants

- only `executing` may invoke the existing single-task orchestrator
- `claim_lost` is a real runtime state in Phase 2, even though a later phase
  may reduce it to a transition-only outcome
- `recovering` may run deterministic recovery logic or consume the result of a
  bounded exception-agent decision, but it may not widen authority
- `paused` and `escalated` both suspend new task execution, but `paused` is
  operational and `escalated` is governance-facing
- `stopped` is terminal for the worker process

## Gate Vocabulary

The runtime uses consistent gate labels:

- `poll_gate`
  - determine whether queue polling should continue now
- `claim_gate`
  - validate claim candidate readiness and overlap conditions
- `execute_gate`
  - confirm the claimed task may enter execution on this worker
- `recovery_gate`
  - determine whether deterministic retry, cooldown, reroute, or escalation is
    required
- `resume_gate`
  - determine whether a paused or escalated worker may resume normal flow
- `stop_gate`
  - determine whether graceful stop or checkpointed stop should terminate now

## Transition Table

| From | Trigger | Required checks | Policy route | Audit event | Side effects | Next state | Budget impact | Terminality |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `idle` | worker start or idle timer fires | `poll_gate` | none when deterministic | `worker_poll_started` | initialize poll cycle and worker heartbeat | `polling` | none | non-terminal |
| `idle` | stop requested before next poll | `stop_gate` | none | `worker_stop_requested` | record stop intent | `stopped` | none | terminal |
| `polling` | no ready work | `poll_gate` | optional deterministic cooldown | `worker_no_ready_work` | schedule next poll or idle delay | `idle` | optional idle budget consumption | non-terminal |
| `polling` | claim candidate selected | `claim_gate` | none when deterministic | `worker_claim_attempt_started` | lock candidate context for claim | `claim_pending` | none | non-terminal |
| `polling` | pause requested | `stop_gate` plus pause policy | none | `worker_paused` | persist pause reason | `paused` | none | non-terminal |
| `claim_pending` | claim confirmed for another worker | `claim_gate` | none | `worker_claim_lost` | clear local candidate, record loser outcome | `claim_lost` | none | non-terminal |
| `claim_pending` | claim confirmed for this worker | `claim_gate` | none | `worker_claim_won` | persist claim ownership and execution context | `claim_won` | none | non-terminal |
| `claim_pending` | claim preflight fails with `blocked` | `claim_gate` | none | `worker_claim_blocked` | release candidate and record cause | `idle` | none | non-terminal |
| `claim_pending` | claim preflight returns `policy_evaluable` | `claim_gate` | relevant sub-workflow result | `policy_evaluation_recorded` | store sub-workflow result for claim-time decision | `recovering` | depends on result | non-terminal |
| `claim_lost` | claim loss recorded | `poll_gate` | none | `worker_claim_loss_processed` | apply backoff or immediate repoll | `idle` | optional contention backoff budget | non-terminal |
| `claim_won` | execute preflight passes | `execute_gate` | none | `worker_execute_started` | invoke existing single-task orchestrator | `executing` | none | non-terminal |
| `claim_won` | execute preflight returns deterministic reroute or cooldown | `execute_gate` | deterministic policy only | `policy_evaluation_recorded` | update execution lane or cooldown schedule | `recovering` | retry or cooldown budget consumption | non-terminal |
| `claim_won` | execute preflight returns `policy_evaluable` | `execute_gate` | relevant sub-workflow result | `policy_evaluation_recorded` | capture routing decision for runtime handling | `recovering` | depends on result | non-terminal |
| `executing` | orchestrator completes successfully | `execute_gate` | none | `worker_execution_completed` | mark attempt complete and release claim | `idle` | none | non-terminal |
| `executing` | deterministic runtime outcome such as `task_already_complete` or `task_already_escalated` | `execute_gate` | none | `worker_execution_short_circuited` | release claim and record deterministic outcome | `idle` | none | non-terminal |
| `executing` | execution failure with deterministic retry path | `recovery_gate` | deterministic retry policy | `retry_scheduled` | persist retry parameters | `recovering` | consume retry budget | non-terminal |
| `executing` | execution failure returns `policy_evaluable` | `recovery_gate` | relevant sub-workflow result | `policy_evaluation_recorded` | capture recovery decision inputs and evidence | `recovering` | depends on result | non-terminal |
| `executing` | pause requested or checkpointed stop required | `stop_gate` | none | `worker_pause_checkpointed` | persist checkpoint and execution context | `paused` | none | non-terminal |
| `recovering` | result is retry now | `recovery_gate` | none beyond existing result | `retry_scheduled` | tighten constraints, model, harness, or timeout as directed | `executing` | consume retry budget | non-terminal |
| `recovering` | result is reroute before re-execution | `recovery_gate` | none beyond existing result | `reroute_selected` | update worker or execution lane settings | `claim_won` | optional retry budget consumption | non-terminal |
| `recovering` | result is cooldown then requeue | `recovery_gate` | none beyond existing result | `cooldown_started` | release active execution path and schedule recheck | `idle` | consume cooldown budget | non-terminal |
| `recovering` | result invokes bounded exception agent | `recovery_gate` | bounded exception-agent envelope | `exception_agent_invoked` | run agent inside enforced action envelope | `recovering` | consume bounded exception-agent attempt | non-terminal |
| `recovering` | exception agent selects action | `recovery_gate` | none beyond validated action | `exception_agent_action_selected` | validate and apply permitted recovery action | `executing` or `claim_won` | depends on action | non-terminal |
| `recovering` | result requires pause | `recovery_gate` | none | `worker_paused` | persist pause reason and resume conditions | `paused` | none | non-terminal |
| `recovering` | result requires human escalation | `recovery_gate` | escalation policy | `human_escalation_created` | create escalation artifact and suspend autonomous progress | `escalated` | escalation budget consumption | non-terminal |
| `recovering` | no safe action remains for current attempt | `recovery_gate` | none | `worker_attempt_terminated` | mark attempt terminal and release active lane | `idle` | none or cooldown, depending on cause | terminal for current attempt |
| `paused` | resume condition satisfied | `resume_gate` | optional deterministic policy | `worker_resumed` | restore polling or execution context | `idle` or `claim_won` | none | non-terminal |
| `paused` | stop requested | `stop_gate` | none | `worker_stop_requested` | persist final checkpoint if needed | `stopped` | none | terminal |
| `escalated` | human escalation resolved in favor of resume | `resume_gate` | optional policy reroute | `human_escalation_resolved` | attach resolution rationale and restore context | `idle`, `claim_won`, or `recovering` | may reset retry or cooldown posture per policy | non-terminal |
| `escalated` | human escalation resolved as blocked or cancelled | `resume_gate` | none | `human_escalation_resolved` | release claim and mark issue state as directed | `idle` or `stopped` | none | may be terminal for current attempt |
| `escalated` | stop requested while waiting | `stop_gate` | none | `worker_stop_requested` | preserve escalation linkage and stop state | `stopped` | none | terminal |

## Normal Queue-Control Transitions

These outcomes remain deterministic runtime control flow, not exceptions:

- `claim_lost`
- `no_ready_work`
- `idle_timeout`
- `stale_claim_observed`
- `task_already_complete`
- `task_already_escalated`

The runtime may audit them, but it does not treat them as justification for an
exception-agent lane.

## Claim Win And Claim Loss Handling

### Claim Win

- `claim_pending -> claim_won` records that this worker owns the task
- the runtime must snapshot enough context to support execute preflight,
  retries, and escalation without re-deriving claim evidence
- execution cannot start until execute-gate checks pass

### Claim Loss

- `claim_pending -> claim_lost` is deterministic and expected under contention
- the runtime records the loss, clears the candidate, and returns to idle or
  backoff
- claim loss does not spend exception-agent budget

## Execution Handoff

`claim_won -> executing` is the handoff from queue control to the existing
single-task orchestrator. The runtime remains responsible for:

- recording start and finish audit events
- attaching retry and cooldown policy
- interpreting DoE or execution-time `policy_evaluable` outcomes
- coordinating pause, stop, and escalation behavior

The existing single-task orchestrator remains responsible for task execution,
not queue ownership.

## Deterministic Recovery And Bounded Exception Recovery

The `recovering` state unifies:

- deterministic retries
- deterministic fallback model or harness selection when exactly one safe
  choice exists
- deterministic cooldown and requeue
- bounded exception-agent recovery after deterministic and policy ranking fail

This state does not grant open-ended authority. It only applies the result from
policy or sub-workflow evaluation.

## Pause And Resume Behavior

The runtime may enter `paused` because of:

- explicit operator pause
- checkpointed stop preparation
- policy-directed pause for provenance or recovery review

Resume requires:

- pause reason still validly cleared
- no higher-priority stop request
- resume gate confirming that the preserved context still matches current
  policy and audit state

## Human Escalation Behavior

`recovering -> escalated` is the narrowest governance lane:

- escalation exists because bounded deterministic and exception recovery were
  insufficient
- the runtime must attach evidence, policy references, and prior attempts
- the runtime remains suspended until resolution is materialized

Human escalation is not a generic catch-all for routine execution friction.

## Graceful And Checkpointed Stop Behavior

The runtime may stop from `idle`, `paused`, or `escalated` immediately once the
stop gate confirms the state is safe to terminate.

If stop is requested during `executing`, the runtime should checkpoint first
when policy requires preserving in-flight context. That checkpoint produces
`executing -> paused`, after which `paused -> stopped` completes termination.

## Required Checks By Gate

### `poll_gate`

- worker enabled and not stopped
- cooldown window expired or absent
- worker quota allows another poll

### `claim_gate`

- candidate still appears ready
- claim prerequisites still hold
- claim-time overlap checks pass or route through a sub-workflow

### `execute_gate`

- claim still owned
- execution prerequisites still hold
- DoE signals are classified as `pass`, `informational`, deterministic
  fallback, or `policy_evaluable`

### `recovery_gate`

- retry or cooldown budgets are current
- prior bounded exception-agent attempts for the triggering condition are known
- sub-workflow results are fully captured before transition

### `resume_gate`

- pause or escalation resolution is materialized
- claim and execution context remain valid if resume skips idle

### `stop_gate`

- checkpoint requirement evaluated
- any required audit events emitted before final stop

## Audit Event Stubs By Transition Class

- poll lifecycle
  - `worker_poll_started`
  - `worker_no_ready_work`
- claim lifecycle
  - `worker_claim_attempt_started`
  - `worker_claim_won`
  - `worker_claim_lost`
  - `worker_claim_loss_processed`
  - `worker_claim_blocked`
- execution lifecycle
  - `worker_execute_started`
  - `worker_execution_completed`
  - `worker_execution_short_circuited`
- recovery lifecycle
  - `policy_evaluation_recorded`
  - `retry_scheduled`
  - `reroute_selected`
  - `cooldown_started`
  - `exception_agent_invoked`
  - `exception_agent_action_selected`
  - `worker_attempt_terminated`
- pause, escalation, and stop lifecycle
  - `worker_paused`
  - `worker_pause_checkpointed`
  - `worker_resumed`
  - `human_escalation_created`
  - `human_escalation_resolved`
  - `worker_stop_requested`

These are Phase 2 audit event stubs. Later phases may expand them into
implementation-specific schemas without changing the control model defined here.

## Cross-Document Contract

- `orchestrate-policy-subworkflow-specification.md` defines the shared result
  shape consumed by several transitions here
- `orchestrate-runtime-policy-model.md` defines retry, cooldown, worker, model,
  harness, quota, decomposition, escalation, and sub-workflow settings that
  influence gates and transitions
- `orchestrate-audit-model.md` defines the shared audit fields and event
  catalog for the events referenced here
