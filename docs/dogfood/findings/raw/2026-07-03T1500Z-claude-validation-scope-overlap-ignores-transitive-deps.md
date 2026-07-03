---
date: 2026-07-03
agent: claude
area: validation
task: Planning HOOKBIND story (ADR-0007 path-based issue binding resolution)
tags: [validate, scope-overlap, blocked_by, transitivity]
---

# arm validate scope-overlap warnings ignore transitive blocked_by ordering

## User Goal

Release a 5-task story (HOOKBIND-T1..T5) where overlapping scopes were already
serialized by a linear dependency chain: T2→T1, T3→T2, T4→T3, T5→T4.

## Observed

`arm validate` emitted scope-overlap WARNINGs for T5/T1, T3/T1, and T4/T1 even
though every overlapping pair was strictly ordered through the transitive
blocked_by chain. The warnings only cleared after adding redundant direct
edges (`arm link --source HOOKBIND-T3 --dep HOOKBIND-T1`, likewise T4→T1,
T5→T1).

## Impact

Extra planner round-trip and three edges that carry no information the DAG
did not already encode. Redundant edges also add noise for anyone reading the
graph, and the workaround pattern will scale poorly on longer chains
(a k-task linear chain over one shared file needs O(k²) explicit edges).

## Evidence

```
WARNING: scope overlap: HOOKBIND-T5 and HOOKBIND-T1 both modify docs/harness-hook.md
WARNING: scope overlap: HOOKBIND-T3 and HOOKBIND-T1 both modify cmd/armature/harness_hook.go, cmd/armature/harness_hook_test.go
WARNING: scope overlap: HOOKBIND-T1 and HOOKBIND-T4 both modify cmd/armature/merged.go, cmd/armature/merged_test.go
```

All three cleared after direct `arm link` edges; `arm validate --ci` then
exited 0 with only phantom-scope INFOs (files created by an upstream task).

## Suggested Follow-Up

Scope-overlap check should compute reachability over blocked_by (transitive
closure) rather than direct edges only, so any strictly-ordered overlapping
pair is considered resolved.
