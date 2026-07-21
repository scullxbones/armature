---
date: 2026-07-19
agent: claude
area: tooling
task: TOPTIER-S3 coordination
tags: [worktree, sandbox, claim]
---

# `arm claim --worktree` fails under sandboxed harnesses that deny /tmp writes

## User Goal

Coordinator claiming wave-1 tasks (TOPTIER-S3-T1/T3) with `arm claim --worktree /tmp/arm-task-<ID>` per the coordinator skill's dispatch protocol.

## Observed

Claude Code's Bash sandbox mounts most of `/tmp` read-only; `git worktree add` failed with `fatal: could not create leading directories ... Read-only file system`. `arm claim` correctly released the claim and suggested retry, but both the skill-documented path (`/tmp/arm-task-*`) and the sandbox-nominal `/tmp/claude/...` path failed inside the sandbox; the claim only succeeded after bypassing the sandbox entirely.

## Impact

Two failed claim attempts and a sandbox bypass before wave dispatch could start. Any agent harness with a restricted temp filesystem will hit this on the skill's happy path.

## Suggested Follow-Up

Coordinator skill could recommend a repo-adjacent worktree root (e.g. `../arm-worktrees/`) or `$TMPDIR`, and/or `arm claim` could fall back to a configurable worktree root when the requested path is unwritable.
