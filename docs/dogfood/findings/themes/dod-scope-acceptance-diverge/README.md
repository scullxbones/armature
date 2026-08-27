# Theme: DoD / scope / acceptance diverge

## Summary

`dod`, `scope`, and `acceptance` are independent required fields. `arm
validate` checks presence and DoD length, not that DoD verbs are
implementable in the scoped files, and not that acceptance tests the
same surface as the DoD. Product sentences from LH/story docs get copied
into task DoDs while scope lists only new helpers. Workers then
correctly stay in scope; wrap Yellow (or a silent Green on unit tests
alone) is the first enforced detection.

Doctor check IDs are the same class of unallocated namespace: claimed in
DoD prose, consumed by whoever next edits `Run()`.

## Evidence

- [`Task DoD promised arm doctor D9; scope could not wire Run(); wrap Yellow is first fail`](../../raw/2026-08-27T0127Z-5207ee28-validation-dod-scope-acceptance-diverge.md) — LNGHZN-S7-T2 LH D1 language vs four new files; grilling pinned D9 without `doctor.go`; live D9 later taken by LNGHZN-S5-T8.
- [`DAG task scope left real call sites uncovered, only caught at final verification`](../../raw/2026-07-01T0910Z-claude-planning-dag-scope-gaps-left-files-uncovered.md) — decomposition by file area omitted the actual call sites; nothing failed until the merged tree.
- Related but distinct: [Scope-overlap validation gaps](../scope-overlap-validation-gaps/README.md) — those are overlap *warnings*. This theme is "the contract cannot be fulfilled by the scoped files even with zero overlap."

## Candidate Follow-Ups

- Validate (or planner skill) DoD∩scope: `arm doctor` / "gains check Dn" requires `internal/doctor/doctor.go` in scope, or DoD rewritten to helper-only.
- Acceptance must name the same surface as DoD (CLI output vs unit test).
- Doctor check-ID registry from live `Run()` plus open-task reservations.
- Do not copy LH/story product sentences into task DoDs.
