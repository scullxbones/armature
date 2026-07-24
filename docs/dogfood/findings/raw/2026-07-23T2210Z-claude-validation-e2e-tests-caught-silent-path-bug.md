---
date: 2026-07-23
agent: claude
area: validation
task: LNGHZN-S3 coordination
tags: [testing, path-resolution, review-gate, silent-failure]
---

# Named-acceptance-test enforcement forced end-to-end tests that caught a silent, self-consistent path bug

## User Goal

Remediating LNGHZN-S3-T1 after the armature-reviewer's second RED pass, which
required adding tests under six literal contract-mandated names (e.g.
`TestShouldHeartbeatSkipsAlreadyStaleClaim_REQ_LNGHZN_S3_T1`).

## Observed

The original delivery's unit tests called `tryEmitHeartbeat(repoPath, issueID,
eventKind)` directly and read the resulting op back from a log path the test
itself computed the same (buggy) way the production code did:
`filepath.Join(repoPath, ".armature", "issues", "ops", ...)`. Because both the
production code and the test independently reconstructed the same wrong path,
the tests passed cleanly — there was no way for that style of unit test to
notice the divergence from the real ops directory
(`config.Context.IssuesDir` + `/ops`, i.e. `.armature/ops` in the collapsed
layout, confirmed via `internal/config/context.go`). Only once the reviewer's
literal-test-name requirement forced writing tests that drive the real
`harness-hook` CLI command end-to-end (via `newRootCmd().Execute()`, exercising
the actual `config.Context` resolution path) did the mismatch surface — one of
those tests failed with "issue not found" during unrelated debugging of a
different scenario, which is what led to discovering that heartbeat ops were
being written somewhere `materialize`/`snapshot` never read.

## Impact

This is a case where enforced friction (rejecting non-canonical test names)
paid for itself: it forced a testing style change (CLI-level over
internal-function-level) that caught a bug with real production consequences
(hook-emitted heartbeats having zero actual effect on claim liveness in any
real repo, silently, because the write "succeeded" into the wrong directory
and fail-open error handling never fired). Discovering and fixing this cost
roughly one extra remediation round beyond the two already needed for the
naming issue itself.

## Evidence

- Original `tryEmitHeartbeat` signature: `func tryEmitHeartbeat(repoPath,
  issueID string, eventKind harnesshook.EventKind)`, computing
  `issuesDir := filepath.Join(repoPath, ".armature", "issues")` internally.
- `internal/config/context.go`'s `resolveIssuesDir`/`issuesDirFor`: for a
  collapsed worktree, `IssuesDir` resolves to the worktree root itself (e.g.
  `.armature`), not `.armature/issues` — confirmed by grepping
  `resolveWorkerAndLog` (the canonical manual-heartbeat path), which builds
  `fmt.Sprintf("%s/ops/%s.log", ctx.IssuesDir, ownerID)`.
- Fixed signature: `func tryEmitHeartbeat(repoPath, issuesDir, worktreePath,
  issueID string, eventKind harnesshook.EventKind)`, called with
  `appCtx.IssuesDir`/`appCtx.WorktreePath` at the call site.
- The bug was undetectable via `go build`, `go vet`, or the original unit tests
  — all green throughout, because the wrong path was internally self-consistent.

## Suggested Follow-Up

- Consider adding guidance to the armature-worker or armature-planner skill:
  when a new code path's job is to read/write filesystem state that another
  part of the system depends on (an ops log, a state file, a config path),
  prefer a test that drives the real CLI/command entry point end-to-end over
  calling the new internal function directly — a test written against the same
  (possibly wrong) assumptions as the implementation cannot catch a shared
  wrong assumption.
- This also argues for keeping acceptance criteria specific (literal test names
  tied to described behavior, not just "add tests") as a first-class planner
  practice, since the specificity itself is what forced the effective test
  style here.
