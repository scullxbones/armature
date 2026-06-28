---
date: 2026-06-27
writer: coordinator
area: workflow
slug: parallel-git-checkout-race
---

# Parallel git checkout + merge commands in the same worktree caused branch cross-contamination

## What the agent was trying to do

After three parallel haiku workers completed their T1 tasks (S15-T1, S16-T1, S17-T1),
the coordinator dispatched three parallel Bash tool calls to merge each task branch
into its story branch:

```bash
# Three parallel Bash calls:
git checkout feat/ARCHIMP-S15 && git merge task/ARCHIMP-S15-T1 --no-edit
git checkout feat/ARCHIMP-S16 && git merge task/ARCHIMP-S16-T1 --no-edit
git checkout feat/ARCHIMP-S17 && git merge task/ARCHIMP-S17-T1 --no-edit
```

## What happened

All three commands ran concurrently in the same working directory. The `git checkout`
calls raced — the last checkout to win set the active branch. Subsequent `git merge`
commands then merged into whatever branch happened to be active at the time.

Result:
- `feat/ARCHIMP-S15` received S17-T1's `internal/harnesshook/hook.go` commits
- `feat/ARCHIMP-S16` also received S17-T1's commits
- `feat/ARCHIMP-S17` correctly received only S17-T1

Both S15 and S16 story branches were contaminated with unrelated S17 files.

## Impact

- Story branches contained out-of-scope files from another story's task
- PRs would have shown harnesshook changes in S15 and S16 diffs (wrong)
- Required detective work and branch reconstruction via cherry-pick
- Extra time: ~15 minutes to diagnose and recreate clean branches

## Evidence

```
git log --oneline main..feat/ARCHIMP-S15
1504dc9d feat(ARCHIMP-S15-T2): Replace inline overlap loop...
40ed1378 feat(ARCHIMP-S17-T1): Add Hook type...   ← wrong!
7abfc5e2 feat(ARCHIMP-S15-T1): Add claim.PlanClaim...
```

## Recommended remediation / investigation

1. **Never run parallel `git checkout` + `git merge` in the same working tree.**
   Git's working tree has a single HEAD — parallel checkouts race.

2. **Coordinator skill should state explicitly:** when integrating multiple task
   branches, do the merges sequentially (one story at a time) even if the stories
   themselves are independent.

3. **Preferred pattern for multi-story parallel work:** use separate git worktrees
   per story branch (e.g. `git worktree add /tmp/s15-work feat/ARCHIMP-S15`),
   then merge in each worktree independently without touching the main working tree.

4. **Alternative:** after parallel workers complete, integrate task branches into
   story branches one at a time with explicit `git checkout` before each merge,
   not as parallel Bash calls.
