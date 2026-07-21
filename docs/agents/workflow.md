# Workflow & Operating Model

Armature coordinates task-driven work; it does not execute or supervise external harnesses.

Normal loop:
```
arm ready → arm claim → arm render-context → (launch worker outside Armature) → arm transition
```

`arm harness-hook` is a harness-native integration surface (guardrails), not a queue runner.

## Invariants

- Ops are append-only JSONL in `.armature/ops/<worker-id>.log`; each worker writes only its own log.
- Materialized state is derived from ops, not source of truth.
- `done` = worker-complete; `merged` = confirmed on main branch.

## Before closing out work

```bash
arm validate --ci
arm doctor
```

This is a task-completion sanity check, separate from the `make check` commit gate — see [quality-gates.md](quality-gates.md).

## Recovery & Claim State Management

When tasks fail to complete, become orphaned, or blockers are left unresolved, the system requires deliberate recovery actions. Coordination failures (stale claims, expired TTLs, skipped redispatches) can leave tasks silently stuck and block downstream work.

See [Recovery State Machine](../design/recovery-state-machine.md) for:
- A complete matrix of issue status × claim liveness combinations
- Correct reconciliation actions for `arm doctor` and the Coordinator for each state
- Recovery procedures for three key failure scenarios:
  - **D1 — Branch Divergence:** Commits reference issues not yet done/merged
  - **D2 — Orphaned Claims:** Tasks with expired TTLs and no worker activity
  - **Redispatch Starvation:** Coordinator fails to re-dispatch when claims expire

The state machine defines which state combinations are valid, which are errors, and what action each should trigger.
