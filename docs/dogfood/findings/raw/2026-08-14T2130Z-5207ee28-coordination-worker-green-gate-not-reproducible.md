---
area: coordination
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
date: 2026-08-14T21:30Z
story: LNGHZN-S9
---

# Worker-reported green gate was not reproducible at integration

## What the agent-user was trying to do

Accept the completed `LNGHZN-S9-T1` task after the worker reported focused
tests, build, and the full `make check` gate as passing.

## What happened

The coordinator reran the integrated gate from the story branch at the same
task commit. `make check` failed immediately in lint because the newly added
existing-branch mismatch error line exceeded the repository's line-length
limit. A remediation commit was required solely to wrap that expression before
the same gate completed successfully.

## How it changed behavior, confidence, or time spent

The worker's pass report could not be treated as delivery evidence. The
coordinator had to rerun the full expensive gate, dispatch another review, and
perform an additional remediation cycle. This extended a small task and showed
that prose-only gate reports are weaker than captured command exit evidence.

## Evidence

- The integrated lint failure identified a 163-character line in
  `cmd/armature/claim.go`.
- Commit `4b888fbc16cda4c0446dd4b57e42476e3bb665b2` only wraps that error and then
  passes focused tests, lint, build, coverage, mutation, documentation, census,
  and cross-compilation gates.
- The final conformance assessment still marks the behavioral `make check`
  criterion indeterminate because the review bundle contains no raw activity
  entry with a successful exit status.

## What would have helped

Record gate invocations and exit status as task activity that can be included in
the review bundle, and have the coordinator consume that evidence directly.
When no citable execution record exists, automatically require a fresh
integration rerun before accepting the worker's completion report.
