---
date: 2026-09-27
agent: cursor
area: tooling
task: pre-push graph validate guard
tags: [lint, errcheck, tests]
---

# errcheck on os.Remove after git reset is a false fail

## User Goal

Satisfy golangci-lint errcheck in `TestPushOpsRefusesRemoteW1Shape` by
replacing `_ = os.Remove(...)` with `require.NoError`.

## Observed

`git reset --hard HEAD~1` already dropped the committed `claimord-worker.log`
from the worktree. `os.Remove` then returns ENOENT and the test fails before
the W1 assertion.

## Impact

A lint-only edit turned a passing proof test red. The #284 remote-overlap
scenario still works; the fixture cleanup was the only breakage.

## Evidence

- `go test ./cmd/armature -run TestPushOpsRefusesRemoteW1Shape`
- `os.RemoveAll` returns nil when the path is already gone

## Suggested Follow-Up

Prefer `os.RemoveAll` (or `os.IsNotExist`) for cleanup after `git reset`,
not `require.NoError(os.Remove)`.
