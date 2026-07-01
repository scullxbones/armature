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
