# Orchestrate Runtime Brainstorm Index

This document indexes the focused design notes produced from the `arm orchestrate`
runtime brainstorming. The intent is to preserve decisions while keeping future
conversation context light.

## Documents

- `orchestrate-runtime-direction.md`
  - Core direction for queue draining, deterministic runtime ownership, and the
    role of exception agents.
- `orchestrate-readiness-and-executability.md`
  - Separation of Definition of Ready (DoR) and Definition of Executability
    (DoE), including tagged checks.
- `orchestrate-exception-taxonomy.md`
  - Tiered failure taxonomy distinguishing normal control flow, deterministic
    recoverables, and agent-worthy exceptions.
- `orchestrate-policy-subworkflows.md`
  - Reusable policy-evaluable sub-workflows that resolve ambiguous conditions.

## Current Position

The current direction is:

1. Queue draining should not depend on the user manually running a loop.
2. Queue draining should not spend LLM tokens on routine polling, claiming, or
   idle behavior.
3. A deterministic embedded worker runtime should own normal execution.
4. Exception agents are allowed, but only for bounded recovery within policy and
   with audit traceability.
5. Human escalation remains available, but should be reserved for ambiguous,
   high-impact, or exhausted-recovery cases.

## Open Follow-Ups

- Perform a Go OSS review for embedded workflow libraries once the requirements
  are firm enough to compare against real extension points.
- Decide whether the runtime surface should be a new command such as
  `arm worker run` or an expanded `arm orchestrate --loop` mode.
- Define the audit record schema for policy decisions and agent-selected
  recovery actions.
