# Provider Smoke Tests (Claude + Codex)

This runbook verifies that worker agents can claim, implement, and complete tasks
using the standard coordinator dispatch flow with Claude and Codex as the AI provider.

Devin is intentionally deferred for a later phase.

## Preconditions

- `arm` binary is built from current branch.
- `arm doctor` is green.
- Worker identity exists:
  - `arm worker-init --check || arm worker-init`
- A test story with at least one task exists with valid scope and acceptance criteria.
- The selected AI provider CLI (`claude` or `codex`) is installed and authenticated.

## Shared Coordinator Smoke

1. Confirm ready queue is not empty:
   - `arm ready`
2. Claim a task and render context:
   - `arm claim <TASK-ID>`
   - `arm render-context <TASK-ID> --format agent`
3. Verify context output includes:
   - `task_id`
   - `description`
   - `scope`
   - `acceptance`

## Dogfood Worktree Posture

- Run provider dogfood tasks from a disposable branch or linked worktree.
- After each live run, inspect `git status --short` before pushing. Provider
  runtime state may remain on disk but must not be committed.
- Only stage files from the task's declared `scope` plus `.armature/`.

## Claude Provider Smoke

1. Claim a test task and render context:
   - `arm claim <TASK-ID>`
   - `arm render-context <TASK-ID> --format agent`
2. Dispatch Claude Code with the render-context output as the task spec.
3. Worker agent implements the task using the `armature-worker` skill.
4. Verify:
   - task transitions to `done` via `arm transition`
   - commit message follows `<type>(<TASK-ID>): <description>` format
   - `.armature/` ops are staged alongside code files
   - no out-of-scope files in the commit diff

## Codex Provider Smoke

1. Claim a test task and render context:
   - `arm claim <TASK-ID>`
   - `arm render-context <TASK-ID> --format agent`
2. Dispatch Codex with the render-context output as the task spec.
3. Worker agent implements the task using the `armature-worker` skill.
4. Verify:
   - task transitions to `done` via `arm transition`
   - commit message follows `<type>(<TASK-ID>): <description>` format
   - `.armature/` ops are staged alongside code files
   - no out-of-scope files in the commit diff

## Evidence Capture

Record these in task notes or PR description:

- command lines used
- exit codes
- final issue status
- environment notes (OS, shell, provider versions)

## Known Follow-Up

- Add Devin smoke section after Devin support is enabled.
