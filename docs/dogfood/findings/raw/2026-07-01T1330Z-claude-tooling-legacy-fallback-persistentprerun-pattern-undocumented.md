---
date: 2026-07-01
agent: claude
area: tooling
task: PR-66-review-remediation
tags: [pattern-reuse, documentation-gap, cobra]
---

# The "delegate to root PersistentPreRunE, fall back to legacy probe" pattern exists twice now with no shared reference

## User Goal

Give `arm doctor` the ability to run against legacy (pre-migration,
single-branch) repos without breaking its normal context resolution on
modern repos, per a PR review finding on PR #66.

## Observed

`cmd/armature/decompose.go` already had a working pattern for a subcommand
that sometimes needs to bypass root's mandatory `armature.ops-worktree-path`
probe: try `cmd.Root().PersistentPreRunE(cmd, args)` first, fall through only
on error. The first (haiku) implementation of the doctor fix did not reuse
this pattern — it re-derived format/non-interactive/context-resolution logic
from scratch in `doctor.go`, which both duplicated `main.go`'s root logic
(drift risk) and dropped the `StateDir` assignment step, causing the P0
regression logged separately in this findings set. The opus review had to
explicitly point at `decompose.go` as the reference implementation before
the second (sonnet) pass converged on the same delegation pattern.

## Impact

Two independent commands (`decompose`, now `doctor`) implement variations of
"delegate to root, fall back on error" with no shared helper or documented
convention. A future subcommand needing the same legacy-repo escape hatch is
likely to re-derive it from scratch again (as haiku did here), risking the
same StateDir-omission class of bug. This is a real but low-urgency
maintenance risk, not a shipped bug — flagging for future consolidation.

## Evidence

- `cmd/armature/decompose.go`'s `PersistentPreRunE` (around lines 45-50):
  delegates to `cmd.Root().PersistentPreRunE`.
- `cmd/armature/doctor.go`'s first-pass `PersistentPreRunE` (haiku commit):
  duplicated root's format/non-interactive block instead of delegating.
- Opus review finding #2 explicitly named this as "drift risk" and pointed at
  `decompose.go` as the pattern to mirror.
- Final `doctor.go` (sonnet commit `be54f5fa`) now matches `decompose.go`'s
  approach.

## Suggested Follow-Up

Consider extracting the "delegate to root PersistentPreRunE, fall back to a
legacy/degraded probe on error" logic into a small shared helper (e.g. in
`cmd/armature` or `internal/config`) once a third subcommand needs it, so the
pattern is discoverable rather than something each new command re-derives
(or fails to) independently.
