---
area: coordination
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
date: 2026-08-14T20:21Z
story: LNGHZN-S9
---

# Sequential code and documentation tasks deadlock the coordinator gate

## What the agent-user was trying to do

Complete and verify `LNGHZN-S9-T1` before promoting it to `merged`, which is
required to unblock the dependent documentation task `LNGHZN-S9-T2`.

## What happened

T1's build, lint, tests, coverage, and mutation phases all passed, but the final
`make check` census gate failed:

```text
FAIL: Command flag '--from' in code but not in census
make: *** [Makefile:124: census-drift-check] Error 1
```

The census is `docs/design/surface-census.md`, and its command-flag ownership is
derived from documented command surfaces. T2 owns `docs/commands.md` and is the
story task that documents `--from`, but T2 is blocked until T1 is `merged`. The
coordinator skill forbids marking T1 merged or dispatching T2 while T1's gate is
red, so the declared serial DAG cannot reach the state that makes its own gate
green.

## How it changed behavior, confidence, or time spent

The coordinator had to stop after an otherwise green code delivery and inspect
whether to violate scope, violate the merge gate, or amend the dependency. None
of those policy decisions is represented in the task contracts. This defeats the
purpose of serial task progression and makes a deterministic gate require
coordinator judgment.

## Evidence

- T1: `cmd/armature/claim.go`, `cmd/armature/claim_test.go` only.
- T2: `docs/commands.md`, `docs/use-cases.md`, and the embedded worker skill.
- `make check` reached 100% mutation efficacy for both internal and cmd packages
  before failing only on the new undocumented `--from` flag.
- `arm show LNGHZN-S9-T2` reports `blocked_by: [LNGHZN-S9-T1]`.

## What would have helped

Plan code plus its census/documentation update in the same task, or allow the
coordinator to execute a gate-coupled dependent task before promoting either task
and then verify the combined serial wave. A planning validation could detect a
code flag added in one task while the immediately dependent documentation task
owns the census surface required by `make check`.
