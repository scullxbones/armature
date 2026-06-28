---
writer: claude
area: commands
slug: arm-claim-worktree-required
date: 2026-06-28T22:00Z
---

# `arm claim` requires `--worktree` but coordinator skill doesn't mention it

## What I was trying to do

Coordinating SMTC-S1 story. Following the coordinator skill's step 4: "Claim and get context for each task: `arm claim TASK-ID --ttl <minutes>`". Ran the command exactly as shown.

## What happened

```
$ arm claim SMTC-S1-T1 --ttl 120
Error: --worktree is required
{"error":"--worktree is required","code":"general_error","exit_code":1}
```

The coordinator skill's dispatch protocol shows `arm claim TASK-ID --ttl <minutes>` without `--worktree`. The actual CLI requires `--worktree` on every claim. Had to read `arm claim --help` to discover that `--worktree` is mandatory and creates a git worktree at the given path.

## Why it matters

- Coordinator dispatches every wave by first claiming tasks — if the claim command is wrong, the entire wave fails immediately.
- The error message is terse; it doesn't tell you what path format to use or where worktrees should live.
- The user's goal specified `./.worktrees` as the directory — there's no standard guidance on this in the skill.
- Had to infer the correct invocation: `arm claim SMTC-S1-T1 --ttl 120 --worktree /absolute/path/to/.worktrees/SMTC-S1-T1`

## Evidence

- `arm claim SMTC-S1-T1 --ttl 120` → exit 1, `--worktree is required`
- `arm claim SMTC-S1-T1 --ttl 120 --worktree /home/brian/development/armature/.worktrees/SMTC-S1-T1 --force` → success

## Potential mitigations

- Update coordinator skill step 4 example to include `--worktree ./worktrees/TASK-ID`
- Add a note on worktree path conventions (absolute vs relative, naming scheme)
- Consider making `--worktree` optional with a sensible default (e.g., `./.worktrees/<task-id>`)
