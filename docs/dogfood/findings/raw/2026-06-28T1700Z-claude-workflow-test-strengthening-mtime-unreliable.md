---
writer: claude
area: workflow
slug: test-strengthening-mtime-unreliable-due-to-system-binary
date: 2026-06-28T17:00Z
---

# Mtime-based "no rematerialization" assertion fails due to system arm binary side effects

## What I was trying to do

Strengthening the `TestHookDetectScopeChanges_WithExistingCheckpoint` test in `cmd/armature/hook_test.go` to assert that `hookDetectScopeChanges` does NOT write/update `checkpoint.json` (i.e., that switching from `store.Load()` to `store.ReadIndex()` is actually tested). The opus review correctly identified that the existing test passed with both the buggy and fixed code.

## What happened

The haiku agent tasked with this fix (Finding A) initially tried the obvious approach: capture the mtime of `checkpoint.json` before calling `hook run post-commit`, then assert it was unchanged after. This approach was abandoned because:

> "The installed system `arm` binary's `prepare-commit-msg` hook calls `arm show active-claim` which triggers `snapshot.Load`, corrupting mtime between the capture point and assertion."

The integration test environment runs real git operations including hooks. The `arm` binary installed system-wide (or in the PATH) fires its own `snapshot.Load` during the commit's `prepare-commit-msg` hook before `hookDetectScopeChanges` even runs — so `checkpoint.json` gets written/touched by the hook infrastructure, not by the function under test.

The agent pivoted to an indirect assertion: inject a fake entry into `index.json` that doesn't exist in ops, then verify scope-rename ops reference it (which would only be possible if `ReadIndex` was used rather than `Load`, since `Load` would materialize from ops and produce a clean index without the fake entry).

## Why it matters

- "Assert no checkpoint.json written" is a pattern used in `TestStore_ReadIssue_ReadsFromDiskWithoutMaterialize` in `internal/snapshot/snapshot_test.go` — it works there because that test is a pure unit test with no git commits. It does not work in handler-level integration tests that run real commits with real git hooks.
- A coordinator or developer following the established pattern may waste time debugging why the mtime assertion is flaky before discovering the system binary interference.
- The fake-index-entry trick is clever but less obvious than mtime; it needs a comment explaining why it's used instead of the simpler approach.

## Evidence

- Haiku agent report: "Mtime approach was abandoned because the installed system arm binary's prepare-commit-msg hook calls `arm show active-claim` which triggers `snapshot.Load`, corrupting mtime."
- `TestStore_ReadIssue_ReadsFromDiskWithoutMaterialize` in `internal/snapshot/snapshot_test.go` uses the mtime/existence approach successfully — unit test context, no git commits.

## Potential mitigations

- Document in the test suite or a comment near integration tests: mtime assertions on state files are unreliable when tests invoke real git commits with `arm` hooks installed.
- Consider a "no-hooks" test mode or a test-only flag that disables hook side-effects during integration tests.
- The fake-index-entry trick should have a comment explaining why it's preferable to mtime here.
