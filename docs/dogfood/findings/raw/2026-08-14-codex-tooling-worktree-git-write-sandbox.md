---
date: 2026-08-14
agent: codex
area: permissions
task: LNGHZN-S9-T1
tags: [sandbox, git-worktree, arm, go-cache]
---

# Worker verification and task-log writes need two writable locations

## User Goal

Run focused Go tests and record the required Armature decision while working in a linked task worktree.

## Observed

The default Go build cache under the user cache directory was read-only, and `arm decision` could not create the linked worktree Git index lock because the main repository `.git/worktrees` metadata was read-only in the sandbox.

## Impact

Focused tests required a temporary `GOCACHE`, and the Armature state write required an explicit permission escalation. The errors were clear enough to recover, but the worker flow cannot complete using its defaults in this environment.

## Evidence

- `go test ./cmd/armature`: `open /home/brian/.cache/go-build/...: read-only file system`
- `arm decision LNGHZN-S9-T1 ...`: `Unable to create .../.git/worktrees/-arm/index.lock: Read-only file system`

## Suggested Follow-Up

Provide a writable per-task Go cache and linked-worktree Git metadata path in the worker environment.
