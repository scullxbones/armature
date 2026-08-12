# Theme: Git Worktree Friction

## Summary

Using git worktrees for agent isolation (particularly when placed outside the primary project workspace, such as in `/tmp`) introduces IDE diagnostic false positives (gopls warnings/bleed), check-out conflicts with the active branch, and complex bookkeeping/cherry-picking steps.

## Evidence

- [gopls workspace discovery with git worktrees](../../raw/2026-06-22-tooling-worktree-lsp-workspace-warnings.md) - Gopls picks up `go.mod` in sibling worktree paths and surfaces false compiler errors.
- [LSP diagnostics bleed from /tmp worktree](../../raw/2026-06-27T0000Z-coordinator-tooling-worktree-lsp-bleed.md) - Diagnostics from active subagent work in `/tmp` bleed into the coordinator's main session context as false-positive warnings.
- [gopls false-positive errors with worktrees in /tmp](../../raw/2026-06-27T2108Z-claude-tooling-worktree-gopls-false-positives.md) - Worktrees outside the project directory flood the IDE with false-positive diagnostics, causing cognitive overhead.
- [Checked-out branch worktree failure](../../raw/2026-06-28T1700Z-claude-tooling-worktree-already-checked-out-branch.md) - Creating a git worktree directly from the currently checked-out branch fails, requiring workaround branches and subsequent cherry-picks.
- [Worker edited Makefile to add `-buildvcs=false` as a worktree workaround](../../raw/2026-06-27T0001Z-coordinator-workflow-worker-out-of-scope-makefile-edit.md) — A worktree-caused `make build` VCS-stamping error (out of scope for the task) was "fixed" by the worker editing the shared Makefile, rather than working around it locally (`go build ./...`) — an out-of-scope repo-wide change caused directly by worktree friction.
- [`arm claim --worktree` required but undocumented in coordinator skill](../../raw/2026-06-28T2200Z-claude-commands-arm-claim-worktree-required.md) - `arm claim` requires `--worktree` flag on every invocation; coordinator skill examples omit it, causing immediate failure on first wave dispatch. Placing worktrees under `.worktrees/` inside the project root still causes gopls stale-workspace diagnostics after worktree removal.
- [Worker's committed worktree changes reappeared as uncommitted diffs in the main worktree](../../raw/2026-06-30T2210Z-claude-integration-worktree-changes-leak-into-main-worktree.md) — Twice, merging a completed worker's task branch into the main worktree failed with "local changes would be overwritten," even though `git status` in the main worktree (never touched by hand) showed the exact content the worker had already committed — pointing at some form of state bleed between worktrees sharing the same repository.
- [Background worker edited the main checkout instead of its assigned worktree](../../raw/2026-07-05T2228Z-claude-workflow-worker-edited-main-checkout-instead-of-worktree.md) — Despite the dispatch prompt stating the working directory and branch explicitly, a haiku worker made all 20 file edits in the main repository checkout, ran its gates there, and transitioned the task to done — leaving the task branch empty and the story branch dirty.

- [LSP (gopls) repeatedly reported false-positive diagnostics for code that built and tested cleanly](../../raw/2026-07-23T2220Z-claude-tooling-lsp-false-positives-in-worktree.md) — At least five times in one session: "undefined" symbols and "unknown field" errors on code `go build ./...` had just compiled. Consistently accompanied by the workspace-boundary warning for files inside an `arm claim --worktree` path outside the main module's workspace.
- [Stale LSP diagnostics fire false compiler errors after subagent edits](../../raw/2026-08-08T1930Z-claude-tooling-stale-lsp-diagnostics-false-alarm.md) — Twice in one session the harness injected `<new-diagnostics>` reporting serious-looking breakage immediately after a subagent finished editing; `make build` and `go vet` were clean both times. The false alarm arrives at exactly the moment the coordinator is deciding whether to trust the subagent's result.

## Pattern

Beyond LSP/IDE false positives, worktree isolation has a second failure class: agents (and possibly git itself) don't reliably respect the worktree boundary. A worker can edit the wrong checkout despite explicit instructions, and committed worktree state can reappear as uncommitted diffs in a sibling worktree during merge — both point at the isolation being a convention the tooling doesn't enforce, not a hard boundary.

## Candidate Follow-Ups

- Update the `superpowers:using-git-worktrees` skill and the coordinator skill to document the checked-out branch restriction and advise using detached HEADs (`git worktree add /path HEAD`) or temp branch names.
- Investigate placing worktrees inside the project root (e.g., `.worktrees/`) and configure `go.work` to exclude or correctly manage them to prevent gopls workspace warnings/bleed.
- Advise agents to ignore `new-diagnostics` system-reminders referencing `/tmp` paths during active worker dispatches.
- Add a post-dispatch verification step: confirm `git rev-parse --show-toplevel` (or equivalent) inside the worker's session actually resolves to the assigned worktree path before trusting a "done" transition, since prompt instructions alone have already failed to prevent a worker from editing the main checkout.
- Investigate the root cause of committed worktree changes reappearing as uncommitted diffs in the main worktree at merge time — this needs a git-level explanation, not just a documentation fix.
