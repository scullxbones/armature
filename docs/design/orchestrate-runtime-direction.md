# Orchestrate Runtime Direction

## Problem Statement

The current orchestrator flow is documented as a pull loop:

1. `arm ready`
2. pick a task
3. `arm orchestrate --issue <id>`
4. repeat

This is mechanically workable, but it creates two problems:

- It expects some outer operator to conduct the loop.
- If that operator is an LLM-guided harness, tokens are spent on polling,
  contention handling, and idle behavior rather than on task execution.

The user should not be responsible for running the queue loop manually.

## Decision

Armature should move toward an embedded deterministic worker runtime that owns
normal queue draining.

The favored shape is:

1. Deterministic runtime computes ready work.
2. Deterministic runtime claims work.
3. Deterministic runtime confirms claim win/loss.
4. Deterministic runtime invokes the existing single-task orchestrator.
5. Deterministic runtime handles idle, retry, backoff, and requeue behavior.

The single-task orchestrator remains important. The new runtime should wrap it,
not replace it.

## Why This Direction

### Token efficiency

Routine control-plane work should not consume model budget:

- queue polling
- claim contention handling
- sleep/backoff
- worker slot management
- repeated "check again" logic

LLM tokens should be spent on implementation and on rare ambiguous recovery
decisions.

### Better support for cheaper models

A more deterministic runtime allows stronger scaffolding around lower-cost
models. The more structure the runtime enforces, the less we need a strong,
expensive model to improvise workflow behavior.

### Architectural fit

Armature's core differentiator remains:

- git-native coordination
- append-only ops
- deterministic materialization
- repo-scoped task semantics
- auditability

The runtime should stay focused on those concerns rather than becoming a general
agent orchestration platform.

## Status Quo Steelman

The strongest argument for the current design is architectural purity:

- `arm ready` is a read model
- `arm claim` is the write boundary
- `arm orchestrate --issue X` is a deterministic single-task engine
- concurrency stays outside the product

This is clean and composable, but it leaves too much operational burden outside
the product and invites wasteful agentic control loops.

## Boundaries

### What the deterministic runtime should own

- queue polling
- claim attempts
- claim loss handling
- worker-slot lifecycle
- deterministic retries and cooldowns
- task-to-worker-tier routing
- invocation of bounded recovery flows

### What it should not own

- open-ended reasoning during normal queue draining
- arbitrary semantic rewrites of tasks
- product-level governance decisions without policy

## Exception Agents

Exception agents are permitted, but only in a bounded lane:

- invoked after deterministic rules or policies cannot choose safely
- allowed to take recovery actions within policy
- must leave audit notes explaining the action chosen
- should escalate to humans after repeated failed attempts or earlier when the
  agent determines ambiguity is too high

This yields a hybrid model:

- deterministic by default
- agentic only for bounded exception recovery

## Future Research Note

Once the requirements are stable, review active Go OSS libraries for embedded
workflow execution. The target is embedded, not distributed, and any dependency
must preserve Armature's git-native and deterministic product identity.

## Phase 1 Boundary Decision Note (2026-05-09)

Phase 1 locks runtime semantics first and intentionally leaves the final CLI
surface undecided. Future CLI work may explore `arm worker run`,
`arm orchestrate --loop`, or another shape, but must preserve the boundary
below.

### V1 Runtime Posture

- `v1` is a value-first bounded hybrid runtime.
- Its job is to continuously identify clearly executable work, claim it
  deterministically, invoke the existing single-task orchestrator, and drain
  ready work without a human-operated loop.
- The runtime wraps the existing single-task orchestrator. It does not replace
  it.
- The runtime may make one bounded recovery attempt for selected
  `policy_evaluable` failures and must escalate unresolved or overly ambiguous
  cases with audit traceability.

### Deterministic Core

The following remain strictly deterministic and outside exception-agent
discretion in `v1`:

- queue polling
- claim attempts and claim win/loss handling
- retry and backoff behavior
- worker routing
- provenance refresh
- Definition of Executability fallback choice when policy already defines a safe
  option

### Exception-Agent Lane

The exception-agent lane is allowed in `v1` only as a narrow recovery
mechanism:

- entry only from explicitly enumerated recovery cases
- at most one bounded recovery attempt per triggering condition before
  escalation or cooldown
- recovery actions stay within a policy-approved action envelope
- the agent may choose among permitted recovery actions and explain its choice
- the agent may not broaden scope, rewrite task intent, or decompose work
  autonomously

Exception-agent safety is enforced by deterministic controls outside the agent,
not by prompt-only behavioral guidance. The real boundary is a runtime-enforced
permission profile, allowed-action contract, deterministic entry conditions,
and deterministic validation of any proposed recovery action. If the bounded
permission profile is insufficient, the runtime escalates rather than widening
permissions.

### Explicit V1 Non-Goals

- final CLI naming
- persistent agent supervisors
- autonomous decomposition or replanning
- broad semantic rewriting of tasks
- distributed workflow infrastructure

### Deferred By Design

This note does not settle the full worker state machine, complete audit schema,
detailed policy configuration surface, or full list of exception-agent recovery
cases.
