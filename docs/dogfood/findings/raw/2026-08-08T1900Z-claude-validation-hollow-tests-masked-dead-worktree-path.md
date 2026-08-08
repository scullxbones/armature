---
area: validation
writer: claude
date: 2026-08-08T19:00Z
pr: 89
story: LNGHZN-S5
---

# Hollow tests let a dead worktree-lifecycle code path survive multiple PR review rounds

## What the agent-user was trying to do

Remediate review comments on PR #89 (managed worktree lifecycle) across several
rounds. Each round, the reviewer bot surfaced new correctness findings about
`arm worktree list` / `arm worktree gc` (ghost scoping, GC binding checks, output
rendering, error propagation).

## What happened

A holistic review pass discovered that `arm worktree list`/`gc` loaded issues from
`filepath.Join(ctx.IssuesDir, "issues")` = `.armature/issues`, but materialized
issue JSONs actually live under `.armature/state/<worker-id>/issues`. `LoadAllIssues`
returns an empty slice (no error) on a missing dir, so `Reconcile` always received
zero issues. The entire feature was a **no-op in a real repo** — GC removed nothing,
list classified nothing.

A second latent no-op compounded it: `ctx.RepoPath` defaults to `"."`, so the
managed-root prefix was relative and never matched git's absolute worktree paths.

Every prior review round had been hardening logic **downstream of a Reconcile that
was always fed an empty map** — i.e. hardening dead code.

## How it changed behavior / confidence / time spent

- Multiple review rounds (and their remediation cost) were spent on a path that
  never executed, without anyone catching the root no-op.
- The `worktree_test.go` suite passed the whole time: tests asserted only
  `NotEmpty(out)` / `NotNil(jsonKey)` / "runs without error". An empty `Reconcile`
  satisfies all of them — `"No worktrees found..."` is non-empty; empty arrays are
  non-nil. The suite gave **zero** protection against the feature being inert.

## Evidence

- `cmd/armature/worktree.go` (pre-fix) lines ~40, ~126: `LoadAllIssues(filepath.Join(ctx.IssuesDir, "issues"))`.
- `materialize/pipeline.go:125`, `helpers.go:121`: real issues path is `stateDirFor(ctx, workerID)/issues`.
- End-to-end before fix: `arm worktree list --format json` → `"bound":[]` for a live claimed worktree; `arm worktree gc` → `removed_count:0`, worktree persists on disk.
- After wiring through `newSnapshotStore(ctx).Load` + absolute repo path: `"bound":["demo-1"]`, `gc` actually removes the dir.

## Theme

Fits [[test-coverage-gaps]]: assertion-free / existence-only tests that pass
regardless of behavior mask whole-feature failures. A test that cannot fail when the
feature is a no-op is not a test. Reviewer-round churn is a symptom — the deterministic
gate (`make check`) should have caught "feature does nothing" before human/LLM review
ever engaged.
