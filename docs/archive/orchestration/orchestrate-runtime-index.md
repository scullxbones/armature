# Orchestrate Runtime Brainstorm Index

This document indexes the focused design notes produced from the `arm orchestrate`
runtime brainstorming. The intent is to preserve decisions while keeping future
conversation context light.

## Documents

- `orchestrate-runtime-direction.md`
  - Core direction for queue draining, deterministic runtime ownership, and the
    role of exception agents.
- `orchestrate-runtime-decisions.md`
  - Compact decision ledger for locked decisions, derived decisions,
    assumptions, open questions, deferred questions, and source notes.
- `orchestrate-readiness-and-executability.md`
  - Separation of Definition of Ready (DoR) and Definition of Executability
    (DoE), including tagged checks.
- `orchestrate-exception-taxonomy.md`
  - Tiered failure taxonomy distinguishing normal control flow, deterministic
    recoverables, and agent-worthy exceptions.
- `orchestrate-policy-subworkflows.md`
  - Reusable policy-evaluable sub-workflows that resolve ambiguous conditions.
- `orchestrate-policy-subworkflow-specification.md`
  - Full Phase 2 specification for the five reusable sub-workflows, their
    shared invocation model, result shape, and bounded exception-agent entry
    rules.
- `orchestrate-worker-runtime-state-machine.md`
  - Deterministic state machine above the existing single-task orchestrator,
    including transitions, gates, and runtime event stubs.
- `orchestrate-runtime-policy-model.md`
  - Runtime policy surface separating built-in defaults from tunables across
    retry, cooldown, workers, models, harnesses, quota, decomposition,
    escalation, and sub-workflows.
- `orchestrate-audit-model.md`
  - Audit event model for policy evaluations, bounded recovery, retries,
    reroutes, cooldowns, and human escalations.
- `orchestrate-runtime-gap-analysis.md`
  - Phase 3 reconciliation of the proposed runtime model against current
    Armature commands, packages, skills, ops, policy seams, and audit seams.
- `orchestrate-runtime-oss-review.md`
  - Phase 3 review of Go workflow and state-machine libraries, with a
    recommendation to build the `v1` runtime directly while selectively
    borrowing patterns.
- `orchestrate-runtime-v1-scope.md`
  - Phase 4 v1 scope note selecting the thinnest valuable runtime slice,
    included and deferred sub-workflows, CLI posture, audit posture, policy
    posture, exception-agent posture, and acceptance criteria.

## Current Position

The current direction is:

1. Queue draining should not depend on the user manually running a loop.
2. Queue draining should not spend LLM tokens on routine polling, claiming, or
   idle behavior.
3. A deterministic embedded worker runtime should own normal execution.
4. Exception agents are allowed, but only for bounded recovery within policy and
   with audit traceability.
5. Phase 3 found that the runtime can wrap and reuse existing `ready`, `claim`,
   and single-task `orchestrate` surfaces, but needs explicit runtime gates,
   policy-result types, cooldown/pause state, and audit records before
   implementation.
6. The Go OSS review recommends building the `v1` runtime directly while
   selectively borrowing state-machine, replay, retry, and activity-boundary
   patterns instead of integrating a distributed workflow engine.
7. Phase 4 selects `arm worker run` as the v1 runtime surface, keeps
   `arm orchestrate` as the single-task execution engine, and defers bounded
   exception-agent execution while preserving its future hooks.
8. Future implementation work should follow
   `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md`.

## Open Follow-Ups

- Implement `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md`.
- Define the final on-disk policy and audit serialization formats during the v1
  implementation slice.
- Decide whether a later phase should activate bounded exception-agent
  execution on top of the deterministic hooks chosen for v1.
