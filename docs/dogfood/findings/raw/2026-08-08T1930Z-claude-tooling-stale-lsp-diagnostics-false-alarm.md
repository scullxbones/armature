---
area: tooling
writer: claude
date: 2026-08-08T19:30Z
pr: 89
story: LNGHZN-S5
---

# Stale LSP diagnostics fire false compiler errors after subagent edits

## What the agent-user was trying to do

Orchestrate PR-fix subagents that edit Go files (`internal/worktree/reconcile.go`,
`cmd/armature/worktree.go`, etc.), then trust the result of the subagent's
`make check` before moving to the next phase.

## What happened

Twice in one session, immediately after a subagent finished editing, the harness
injected `<new-diagnostics>` blocks reporting compiler errors that looked serious:
`issue.WorktreePath undefined`, `not enough arguments in call to Reconcile`,
`cannot use ... as time.Time`. These implied the branch was broken.

Both times, running `make build` + `go vet` directly showed **BUILD_SUCCESS / VET_OK**.
The diagnostics were a stale LSP snapshot captured mid-edit (after some files were
changed but before the matching call-site/field edits landed), surfaced to the
orchestrator *after* the subagent had already reconciled the whole set.

## How it changed behavior / confidence / time spent

- Each occurrence forced an extra verification round (build + vet) to disprove the
  diagnostics before continuing — a real tax on a multi-phase loop.
- More dangerous failure mode: an orchestrator that *trusts* the diagnostics could
  wrongly conclude a green branch is broken and dispatch a needless "fix" round, or
  one that *ignores* them could miss a real error. The signal is untrustworthy in
  both directions right after a batch of subagent edits.

## Evidence

- Round 2: diagnostics claimed 3 compiler errors in `worktree.go`/`reconcile.go`;
  `make build` → `BUILD_SUCCESS`, `go vet ./internal/worktree/... ./cmd/armature/...` → `VET_OK`.
- Round 1 (earlier this session): near-identical `WorktreePath undefined` diagnostics;
  `make build` → `BUILD_SUCCESS`.

## Takeaway

Fits [[tooling-integration-gaps]]. When a subagent performs a batch of edits across
files with interdependent signatures, the LSP diagnostic snapshot delivered to the
parent can lag the final consistent state. Treat post-subagent `<new-diagnostics>`
as *suspect* — reconcile against an authoritative `make build`/`go vet` before acting,
never as ground truth on their own.
