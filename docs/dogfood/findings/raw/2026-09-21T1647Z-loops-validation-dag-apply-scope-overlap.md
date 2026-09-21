---
date: 2026-09-21
agent: loops
area: validation
task: MATENC-S1 dag apply dry-run
tags: [arm, dag-apply, scope-overlap, planner]
---

# dag apply dry-run rejects multi-task plans that share engine.go

## User Goal

Decompose MK reshape into 7 verifiable tasks matching the agreed PR sequence.

## Observed

`arm dag apply --dry-run` failed with Graph Finding scope overlap: several tasks listed `internal/materialize/engine.go`. Planner had to re-chain `blocked_by` so overlapping scopes never coexist as concurrently open work.

## Impact

Extra planning iteration; the "7 parallelizable units" mental model is wrong for files that concentrate laws (engine.go). Decomposition must encode sequencing as deps, not only as human PR order.

## Evidence

```
cannot introduce Graph Finding on MATENC-S1-T4, MATENC-S1-T1: scope overlap: ... both modify internal/materialize/engine.go
```

## Suggested Follow-Up

Planner skill: before dry-run, compute pairwise scope file intersection and auto-suggest blocked_by edges. Document that PR sequence ≠ ready-wave parallelism when scopes collide.
