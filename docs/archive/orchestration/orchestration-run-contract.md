# Orchestration Run Contract

## Purpose

This document captures the agreed refactor direction for deepening Armature's orchestration run module. It is the source document for the task graph that moves orchestration wiring from `cmd/armature` into the functional core under `internal/`.

## Problem

`arm orchestrate` and `arm worker run` both need the same task execution truth: materialized issue state, model resolution, auth resolution, harness configuration, active scope conflicts, rendered task context, claim ownership, retry behavior, and lifecycle reporting.

Today much of that truth is assembled in command code before crossing into `internal/orchestrate`. That makes `cmd/armature` thicker than an imperative shell and leaves the orchestration interface shallow: callers must know nearly as much as the implementation.

## Agreed Direction

`internal/` should be the functional core. `cmd/` should be a thin imperative shell that parses flags, resolves the invoking worker identity, calls a strict internal contract, and formats the result.

The orchestration run module should load and materialize the issue itself. Callers should not pass prebuilt issue state, active scope maps, harness adapters, git adapters, op-log adapters, or rendered context callbacks.

The module should expose a strict contractual interface for runs and a separate preflight interface for non-mutating inspection.

## Domain Terms

These terms are recorded in `CONTEXT.md` and should be used consistently:

- **Orchestration run**: A single-task execution lifecycle that loads task truth from Armature state, claims the task when needed, dispatches a harness, verifies the result, and records completion or escalation ops.
- **Orchestration preflight**: A non-mutating inspection lifecycle that resolves whether a task can be orchestrated, including harness selection, auth source, endpoint disclosure, and payload class disclosure.
- **Dry orchestration run**: An orchestration run that follows the same preparation path as a mutating run and stops only before durable ops, harness execution, git mutation, or commits.
- **Task lifecycle outcome**: The task status decision produced by an orchestration run after dispatch and verification, independent from the internal orchestration phase.

## Contract Requirements

### Strict run interface

The run interface should accept a small caller-facing request:

```go
type RunRequest struct {
    TaskID string
    WorkerID string
    Harness string
    ModelOverride string
    RetryBudget int
    Timeout time.Duration
    DryRun bool
    Progress ProgressSink
}
```

The run interface should return a caller-facing result rather than exposing `OrchestrateState`:

```go
type RunResult struct {
    TaskID string
    Phase string
    Run int

    DryRun bool
    WouldClaim bool
    ClaimHeld bool
    ClaimOwner string

    WouldDispatch bool
    BlockedReason string
    ScopeConflicts []ScopeConflict

    Harness string
    Model string
    AuthSource string
    LifecycleOutcome LifecycleOutcome
    CompletionMessage string
    Diagnostics Diagnostics
}
```

The exact field names may change during implementation, but the interface must remain caller-oriented and must not require command code or worker runtime code to understand op replay internals.

### Separate preflight interface

Auth checks and network/payload disclosure are orchestration preflight concerns, not orchestration run concerns. `--auth-check` and `--show-network-plan` should call preflight behavior rather than adding inspection branches inside run execution.

Preflight should not write ops, execute harnesses, mutate git state, or commit changes.

### Dry run path parity

Dry orchestration run exists to build user trust in what a mutating run would do. It must follow the same preparation path as a mutating orchestration run and stop only at explicit side-effect gates:

- durable op writes
- harness execution
- git mutation
- commits

Dry run should report what would happen, including whether the run would claim the task, whether dispatch would occur, and whether scope conflicts would block dispatch.

### Claim and scope gate

An orchestration run should claim the task if it is not already claimed by the invoking worker. After claim ownership is verified, active scope conflicts should be evaluated from the same post-claim-intent view.

If another active worker owns the task or a scope conflict blocks execution, the run should return a blocked result with a concrete reason instead of entering dispatch.

### Lifecycle outcome

The internal orchestration phase and the task lifecycle outcome are separate. A complete orchestration phase must include an explicit task lifecycle outcome.

No committed changes should default to a blocked lifecycle outcome unless verification can prove the task was already satisfied. The run result should never report successful completion while leaving the task lifecycle ambiguous.

## Implementation Shape

The intended module shape is:

```text
internal/orchestrate/
  run.go         strict run request/result interface and repo-backed runner
  preflight.go   strict preflight request/result interface
  engine.go      internal state machine and op replay
```

Command code should translate flags to request values and format results. It should not assemble materialized issue truth, active scope maps, harness adapters, git adapters, op-log adapters, or task context callbacks.

`arm worker run` should call the same run contract used by `arm orchestrate`, so retry handling and runtime loop behavior reuse the same orchestration truth.

## Acceptance Invariants

- `cmd/armature/orchestrate.go` does not assemble the full orchestration implementation before calling `internal/orchestrate`.
- `cmd/armature/worker_run.go` does not duplicate harness config, active scope, or task context wiring.
- Dry orchestration run and mutating orchestration run share the same preparation path.
- Preflight checks are non-mutating and separate from run execution.
- Complete orchestration phases include explicit task lifecycle outcomes.
