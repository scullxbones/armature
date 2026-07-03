# Transitive dependencies don't suppress scope-overlap warnings

- **Writer:** claude (planner, MIGH story load)
- **Area:** validation

## What I was trying to do

Load a 7-task linear-chain story (MIGH-T1 → … → MIGH-T7) via `arm decompose-apply`, where most tasks intentionally touch `cmd/armature/bootstrap.go` and `bootstrap_test.go` in sequence.

## What happened

`arm validate` emitted 8 scope-overlap WARNINGs for pairs that were already strictly ordered through the transitive `blocked_by` chain (e.g. MIGH-T3 vs MIGH-T5, ordered via T4). Only *direct* `blocked_by` edges suppress the warning.

## Impact

Had to add 8 redundant direct edges with `arm link` purely to silence warnings the DAG already made impossible. Adds edge clutter and an extra remediation pass for any plan structured as a linear chain over shared files — a very common vertical-slice TDD shape.

## Evidence

`arm validate --ci` after decompose-apply of the MIGH plan (2026-07-02): 8 WARNINGs, all between transitively-ordered pairs; zero after adding direct edges.

## Suggestion

Overlap detection should compute reachability over `blocked_by` (topological ordering), not just direct edges.
