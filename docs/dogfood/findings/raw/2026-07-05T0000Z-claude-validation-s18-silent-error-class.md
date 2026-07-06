---
date: 2026-07-05
agent: claude
area: validation
task: prfix remediation of PR #70 (ARCHIMP-S18 seam deepening)
tags: [error-handling, review, seams, stale-review]
---

# S18 seam refactor introduced a class of silent-failure regressions, not one bug

## User Goal

Remediate the single unresolved review comment on PR #70 (dropped `VerifyAll` error in `arm stale-review`), then broaden review scope across the branch.

## Observed

The reviewer-flagged bug (`verifyResults, _ := lc.VerifyAll()` masking an unreadable sources manifest as "No stale sources detected.") was not isolated. A broad follow-up review of the branch found the same *class* of defect one layer up: the new `Verify()` short-circuits on `entry.SyncFailed` before fingerprint comparison, so `stale-review` silently excluded sync-failed sources whose upstream had changed — a behavior regression vs. main with no pinning test. Additional seam-quality issues clustered on the same refactor: `VerifyAll` N+1 manifest reads with a TOCTOU window, `Manifest.GetByURL` left with no production callers while `create.go` reimplemented it inline, and `Store.Refresh(ctx)` being a literal alias of `Load(ctx)` with both ignoring `ctx`.

## Impact

The original review comment under-scoped the problem; fixing only it would have left the P2 regression (silently unreviewed stale sources) in place. Cost was one extra full-branch review pass plus a six-finding fix batch. Confidence takeaway: seam-deepening refactors (moving logic behind Lifecycle/Store) tend to convert loud failures into silent skips at the new boundary, and per-comment remediation misses that.

## Evidence

- PR #70 thread PRRT_kwDORnVQE86Oc3Ed (manifest error masking), fixed + regression test in `cmd/armature/stalereview_test.go` (`TestStaleReviewCmd_CorruptManifest`).
- Branch review findings fixed in commit `e42c5282` on `feat/ARCHIMP-S18`: VerifyStale inclusion (`TestStaleReviewCmd_StaleSource_SyncFailed`), `verifyEntry` split, `Lifecycle.GetByURL`, `Store.Refresh` removal, `cmd.Context()` for `SyncAll`, fail-fast provider registry.
- `make build` / `make lint` / `make test` green before and after the batch.

## Suggested Follow-Up

When a review comment identifies a dropped error/silent fallback, grep the same diff for siblings of that pattern before closing the thread — consider encoding this in the prfix/review skills.
