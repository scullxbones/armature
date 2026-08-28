---
date: 2026-08-28
agent: codex
area: workflow
task: Close out AOC-S1-T1 and AOC-S1-T2 after PRs #113 and #114 merged
tags: [sync, post-merge, merged, i6, rebase-merge, stacked-pr]
---

# Post-merge hook runs but misses an asynchronous rebase merge

## User Goal

After GitHub merged stacked PRs #113 and #114, let Armature's installed
post-merge hook promote their tasks from `done` to `merged` and remove the
managed task worktrees.

## Observed

The hook ran when the clean local `main` checkout fast-forwarded to
`origin/main`, but printed `No merged branches detected.` Both
`AOC-S1-T1` and `AOC-S1-T2` remained `done`, and both canonical worktrees
remained registered and present.

PR #114's branch had been explicitly rebased onto `main` before push. GitHub's
asynchronous stacked-PR rebase merge then created new commit SHAs on `main`, so
the pushed branch tip was still not an ancestor of `main`. `arm sync` relies on
`BranchMergedInto`, so it silently missed the landed content.

Explicit closeout with `arm merged --issue AOC-S1-T1 --pr 113` and
`arm merged --issue AOC-S1-T2 --pr 114` succeeded without `--force`, recorded
both tasks as `merged`, and removed both clean managed worktrees.

## Impact

The hook appeared healthy because it executed successfully, but it did not
perform its advertised lifecycle action. A coordinator could trust the
zero-exit output, leave merged work recorded as merely `done`, retain worktrees,
and keep downstream tasks blocked. The manual verification and recovery added a
separate closeout pass after an otherwise successful merge.

## Evidence

- Post-merge output: `No merged branches detected.`
- PR #114 remote branch head before merge: `9bd52dde`
- PR #114 rebase-merged head on `main`: `772471ef`
- `git branch -r --contains 9bd52dde` listed only
  `origin/task/AOC-S1-T2`, not `origin/main`
- Before recovery, `arm show AOC-S1-T1 AOC-S1-T2` reported both tasks `done`
  and Git listed `.worktrees/AOC-S1-T1` and `.worktrees/AOC-S1-T2`
- After explicit recovery, both tasks report `merged` and neither worktree is
  present in `arm worktree list` or `git worktree list`
- Recurs: [arm sync is structurally blind to this repo's own squash-merge workflow](2026-08-23T1533Z-claude-workflow-sync-blind-to-squash-merges.md)

## Suggested Follow-Up

Make `arm sync` use stable merge evidence for squash and rebase strategies,
such as recorded PR state or patch/content equivalence. At minimum, distinguish
"no eligible issue was examined" or "branch is not an ancestor" from a
successful no-op so the post-merge hook cannot silently imply lifecycle closure.
