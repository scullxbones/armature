# PRD: Harness Hook Worktree Governance

## Problem Statement

The harness hook enforces task scope and verification policy during AFK (automated) workflow — but it currently has no viable operating model for that workflow. It requires `ARMATURE_TASK_ID` to be set as an environment variable, yet the coordinator and worker skills never set that variable. The hook fails immediately on every tool call, making the harness unusable for design work, spec generation, and any session without an active claimed task. Additionally, environment variables cannot provide per-task isolation in a parallel-wave subagent model: two workers dispatched in the same Claude Code session share an environment, so a single variable cannot simultaneously represent two different task IDs.

## Solution

Governance is anchored to the worktree filesystem rather than the process environment. `arm claim` gains a required `--worktree <path>` flag that atomically claims the task, creates the worktree if needed, derives the git branch from the node type and ID, and writes the task ID to `.git/armature-task-id` within that worktree. The harness hook reads task binding from that file first, falling back to the `ARMATURE_TASK_ID` environment variable (retained for process-isolation platforms: Codex, Devin). When no binding is present, the hook passes through silently rather than erroring — absence of a task is a valid state, not a failure. Pass-through events are logged to `.git/armature-hook.log`. `arm merged --issue` tears down parallel task/bug worktrees and warns if pass-throughs were logged during the task's execution.

## User Stories

1. As a coordinator worker, I want `arm claim TASK-ID --worktree <path>` to create the worktree and bind the task ID in one call, so that governance is established without requiring additional setup steps.
2. As a coordinator worker, I want `arm claim` to derive the git branch name from the node type and ID, so that I do not need to decide or track branch names.
3. As a coordinator worker, I want `arm claim` to create the worktree on the correct branch if it does not already exist, so that workers are always dispatched into a properly configured worktree.
4. As a coordinator worker dispatching sequential tasks, I want `arm claim --worktree <path>` to update the task binding in an existing worktree, so that the same worktree can be reused across sequential tasks without stale enforcement.
5. As a coordinator worker dispatching parallel tasks, I want each `arm claim --worktree <path>` call to target a distinct path, so that parallel workers enforce their own task's scope independently.
6. As a coordinator worker, I want `arm merged --issue TASK-ID` to remove the worktree for parallel task and bug issues, so that I do not need to manage worktree teardown separately.
7. As a coordinator worker, I want `arm merged --issue TASK-ID` to warn me if hook pass-throughs were logged during the task's execution, so that I can decide whether to accept work that ran without governance enforcement.
8. As a developer doing design work or spec generation without a claimed task, I want the harness hook to pass through silently, so that design sessions are not blocked by a hook that has nothing to enforce.
9. As a developer, I want the hook to pass through silently when the bound task is no longer active (done, merged, cancelled), so that a stale binding from a previous sequential task does not block the next worker with the wrong scope.
10. As a developer, I want hook pass-through events logged to `.git/armature-hook.log`, so that the audit trail is available at integration time without polluting the ops log or being accidentally committed.
11. As a developer on Codex or Devin, I want `ARMATURE_TASK_ID` to continue working as a task binding source, so that process-isolation harness platforms are not broken by the new file-based model.
12. As a developer, I want `arm claim` to fail immediately if `--worktree` is omitted, so that coordinators cannot accidentally dispatch workers without governance in place.
13. As a worker agent, I want the harness hook to enforce only the scope of my currently bound task, so that I cannot modify files outside my task's declared scope even if I attempt to.
14. As a worker agent, I want the harness hook to block direct `git commit` commands regardless of task binding, so that Armature retains ownership of commits during harness execution.
15. As a developer, I want the convention for branch names to be consistent and predictable (story/feature → `feat/<id>`, task → `task/<id>`, bug → `fix/<id>`), so that branch management does not require coordinator judgment or documentation lookups.

## Implementation Decisions

- **`arm claim --worktree` is required.** Omitting `--worktree` is an error. `arm claim` is a coordinator operation; human ergonomics are not a design constraint here.

- **Worktree creation is create-or-update.** If the worktree path already exists, `arm claim --worktree` updates the task binding file without touching the branch or worktree structure. If the path does not exist, it creates the worktree and checks out the branch derived from the node type and ID.

- **Branch naming is derived by `arm claim` from node type and ID with no coordinator input:**
  - Epic: no branch, no worktree (error if `--worktree` is passed for an epic)
  - Story or Feature: `feat/<id>`
  - Task (parallel): `task/<id>` off the parent story/feature branch
  - Bug (parallel): `fix/<id>` off the parent story/feature branch
  - Sequential tasks/bugs share the parent worktree; no new branch is created on update

- **Worktree granularity is per parallel execution group, not per task.** Sequential tasks in a story share the parent story/feature worktree, updating the task binding on each claim. Parallel tasks each receive a distinct task-scoped worktree. The coordinator decides whether tasks are sequential or parallel and passes the appropriate path.

