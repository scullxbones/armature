# ADR: Task Dispatch Always Requires a Worktree

## Status

Accepted

## Principles touched

I3, I4

## Context

The harness hook enforces task scope and verification policy by reading the active task ID from the worktree's local git state. For this enforcement to be reliable, the task ID must be written by the `arm claim` call itself — not by a separate coordinator instruction the LLM might omit. The coordinator is an LLM and is stochastic; governance setup cannot depend on it following a sequence correctly.

The historical model allowed the coordinator to decide whether to use a worktree, defaulting to a single story-level worktree shared across all tasks. This left task binding as an optional, coordinator-chosen step, making governance contingent on LLM behavior.

Two alternatives were considered and rejected:

- **Per-task worktrees**: each `arm claim` creates a fresh task-scoped worktree. Rejected because sequential tasks need shared state — TASK-B often depends on TASK-A's compiled artifacts and filesystem changes. Isolating them requires a merge ceremony between every sequential task pair, adds Go compilation overhead at each boundary, and moves integration conflict resolution into the hot path.
- **Per-story worktrees with optional binding**: one worktree per story, task ID updated per claim if the coordinator passes `--worktree`. Rejected because the coordinator can omit `--worktree`, producing a governance gap with no visible failure.

## Decision

`arm claim` always requires `--worktree <path>`. The worktree is created if it does not exist. `arm claim` derives the branch name from the node type and ID with no coordinator input:

- Story or Feature: `feat/<id>` 
- Task (parallel): `task/<id>` off the parent story/feature branch
- Bug (parallel): `fix/<id>` off the parent story/feature branch
- Epic: no branch, no worktree

Sequential tasks share the parent story/feature worktree; `arm claim --worktree` updates the task binding on each sequential claim. Parallel tasks each receive a separate worktree. `arm merged --issue` removes parallel task/bug worktrees after integration.

The coordinator specifies the worktree path. It applies the convention: story/feature path for sequential tasks, task-scoped path for parallel tasks. This is the coordinator's only governance-sensitive responsibility — one required argument to a call it already must make.

## Consequences

The coordinator skill protocol must be updated to always pass `--worktree` to `arm claim`. `arm claim` without `--worktree` is an error. Human operators are not the expected callers of `arm claim`; the ergonomic cost is acceptable.

The harness hook reads the task ID from `.git/armature-task-id` (written by `arm claim`) before falling back to the `ARMATURE_TASK_ID` environment variable (retained for process-isolation platforms: Codex, Devin). Missing or stale bindings produce a hook pass-through, which is logged to `.git/armature-hook.log`. `arm merged` warns if pass-throughs were recorded during the task's execution.
