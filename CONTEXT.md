# Armature Context

Armature is a git-native work orchestration system for coordinating human and AI workers through append-only ops, materialized task state, and deterministic execution flows.

## Language

**Orchestration run**:
A single-task execution lifecycle that loads task truth from Armature state, claims the task when needed, dispatches a harness, verifies the result, and records completion or escalation ops.
_Avoid_: Orchestrator service, execution wrapper, harness run

**Orchestration preflight**:
A non-mutating inspection lifecycle that resolves whether a task can be orchestrated, including harness selection, auth source, endpoint disclosure, and payload class disclosure.
_Avoid_: Dry run, auth check, network plan

**Dry orchestration run**:
An orchestration run that follows the same preparation path as a mutating run and stops only before durable ops, harness execution, git mutation, or commits.
_Avoid_: Preflight, inspection mode, separate dry-run path

**Task lifecycle outcome**:
The task status decision produced by an orchestration run after dispatch and verification, independent from the internal orchestration phase.
_Avoid_: Phase, completion flag, run status