- **Task binding is stored at `<git-dir>/armature-task-id`** where `<git-dir>` is resolved via `git rev-parse --git-dir` from within the worktree. Files under `.git/` are never committed; no `.gitignore` entry is needed. Each worktree has its own git dir, providing natural isolation.

- **Hook task ID resolution order:**
  1. Read `<git-dir>/armature-task-id`
  2. Fall back to `ARMATURE_TASK_ID` environment variable
  3. If neither is present, pass through silently (exit 0)

- **Stale binding detection.** When a task ID is resolved, the hook loads the materialized state snapshot (already done today) and checks the task's current status. If the status is anything other than `claimed` or `in-progress`, the hook treats the binding as stale and passes through silently.

- **Pass-through logging.** Any pass-through (missing binding or stale binding) is appended as a line to `<git-dir>/armature-hook.log` recording the timestamp, tool name, and reason (`no-binding` or `stale:<status>`). This file is never committed.

- **`arm merged --issue` teardown.** For issues with node type `task` or `bug`, `arm merged` checks for a worktree whose path matches the conventional task/bug worktree location. If found, it reads `.git/armature-hook.log` from that worktree, emits a warning to stderr if any pass-throughs were recorded, then removes the worktree via `git worktree remove`. Story and feature worktrees are not removed by `arm merged` — they persist for the duration of the story.

- **`resolveTaskBinding(gitDir string) string`** is the new internal function that encapsulates reading the task binding file. It is used by the `harness-hook` command. It returns the empty string (not an error) when the file is absent.

- **`TestHarnessHookRequiresTaskID` becomes `TestHarnessHookPassesThroughWithoutTaskID`.** The existing test asserting an error on missing task ID must be inverted to assert a pass-through (exit 0, no decision output or a pass-through decision).

## Testing Decisions

Good tests exercise the external behavior of the command — what the caller observes — not internal implementation. They do not assert on log file paths, internal function calls, or intermediate state. They assert on: exit code, stdout decision JSON, stderr warnings, and file system outcomes (worktree exists/absent, task binding file contents).

**`harness-hook` command (extends `harness_hook_test.go`):**
- Pass-through when no task binding file and no env var (replaces `TestHarnessHookRequiresTaskID`)
- Pass-through when task binding file contains a task in `done` status
- Pass-through when task binding file contains a task in `merged` status
- Enforce scope when task binding file contains a task in `claimed` status
- Enforce scope when task ID is from env var (file absent) and task is active
- Pass-through is logged to `<git-dir>/armature-hook.log`
- Existing: blocks out-of-scope edit, allows in-scope edit, blocks stop when verification fails

**`claim` command (new tests alongside `claim.go`):**
- Fails when `--worktree` is omitted
- Creates worktree at the given path if it does not exist, on the branch derived from node type
- Story node creates worktree on `feat/<id>` branch
- Bug node creates worktree on `fix/<id>` branch off parent story branch
- Task node creates worktree on `task/<id>` branch off parent story branch
- Epic node returns an error (no worktree/branch supported)
- Writes task ID to `<git-dir>/armature-task-id` inside the new worktree
- Updates task ID file when worktree already exists (sequential task reuse)
- Claim op is still appended to the ops log

**`merged` command (new tests alongside `merged.go`):**
- Removes the task-scoped worktree for a task-type issue
- Removes the bug-scoped worktree for a bug-type issue
- Does not remove the worktree for a story-type issue
- Emits a warning to stderr when hook log contains pass-through entries
- Emits no warning when hook log is absent or contains no pass-throughs

Prior art: `harness_hook_test.go` uses `setupRepoWithTask`, `runTrls`, and `newRootCmd()` + `cmd.Execute()`. New tests should follow the same pattern.

## Out of Scope

- Per-task worktrees for sequential tasks. Sequential tasks share the parent story/feature worktree; per-task isolation for sequential tasks adds integration overhead with no governance benefit (see ADR-0003).
- Recording task binding events in the Armature ops log. Pass-throughs are logged locally to `.git/armature-hook.log` only.
- Auto-deriving the worktree path in `arm claim`. The coordinator always specifies the path explicitly.
- Epics getting worktrees or branches. Epics are coordination groupings above the dispatch level.
- Changing hook behavior for Codex or Devin process-isolation platforms. The `ARMATURE_TASK_ID` env var path remains fully supported as a fallback.
- `arm merged` removing story or feature worktrees. Story/feature worktree lifecycle is managed by the coordinator after all tasks in the story are complete.

## Further Notes

The governance model established here is the best achievable within the constraint that the coordinator is an LLM and cannot be the subject of its own governance. The design minimizes the coordinator's governance-sensitive responsibilities to a single required argument (`--worktree`) on a call it must already make (`arm claim`). See ADR-0003 for the full reasoning behind the worktree mandate and the alternatives considered (per-task worktrees, per-story with optional binding).

The `--print` mode / process-inversion model (external process manager launching Claude Code as a subprocess) was considered and rejected on market grounds: the required API spend mode is 10-100x more expensive than the subscription model, making it non-viable for the target addressable market.
