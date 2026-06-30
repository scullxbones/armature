# Session Recovery After Usage Limit: Branch Divergence Was Hard to Reconstruct

**Date:** 2026-06-27  
**Writer:** 5207ee28 (coordinator)  
**Area:** coordination  
**Task:** Completing ARCHIMP-S16 after session loss

## What the agent was trying to do

Continue coordinating ARCHIMP-S16 ("Phase 3: Render Context assembly — inject FileReader, collapse inputs") after the session was killed by a usage limit mid-execution.

## What happened

Recovery required ~8 sequential diagnostic commands before the coordinator understood the actual state:

1. `arm list --parent ARCHIMP` — showed S16 `in-progress`, S17 `merged`
2. `arm list --parent ARCHIMP-S16` — showed T1/T2 `merged`, T3 `done`
3. `git log --oneline` — showed HEAD on `feat/ARCHIMP-S17`, not `feat/ARCHIMP-S16`
4. `git log --all --grep="ARCHIMP-S16-T3"` — found the task branch had no impl commit, just ops
5. `git worktree list` — found `/tmp/armature-S16-T3` listed as "prunable" (directory gone)
6. `git diff feat/ARCHIMP-S16..feat/ARCHIMP-S17` — revealed S17 had *reverted* S16's FileReader changes

The root cause: S16 and S17 were developed in parallel from the same base commit (`ccf75c39`). S17 was deployed first and merged. S17's changes to `render_context.go`, `context_history.go`, `harness_context.go`, and `assemble.go` reverted S16's FileReader injection to the old `stateDir`/`graph` API. T3's worktree at `/tmp/armature-S16-T3` had been used to attempt the merge but the worktree was prunable (tmp path, rebooted between sessions) and the integration commit never landed on `feat/ARCHIMP-S16`.

## How it changed behavior

- Recovery took ~15 minutes of diagnostic work before any implementation could begin
- The coordinator had to implement T3 directly (against the coordinator skill's stated role) because the worker had lost its worktree
- The git merge of S17 into S16 was needed to get harness hook changes + FileReader together
- The T3 work itself (simplifying `repoRoot` derivation to `appCtx.RepoPath`) was small, but only discoverable by reading all three branches' versions of the same files

## What would have helped

- `arm doctor` or `arm list` showing "T3 was done in worktree path X, which no longer exists" or "T3 has no feat-branch commit"
- The coordinator skill's recovery section could note: when a task is `done` but has no commit on the feature branch, check if the worktree was at a temp path
- Storing the worktree path in the armature task claim record would make recovery unambiguous
