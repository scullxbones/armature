---
date: 2026-07-01
agent: claude
area: validation
task: PR-66-review-remediation
tags: [multi-agent-review, false-confidence, regression]
---

# make build/lint/test all passing did not catch a P0 regression the fix itself introduced

## User Goal

Fix two PR review findings on PR #66 via a fresh haiku subagent using TDD,
with `make build`, `make lint`, and `make test` as the acceptance gate before
handing off to an opus review pass.

## Observed

The haiku subagent's fix for "doctor fails on legacy repos" (giving `doctor`
its own `PersistentPreRunE`) introduced a regression: on the *normal* (modern,
non-legacy) repo path, the new `PersistentPreRunE` set `appCtx` directly
instead of delegating to root's `PersistentPreRunE`, so `appCtx.StateDir`
was left empty for every ordinary `arm doctor` invocation, not just the
legacy-repo case being fixed. `make build`, `make lint`, and full `make test`
(41 packages) all passed with this bug present — no existing test asserted
*where* doctor's materialize output landed, only that the commands
succeeded and returned expected exit codes/output content.

## Impact

Without a second independent review pass (the opus agent explicitly asked to
scrutinize ordering/edge-cases and re-verify), this P0 regression — every
`arm doctor` run on a modern repo silently materializing state into the
current working directory instead of the correct worktree state dir — would
have shipped past a green CI gate. The bug was only surfaced because the
orchestrator (1) noticed stray files in a `git status` diff and (2) had
independently scoped the opus reviewer to look for exactly this class of
issue ("does the fallback appCtx have all fields doctor's RunE actually
needs").

## Evidence

- Haiku subagent's final report claimed all three verification commands
  passed with no caveats.
- Opus review finding #1 (P0): traced the leak to `doctor.go`'s
  `PersistentPreRunE` never calling `stateDirFor`, unlike `main.go`'s root
  `PersistentPreRunE` and `decompose.go`'s delegation pattern.
- Fix (Sonnet implementation pass, commit `be54f5fa`): delegate to
  `cmd.Root().PersistentPreRunE(cmd, args)` first, matching `decompose.go`.

## Suggested Follow-Up

For fixes that touch a command's `PersistentPreRunE`/context-resolution
path, a green `make test` is not sufficient evidence of correctness — add an
explicit assertion (as the Sonnet pass eventually did) that state-writing
side effects land in the expected directory, not just that commands exit
zero. More generally: the three-stage haiku→opus→sonnet review loop caught
this precisely because it was structured as fix→independent-review→fix
rather than fix→ship; a single-pass "agent fixes and commits" flow would
likely have missed it.
