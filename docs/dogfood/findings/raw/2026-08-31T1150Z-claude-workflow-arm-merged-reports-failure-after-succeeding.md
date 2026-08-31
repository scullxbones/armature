---
date: 2026-08-31
agent: claude
area: workflow
task: Close out LNGHZN-S8-T2 after PR #123 merged, using the manual arm merged path
tags: [merged, worktree-gc, error-handling, partial-success, exit-code, wsl, ebusy]
---

# `arm merged` exits 1 after the transition already succeeded

## User Goal

Manually close out `LNGHZN-S8-T2` after PR #123 landed, since `arm sync` could
not detect the merge (see the companion finding on the unrecorded `branch`
field).

## Observed

`arm merged --issue LNGHZN-S8-T2 --pr 123` printed a bare error and exited 1:

```
Error: remove worktree for LNGHZN-S8-T2: git worktree remove …/.worktrees/LNGHZN-S8-T2: exit status 255
error: failed to delete '.git/worktrees/LNGHZN-S8-T2': Device or resource busy
{"error":"…","code":"general_error","exit_code":1}
```

The command had in fact done almost all of its job. Checked afterwards:

- `arm show LNGHZN-S8-T2` → `Status: merged` — the op was appended and
  materialized
- the working checkout `.worktrees/LNGHZN-S8-T2` was gone
- `git worktree list` no longer registered it
- `arm worktree list` returned empty in every category

Only the final step — deleting the `.git/worktrees/LNGHZN-S8-T2` administrative
directory — failed, with `EBUSY` from the filesystem (WSL2). Nothing in the
output distinguishes "the lifecycle transition failed" from "the transition
succeeded and a directory unlink failed."

The same `EBUSY` blocks `git worktree prune`, and residue from earlier runs
shows this has been happening for a while. Eight stale admin directories were
present, all reporting `gitdir file does not exist`, all failing to delete:
`LNGHZN-S6-T1..T4`, `LNGHZN-S7-T2`, `LNGHZN-S7-T3`, `LNGHZN-S8-T1`,
`LNGHZN-S8-T2`. So every prior `arm merged`/gc in this clone left the same
residue and presumably the same error.

## Impact

The failure mode is inverted from the companion finding: there, a no-op
presented as success; here, a success presents as failure. An agent following
the exit code would conclude the close-out did not happen and retry or escalate,
when the durable state was already correct and the only remaining artifact is a
disposable admin directory.

This matters more for agents than humans: exit 1 plus `code: general_error` is
exactly the shape a harness keys on. A retry is harmless here but not obviously
so in advance, and the operator has to go read three separate commands' output
to establish that the work is actually done.

The accumulating `.git/worktrees/*` residue is cosmetic on its own, but it means
`git worktree prune` never converges, so it cannot be used as a health signal.

## Evidence

- `arm merged --issue LNGHZN-S8-T2 --pr 123` → exit 1, error above
- immediately after: `arm show LNGHZN-S8-T2` → `Status: merged`
- immediately after: `arm worktree list` → all categories empty
- `git worktree prune -v` → 8 × `Removing worktrees/<id>: gitdir file does not
  exist` each followed by `error: failed to delete …: Device or resource busy`
- `.git/worktrees/` is itself writable — `mkdir`/`rmdir` of a probe directory
  both succeed — so this is a per-directory `EBUSY`, not a permission problem
- the stuck directories contain only ordinary files plus a `refs/` subdirectory

## Suggested Follow-Up

Treat administrative-directory cleanup as a non-fatal step. Once the `merged` op
is durable and the working checkout is gone, the lifecycle action has happened;
a failure to unlink `.git/worktrees/<id>` should be reported as a warning with a
zero exit, naming the leftover path, rather than as `general_error`.

If a non-zero exit is wanted for the residue, it needs a distinct code and a
message that states plainly that the issue *was* marked merged — the current
output leads with `Error:` and never mentions the transition succeeded.
