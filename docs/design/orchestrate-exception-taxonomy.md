# Orchestrate Exception Taxonomy

## Goal

Keep the expensive exception lane narrow. Not every unusual condition is an
 "exception" in the sense that it should wake up an agent or a human.

## Tier 0: Normal Control Flow

These are normal queue/runtime outcomes and should remain deterministic.

- `claim_lost`
- `no_ready_work`
- `idle_timeout`
- `stale_claim_observed`
- `task_already_complete`
- `task_already_escalated`

Expected behavior:

- requeue
- backoff
- continue loop
- exit worker cleanly

No subagent should be involved.

## Tier 1: Deterministic Recoverables

These are real failures, but policy can often resolve them without an
 exception agent.

- retryable verification failure
- timeout within retry budget
- deterministic harness fallback
- deterministic provider cooldown
- deterministic repo repair path
- stale source refresh with no semantic change

Expected behavior:

- retry
- tighten constraints
- switch to pre-approved fallback
- cooldown and requeue
- block if deterministic prerequisites are missing

## Tier 2: Agent-Worthy Exceptions

Use the exception agent only when there are multiple plausible recovery actions
 and deterministic policy cannot choose safely.

### 1. Execution strategy exceptions

- repeated verification failure after deterministic retries
- repeated timeout after deterministic retries
- model appears underpowered for the task class
- harness-specific failure pattern suggests switching harness/provider

### 2. Task quality exceptions

- task is underspecified despite passing earlier gates
- acceptance criteria are contradictory or unusable in practice
- scope is semantically mismatched to the requested outcome

### 3. DAG/governance exceptions

- overlap resolution requires semantic judgment
- dependency/blocking semantics appear wrong or incomplete
- governance state is technically valid but operationally suspicious

### 4. Provenance exceptions

- citation supports the general area but not clearly the actual task
- refreshed source meaning changes the intended implementation
- conflicting decisions materially alter what should be built

### 5. Environment ambiguity exceptions

- failure could plausibly be task-related, harness-related, or repo-related
- repeated runtime failures leave multiple safe recovery options open

## Exception Agent Rules

The exception agent should only run when:

1. the condition is `policy_evaluable`
2. more than one policy-permitted action remains plausible
3. the deterministic runtime cannot choose with high confidence

The exception agent should not be used for:

- polling
- claim contention
- idle waiting
- routine backoff
- fixed retry schedules
- known deterministic preflight failures

## Audit Requirements

When an exception agent is invoked, the audit record should include:

- exception class
- evidence digest
- allowed actions
- chosen action
- rationale
- attempt count
- whether human escalation was considered
- whether human escalation was triggered

## Human Escalation

Human escalation should be the narrowest lane:

- unresolved intent conflict
- provenance dispute with material impact
- governance ambiguity with meaningful downstream consequence
- exhausted recovery budget

This is intentionally much smaller than the total set of failures.
