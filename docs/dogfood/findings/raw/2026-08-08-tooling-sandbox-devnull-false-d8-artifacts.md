---
date: 2026-08-08
agent: claude
area: tooling
task: LNGHZN-S5 (coordinator, doctor gate)
tags: [sandbox, arm-doctor, D8, false-positive]
---

# Sandbox `/dev/null` overlay nodes appear as untracked files, causing false `arm doctor` D8 errors

## User Goal

Get `arm doctor` clean before dispatching workers and before story sign-off.

## Observed

Running `arm doctor` inside the command sandbox reported a D8 failure ("out-of-scope artifacts") listing home-style dotfiles at the repo root: `.bashrc`, `.zshrc`, `.gitconfig`, `.mcp.json`, `.claude/agents`, `.claude/hooks`, etc. `git status` also showed them as untracked. `ls -la` revealed they are character-device nodes (`crw-rw-rw- nobody nogroup 1, 3` = `/dev/null`) — sandbox overlay masks over sensitive paths, not real files. Re-running `arm doctor` with the sandbox disabled showed **D8 clean**. The armature-auditor subagent (running sandboxed) later re-reported the same false D8, attributing the artifacts to the most-recently-completed task.

## Impact

- False FAIL on a mandatory gate; wasted time distinguishing sandbox artifacts from a real out-of-scope-write problem. A coordinator that trusts the sandboxed result would either block on nothing or, worse, try to `rm` the user's dotfiles.
- The auditor subagent's verdict was misleading because it ran sandboxed and couldn't tell.

## Evidence

- Sandboxed: `✗ D8: Out-of-scope artifacts detected ... - .bashrc / .zshrc / .claude/agents ...`.
- `ls -la .bashrc` → `crw-rw-rw- 1 nobody nogroup 1, 3 .bashrc` (device 1,3 = /dev/null).
- Sandbox disabled: `✓ D8: No out-of-scope artifacts detected`.

## Suggested Follow-Up

`arm doctor`'s D8 check should ignore non-regular files (skip anything that isn't a regular file/dir via `os.Lstat` mode check) so `/dev/null` overlay nodes never count as artifacts. Document that doctor/auditor gates must run unsandboxed (or that sandbox overlays can inject phantom untracked entries).
