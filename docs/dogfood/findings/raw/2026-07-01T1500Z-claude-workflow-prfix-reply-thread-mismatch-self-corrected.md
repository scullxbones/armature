---
area: workflow
writer: claude
---

## What I was trying to do

Second consecutive run of the same `/prfix` multi-tier chain (haiku fix →
opus review → sonnet finalize) on PR #66, this time against two *new*
Codex findings that appeared after the previous cycle's commit (P1: migrated
ops never committed in `runRepoSetup`; P2: `arm push-ops` referenced by the
post-commit hook but never implemented anywhere in the codebase).

## What happened

- Haiku fixed both findings with real TDD tests; `make build`/`lint`/`test`
  green.
- Opus review found both fixes structurally correct (verified ordering
  against the dirty-tree guard, commit scoping, worktree targeting, and CLI
  command-group wiring by reading the actual git/cobra internals, not just
  the diff) and surfaced three minor hardening items: `push-ops` swallowing
  push failures as a false success, JSON output being dropped on the error
  path, and thin test coverage (existence-only test, no idempotent-migration
  test).
- Sonnet implemented all three hardening items via TDD, then handled
  finalize (commit, push, reply, resolve). It self-reported a mistake mid
  finalize: its first reply-to-thread call posted the P2-content message
  onto the P1 thread (a node/comment-ID mixup when replying to two GraphQL
  review threads back-to-back). It caught this itself, used
  `updatePullRequestReviewComment` to correct the misplaced reply, and only
  then resolved both threads. I verified post-hoc via the GraphQL API that
  both threads carry correct, distinct, accurate reply content and are
  marked resolved.

## How it changed behavior, confidence, or time spent

The self-correction worked, but it was only caught because the agent
happened to notice and fix it before reporting done — nothing in the
finalize instructions required it to re-fetch and diff the posted comments
against intended content before resolving. Had it resolved the threads
without noticing the swap, a human reviewer would see a P1 reply on the P1
thread that actually describes the P2 fix (and vice versa), which reads as
either confused or careless review hygiene, and is hard to notice later
since resolved threads collapse in the GitHub UI. I added an explicit
post-hoc verification step (fetching all threads via GraphQL and diffing
body text against expectation) before considering the run complete.

## Evidence

- Final GraphQL query after the run confirms both threads
  (`PRRT_kwDORnVQE86NnFXi` P1, `PRRT_kwDORnVQE86NnFXo` P2) are
  `isResolved: true` with reply bodies matching their respective findings —
  but this required an independent check from the orchestrating session,
  not just trusting the subagent's self-report.
- Commit `a4a86203` on `feat/SB-ELIM` contains the three hardening changes;
  `make build`/`lint`/`test` all green (41 packages passed).
