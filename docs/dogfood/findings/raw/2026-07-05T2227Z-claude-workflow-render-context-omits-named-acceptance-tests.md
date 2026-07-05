---
date: 2026-07-05
agent: claude
area: workflow
task: ARCHIMP-S18
tags: [render-context, acceptance-criteria, review, red-rating]
---

# Workers rated red for tests named in acceptance criteria they never saw

## User Goal

Dispatch workers with `arm render-context --format agent` output as the complete task spec.

## Observed

`render-context --format agent` emitted only the core_spec layer (title, scope, Definition of Done). The acceptance criteria — which name required test functions like `TestSnapshotCurrentTruthAccess_REQ_ARCHIMP_S18_T3` — were not included. Workers implemented to the DoD, and the reviewer (who sees acceptance criteria via `arm review prepare`) rated deliveries red/yellow for the missing literally-named tests. T3 required a full remediation round-trip (extra worker + re-review) purely because of this asymmetry; T1/T2/T4 collected the same finding.

## Impact

One full remediation cycle on T3 (~20 min of agent time) and recurring yellow ratings across all four tasks. The coordinator had to compensate by manually warning the T4 worker about `_REQ_` test names.

## Evidence

- `arm render-context ARCHIMP-S18-T3 --format agent` → layers contain only core_spec, no acceptance array.
- T3 first review: acceptance[0]/[1] `not_satisfied`, rating red; second review after adding the two named tests: green.
- Related prior finding: 2026-07-03T1810Z-claude-workflow-render-context-omits-acceptance-criteria.md (still unfixed).

## Suggested Follow-Up

Include the acceptance criteria array in the agent-format render-context layers; reviewers and workers must see the same contract.
