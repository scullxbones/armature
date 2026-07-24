---
date: 2026-07-23
agent: claude
area: permissions
task: LNGHZN-S3 coordination
tags: [worktree, sandbox, claim, timeout]
---

# `arm claim --worktree` silently hangs under sandbox instead of erroring

## User Goal

Acting as coordinator for story LNGHZN-S3, claiming task LNGHZN-S3-T1 into an
isolated git worktree per the armature-coordinator skill's dispatch protocol
(`arm claim TASK-ID --ttl 240 --worktree /tmp/arm-task-TASK-ID`).

## Observed

Running `arm claim LNGHZN-S3-T1 --ttl 240 --worktree /tmp/arm-task-LNGHZN-S3-T1`
(a path under plain `/tmp`, not the session's `/tmp/claude` scratchpad) produced
no output and no error — the Bash tool call ran until its 2-minute timeout and
was killed with exit code 143. No worktree was created at the target path.
Re-running `arm list --parent LNGHZN-S3` afterward showed the task had actually
transitioned to `claimed` server-side (the claim op succeeded), but the
`--worktree` git-worktree-creation step never completed or reported failure.
Retrying with `dangerouslyDisableSandbox: true` and a path under the session's
`/tmp/claude/` scratchpad succeeded immediately.

## Impact

Cost one full 2-minute timeout plus a diagnostic round (checking `arm list`,
`git worktree list`, `ls` on the target path) to figure out that the claim had
partially succeeded (issue state changed) while the worktree side silently
never returned, rather than failing fast with a permission error.

## Evidence

- First call: `Command timed out after 2m 0s`, exit code 143, no stdout/stderr.
- `arm list --parent LNGHZN-S3` immediately after showed `"status": "claimed"`
  for LNGHZN-S3-T1 — the underlying claim op had already landed.
- `ls /tmp/arm-task-LNGHZN-S3-T1` → `No such file or directory`.
- Retry with `dangerouslyDisableSandbox: true` and target path
  `/tmp/claude/arm-task-LNGHZN-S3-T1` returned instantly with
  `{"claimed_by":"...","issue":"LNGHZN-S3-T1","ttl":240}` and a working worktree.

## Suggested Follow-Up

- `arm claim --worktree` should fail fast (with a clear permission/IO error) if
  worktree creation can't proceed, rather than appearing to hang indefinitely —
  especially since the claim op itself had already been durably recorded by the
  time the hang was observed, leaving the issue in a claimed-but-no-worktree
  state that a coordinator has to manually detect.
- The armature-coordinator skill's dispatch protocol could note explicitly that
  `--worktree` paths should live under the session's writable scratchpad, not
  bare `/tmp`, to avoid this class of hang under sandboxed harnesses.
