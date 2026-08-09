---
area: validation
writer: claude
date: 2026-08-09T15:10Z
pr: 89
story: LNGHZN-S5
---

# The delivery gate counts gitignored build artifacts as a dirty tree, training workers to skip it

## What the agent-user was trying to do

Transition `LNGHZN-S5-T6` to `done` after committing the work and running
`make check` to green.

## What happened

The delivery gate refused:

```
Error: delivery gate check failed:
  1. CleanTree: Working tree is not clean. Commit or discard changes to:
     bin/, coverage.out, mutesting-report/, scripts/__pycache__/

Use --skip-delivery-gate to override (audit trail will record the override)
```

The working tree was clean. `git status --porcelain` printed nothing. All four
paths are build artifacts produced by `make check` itself, and all four are
gitignored. The gate is counting **ignored** files as uncommitted changes.

So the sequence is: run the required quality gate, and the act of running it puts
the tree into a state the delivery gate rejects. Deleting the artifacts and
re-running the transition worked.

## How it changed behavior / confidence / time spent

- This is not new. `LNGHZN-S5-T1`'s recorded outcome ends with: *"Skipped
  delivery gate: bin/ and coverage.out are build artifacts, not part of task
  scope."* The same obstacle, the same artifacts, resolved the other way.
- That is the real cost. The error message names `--skip-delivery-gate` as the
  remedy, so the path of least resistance is to bypass an I5 deterministic gate
  because of files git already ignores. A worker who does this once learns that
  gate failures are usually noise. The next genuine failure gets the same reflex.
- Every worker on this repo hits it, because `make check` is mandatory and
  produces these artifacts every time.

## What would have helped

`CleanTree` honoring `.gitignore` — matching `git status --porcelain`, which is
what "clean working tree" means everywhere else in the toolchain. Failing that,
the gate naming the specific ignored paths it will tolerate, so the remedy is
never "turn the gate off".
