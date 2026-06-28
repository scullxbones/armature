---
date: 2026-06-27
agent: claude
area: tooling
task: prfix multi-agent fix loop on PR #59 using /tmp worktree
tags: [worktree, gopls, ide, false-positives, prfix]
---

# Worktrees outside the project dir flood IDE with gopls false-positive errors

## User Goal

Create a git worktree at `/tmp/armature-pr59` so multiple subagents could work on PR #59's branch without touching the main working tree.

## Observed

After each subagent committed to the worktree, the IDE (VSCode + gopls) emitted a flood of `new-diagnostics` system messages flagging every file touched in the worktree:

- `"use of internal package ... not allowed"` (gopls resolves imports from main workspace root, not worktree)
- `"undefined: someFunction"` (same cause — package is outside go.work scope)
- `"This file is within module ../../../../tmp/armature-pr59, which is not included in your workspace"`

These are all false positives: `make build && make lint && make test` inside the worktree passed clean (40 packages, 0 issues) every time.

## Impact

Each subagent's commit triggered a `<new-diagnostics>` system-reminder in the conversation, creating visual noise and requiring the parent agent to explicitly distinguish IDE errors from real errors. No actual build or test failures, but added cognitive overhead and could cause confusion in automated workflows that treat diagnostics as blocking signals.

## Evidence

After `950cc16a` committed in `/tmp/armature-pr59`:
```
snapshot_test.go: ⚠ This file is within module "../../../../tmp/armature-pr59"
snapshot.go: ✘ use of internal package not allowed
transition.go: ✘ undefined: appCtx, mustState, ...
```

Inside the worktree:
```
make build && make lint && make test
# → 0 lint issues; 40 packages passed
```

## Suggested Follow-Up

- Document in the `superpowers:using-git-worktrees` skill that worktrees outside the project directory will produce gopls false positives in the IDE; advise treating `new-diagnostics` from `/tmp/...` paths as noise.
- Alternatively, consider placing worktrees under the project root (e.g. `.worktrees/`) so they fall within the go.work scope.
