# Theme: Git Worktree Friction

## Summary

Using git worktrees for agent isolation (particularly when placed outside the primary project workspace, such as in `/tmp`) introduces IDE diagnostic false positives (gopls warnings/bleed), check-out conflicts with the active branch, and complex bookkeeping/cherry-picking steps.

## Evidence

- [gopls workspace discovery with git worktrees](../../raw/2026-06-22-tooling-worktree-lsp-workspace-warnings.md) - Gopls picks up `go.mod` in sibling worktree paths and surfaces false compiler errors.
- [LSP diagnostics bleed from /tmp worktree](../../raw/2026-06-27T0000Z-coordinator-tooling-worktree-lsp-bleed.md) - Diagnostics from active subagent work in `/tmp` bleed into the coordinator's main session context as false-positive warnings.
- [gopls false-positive errors with worktrees in /tmp](../../raw/2026-06-27T2108Z-claude-tooling-worktree-gopls-false-positives.md) - Worktrees outside the project directory flood the IDE with false-positive diagnostics, causing cognitive overhead.
- [Checked-out branch worktree failure](../../raw/2026-06-28T1700Z-claude-tooling-worktree-already-checked-out-branch.md) - Creating a git worktree directly from the currently checked-out branch fails, requiring workaround branches and subsequent cherry-picks.
- [`arm claim --worktree` required but undocumented in coordinator skill](../../raw/2026-06-28T2200Z-claude-commands-arm-claim-worktree-required.md) - `arm claim` requires `--worktree` flag on every invocation; coordinator skill examples omit it, causing immediate failure on first wave dispatch. Placing worktrees under `.worktrees/` inside the project root still causes gopls stale-workspace diagnostics after worktree removal.

## Candidate Follow-Ups

- Update the `superpowers:using-git-worktrees` skill and the coordinator skill to document the checked-out branch restriction and advise using detached HEADs (`git worktree add /path HEAD`) or temp branch names.
- Investigate placing worktrees inside the project root (e.g., `.worktrees/`) and configure `go.work` to exclude or correctly manage them to prevent gopls workspace warnings/bleed.
- Advise agents to ignore `new-diagnostics` system-reminders referencing `/tmp` paths during active worker dispatches.
