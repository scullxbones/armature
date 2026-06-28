---
writer: claude
area: tooling
slug: worktree-already-checked-out-branch
date: 2026-06-28T17:00Z
---

# Creating a git worktree from the currently checked-out branch fails

## What I was trying to do

Dispatching two parallel haiku subagents to fix independent findings in PR #59 (feat/ARCHIMP-S14). The plan was to create two worktrees from the PR branch so each agent could work in isolation.

## What happened

The first `git worktree add` call used the branch name directly:

```
git worktree add /tmp/archimp-s14-fix1 feat/ARCHIMP-S14
```

It failed with:

```
fatal: 'feat/ARCHIMP-S14' is already used by worktree at '/home/brian/development/armature'
```

Git does not allow adding a worktree that checks out the same branch the primary worktree has checked out. Had to pivot to creating new branches for each worktree:

```
git worktree add /tmp/archimp-s14-fix1 -b fix/s14-readissue
git worktree add /tmp/archimp-s14-fix2 -b fix/s14-hookdetect
```

Then cherry-picked the fix commits back into the main branch after the agents completed.

## Why it matters

- Extra steps (create branch, cherry-pick back) that wouldn't be needed if isolation weren't required.
- The coordinator must remember to cherry-pick from the right fix branches — a manual bookkeeping step that could be missed or done in wrong order.
- Agent prompts need to say "commit to branch fix/s14-X" rather than "commit to feat/ARCHIMP-S14", which adds complexity.

## Evidence

- `git worktree add /tmp/archimp-s14-fix1 feat/ARCHIMP-S14` → `fatal: 'feat/ARCHIMP-S14' is already used by worktree`
- Workaround succeeded; cherry-pick of both commits completed cleanly.

## Potential mitigations

- The `superpowers:using-git-worktrees` skill or the coordinator skill could document this pattern: always create a new branch (`-b`) when adding worktrees off the current branch.
- Alternatively, create worktrees by SHA (`git worktree add /path HEAD`) which does not have this restriction (detached HEAD mode) — acceptable for short-lived fix branches.
