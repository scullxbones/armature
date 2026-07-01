---
date: 2026-06-30
agent: claude
area: integration
task: DF-S5
tags: [git-worktree, merge, arm-claim]
---

# Worker's committed worktree changes reappeared as uncommitted diffs in the main worktree

## User Goal

After a worker finished and committed its work inside its own `arm claim`-created
worktree (e.g. `.worktrees/df-s5-t8` on branch `task/DF-S5-T8`), the coordinator
switched to the main worktree on `feat/DF-S5` and ran `git merge task/DF-S5-T8`.

## Observed

Twice (DF-S5-T8 and DF-S5-T3), the merge failed with:

```
error: Your local changes to the following files would be overwritten by merge:
    internal/skillsembed/skills/armature-reviewer/SKILL.md
Please commit your changes or stash them before you merge.
```

`git status` in the **main worktree** (which had never been touched by hand) showed
the exact same content the worker had already committed on its task branch, but as
an *uncommitted* modification plus untracked new files. `git log task/DF-S5-T8`
confirmed the commit existed cleanly on the task branch. The stray working-tree state
in the main worktree had to be discarded (`git checkout --`, `rm -rf` untracked
files) before the merge could proceed.

## Impact

This looked alarming — like the worker had written into the wrong directory — and
required manual investigation (checking `git log`, diffing branches) before it was
safe to discard. It happened silently, with no error at claim/dispatch time, and
recurred for a second task later in the same session. Root cause wasn't conclusively
identified (possibly a coordinator dispatch pattern issue, possibly worktree/index
interaction), which is itself a finding — it should be reproducible and diagnosed.

## Evidence

```
$ git status --short
 M internal/skillsembed/skills/armature-reviewer/SKILL.md
?? internal/skillsembed/skills/armature-reviewer/references/field-rules.md
?? internal/skillsembed/skills/armature-reviewer/templates/
$ git log task/DF-S5-T8 --oneline -1
585a1f44 docs(DF-S5-T8): add citation self-validation and JSON template to reviewer skill
```
Recurred identically for DF-S5-T3 / `armature-coordinator/SKILL.md`.

## Suggested Follow-Up

Reproduce with a minimal two-task parallel-worktree dispatch and bisect: is this
`arm claim`'s worktree creation leaving shared index state, or a coordinator-side
mistake (e.g. an errant `cp`/`cd` in the dispatch script) rather than an `arm` bug?
Until root-caused, coordinator skill should warn to check `git status` in the main
worktree before every merge and discard stray changes rather than assume divergence.
