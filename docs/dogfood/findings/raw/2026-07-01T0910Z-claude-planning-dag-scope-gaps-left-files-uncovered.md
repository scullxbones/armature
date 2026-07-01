---
date: 2026-07-01
agent: claude
area: planning
task: SB-ELIM
tags: [dag, scope, coordination]
---

# DAG task scope left real call sites uncovered, only caught at final verification

## User Goal

Coordinate story SB-ELIM (eliminate single-branch mode) end-to-end: dispatch haiku
workers wave-by-wave per task scope, merge, and reach a fully green `make check`
before opening a PR.

## Observed

Two distinct scope gaps surfaced only during final verification (SB-T13), not
during normal wave dispatch:

1. `internal/tui/app/model.go` and `model_validation_test.go` called the
   pre-refactor `snapshot.Load`/`materialize.MaterializeAndReturn` signatures
   (with the `singleBranch` bool argument SB-T2/SB-T3 removed), but no task in
   the 13-task DAG listed these files in its `scope`. Nothing failed loudly
   until `go build ./...` was run against the fully merged tree.
2. Even after every task was `merged`, `go test ./cmd/armature/...` had 26
   failing tests — stale `.armature/` path assumptions (should have been
   `.arm/.armature/`) and stale auto-merge expectations (tests assumed
   transitioning to `done` auto-promoted to `merged`, a behavior SB-T1
   deliberately removed) that SB-T9/SB-T10 didn't fully clean up.

Both gaps were structural: the story's DAG was decomposed by file/behavior
area, but nothing in the DAG or SB-T13's own scope ("repo-wide grep and make
check; no files modified") accounted for the fact that the *last* task with
that title still needed real remediation work, not just verification.

## Impact

Coordinator had to run an unplanned extra remediation pass (one direct fix for
the tui/app files, one dispatched sonnet-model agent to fix 26 test
regressions) after all 13 tasks were nominally "merged" and before the story
could be honestly called done. This added roughly two full wave-cycles of wall
time that weren't visible in the original task list, and required the
coordinator to override SB-T13's "no files modified" framing once real
failures were found — a case where following the task's literal scope would
have produced a false-green story.

## Evidence

- `go build ./cmd/armature/...` errors after SB-T9 merge referencing
  `cfg.Mode undefined`, `snapshot.Load` arg-count mismatches in
  `internal/tui/app/model.go:247` — no task scope included this file.
- `go test ./cmd/armature/...` post-merge: 26 `--- FAIL` lines including
  `TestListCmd_StatusFilter`, `TestAppCtxStateDirSet`,
  `TestMergedRejectsNonDoneStatus`, etc.
- PR https://github.com/scullxbones/armature/pull/66 body documents both
  fixes under "Additional fixes found during final verification."

## Suggested Follow-Up

When planning a multi-file refactor story with `armature-planner`, consider a
mandatory "repo-wide reference sweep" step (grep for the old signature/symbol
across the whole tree, not just per declared scope) before releasing tasks to
workers, so cross-cutting call sites can't fall outside every task's scope.
Also consider giving the final "verification" task explicit permission/
expectation to modify files when it finds real regressions, rather than
framing it as read-only.
