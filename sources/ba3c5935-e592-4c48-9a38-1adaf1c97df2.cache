# PRD: Coordinator Per-Wave Verification Gate

## Problem Statement

When the Armature coordinator dispatches parallel workers across multiple waves, defects often surface only after the wave is integrated on the story branch: compiler errors, cross-package signature mismatches, incomplete task transitions, and graph-health violations that `arm ready` alone does not catch. The coordinator currently has no explicit wave boundary check, so bad state can leak into the next wave or all the way to PR preparation.

The current failure is not just "missing build." It is missing wave accountability:

- the coordinator does not record the exact task set it dispatched for a wave
- the coordinator does not diff the integrated result against a known pre-wave base
- the coordinator does not run `arm doctor`, which catches lifecycle and commit mismatches that `arm validate` can miss
- remediation has no retry budget, so a bad repair loop can spin indefinitely

## Solution

Introduce an explicit per-wave verification gate in the coordinator skill plus a stricter worker handoff contract.

- Workers keep the existing repo policy that `make check` must be green before commit, and the worker skill makes that requirement explicit before a task is transitioned to `done`.
- The coordinator records a wave manifest before dispatch: exact task IDs, the story branch, and the pre-wave integration SHA.
- After the wave returns and merges cleanly, the coordinator runs an integration gate against the story branch using the recorded manifest, the integrated diff, `go build ./...`, a fast CI-safe check set, `arm validate --quiet`, and `arm doctor`.
- If the gate fails, the coordinator may launch a bounded remediation attempt with the failing command, raw output, wave diff, and task manifest. If the gate stays red after the retry budget is exhausted, the coordinator stops and escalates instead of looping forever.

The per-wave gate is intentionally lighter than the final pre-PR gate. Mutation testing remains part of the repo-wide final `make check`, not the per-wave loop.

## User Stories

1. As a coordinator agent, I want to record the exact task IDs and base SHA for each wave before dispatch, so that post-wave verification is scoped to the wave that actually ran.
2. As a coordinator agent, I want to verify that every dispatched task reached a terminal status before advancing, so that partial waves are never silently promoted.
3. As a coordinator agent, I want to run `go build ./...` after integrating a code wave, so that cross-package signature mismatches are caught on the shared branch.
4. As a coordinator agent, I want to run a fast integrated validation sequence after each wave, so that structural regressions are caught before the next dispatch.
5. As a coordinator agent, I want to run `arm doctor` after each wave, so that lifecycle and commit mismatches block advancement instead of surfacing at story close.
6. As a coordinator agent, I want to compute the changed-file set from the recorded pre-wave SHA, so that remediation receives the real integration surface instead of an ambiguous diff.
7. As a coordinator agent, I want a bounded remediation budget, so that repeated gate failures escalate instead of causing an infinite self-repair loop.
8. As a worker agent, I want the skill to state explicitly that `make check` must be green before I transition a task to `done`, so that I do not hand back unverified work.
9. As a worker agent, I want to run `go build ./...` before completion, so that compiler and signature regressions are caught even when tests miss them.
10. As an operator, I want a per-wave summary that names the exact tasks, gate profile, failures, remediation attempts, and final outcome, so that I can audit the orchestration without reconstructing it manually.
11. As an operator, I want docs-only and skill-only waves to use an honest reduced gate, so that the plan does not claim build coverage for changes that do not touch Go code.
12. As an operator, I want the coordinator to fall back to `go run ./cmd/armature ...` if the installed `arm` binary lacks a required flag, so that the wave gate uses the repo's current source-backed behavior.

## Implementation Decisions

- **Coordinator skill (`internal/skillsembed/skills/armature-coordinator/SKILL.md`) gains an explicit wave manifest step before dispatch.** For every wave, record:
  - `WAVE_TASK_IDS` in the order dispatched
  - `WAVE_BASE_SHA=$(git rev-parse HEAD)` before any worker output is integrated
  - the story branch name
  - whether the wave is expected to be `code` or `docs-skill-only`

- **Coordinator skill gains a "Wave Verification Gate" section after integration and before `arm ready`.** The coordinator must not infer wave success from the whole story state. It verifies the recorded `WAVE_TASK_IDS` specifically.

- **Terminal-state check is wave-scoped, not story-scoped.** Run `arm list --terminal --parent STORY-ID` and verify that every ID in `WAVE_TASK_IDS` appears in a terminal state. Any missing task or any task still outside `done`, `merged`, or `cancelled` fails the gate immediately.

- **The changed-file set is derived from the recorded wave base, not from an unqualified diff.** Compute remediation context with:
  ```bash
  git diff --name-only "$WAVE_BASE_SHA"..HEAD
  ```
  This is the only acceptable source for the wave's modified-file list. Bare `git diff --name-only` is ambiguous and must not be used in the plan.

- **Per-wave gate uses explicit profiles.**
  - `code` wave profile:
    1. `go build ./...`
    2. `make lint test coverage-check validate-skills build`
    3. `arm validate --quiet`
    4. `arm doctor`
  - `docs-skill-only` wave profile:
    1. if any file under `internal/skillsembed/skills/` changed, run `make validate-skills`
    2. `arm validate --quiet`
    3. `arm doctor`
  - If the wave was classified as `docs-skill-only` but the diff contains any `*.go`, `go.mod`, `go.sum`, `Makefile`, anything under `cmd/`, or anything under `internal/` outside `internal/skillsembed/skills/`, promote it to the `code` profile automatically.

- **Per-wave gate does not replace the repo-wide final gate.** After the last wave, before the story is committed, pushed, or turned into a PR, the coordinator still runs:
  ```bash
  make check
  arm validate --ci
  arm doctor
  ```
  Mutation testing stays here because `make check` already includes it, and the repo contract requires it before commit and push.

- **Worker skill (`internal/skillsembed/skills/armature-worker/SKILL.md`) gains a mandatory pre-transition checklist.** Before `arm transition ISSUE-ID --to done`, the worker must run:
  ```bash
  go build ./...
  make check
  ```
  If either command fails, the worker must not transition the task to `done`.

- **Worker completion order is made explicit.** The skill should say: verify first, then transition, then immediately stage the scoped files plus `.armature/` and commit. The plan must not imply that the worker can transition early and "commit later if convenient."

- **Coordinator remediation is bounded and foreground-only.** If any wave-gate step fails:
  - launch at most 2 automated remediation attempts for that wave
  - run them in a foreground session, not as a background agent
  - re-run the full gate after each attempt
  - if the gate is still red after attempt 2, stop and escalate to the operator

- **Remediation prompt contents are strict.** The remediation subagent must receive:
  - the failing command
  - the raw stderr/stdout from that command
  - `WAVE_TASK_IDS`
  - `WAVE_BASE_SHA`
  - `git diff --name-only "$WAVE_BASE_SHA"..HEAD`
  - the story branch name
  - the `armature-worker` skill reference
  - an edit-scope instruction: stay within the wave diff plus the smallest adjacent files required to make the gate green

- **TDD instruction is conditional, not ceremonial.** The remediation prompt must say:
  - if the failure is a real code-path defect with an honest test seam, write a failing test first
  - if the failure is a compile-time integration error, a graph-state problem, or a docs/skill deployment issue with no truthful failing-test seam, state why test-first is not applicable and use the smallest reproducible verification command instead

- **Source-backed `arm` fallback is documented.** If the installed `arm` binary does not support `--terminal`, `--quiet`, or other required flags, the coordinator should run the same commands via:
  ```bash
  go run ./cmd/armature <subcommand> ...
  ```
  The plan assumes current repo behavior, not stale global installs.

## Testing Decisions

- **This is a skill-and-doc change, not a Go-binary change.** No new Go production code or CLI flags are required for this story.

- **Structural verification:**
  - run `make validate-skills` after every skill edit
  - run `make skill` after the coordinator and worker skill edits
  - verify the updated content is deployed to `.claude/skills/`, `.gemini/skills/`, and `.codex/skills/`
  - before commit, run the repo-wide final check: `make check`

- **Deployment verification:** After `make skill`, grep the deployed coordinator and worker skills for:
  - `Wave Verification Gate`
  - `WAVE_BASE_SHA`
  - `make check`
  - `arm doctor`

- **Dogfood verification should exercise the failure modes this PRD is meant to stop.** Run at least one supervised two-task wave where:
  - one worker result introduces an integration-only compile break
  - the wave gate fails on `go build ./...`
  - remediation receives the wave diff derived from `WAVE_BASE_SHA`
  - the gate reruns after remediation
  - repeated failure after the retry budget causes escalation instead of another blind retry

- **State-health verification:** Run a second supervised case where a task in `WAVE_TASK_IDS` never reaches terminal state and verify the coordinator blocks advancement before `arm ready` is called for the next wave.

- **Prior art:** `2026-04-30-coordinator-friction-reduction.md` is still the right reference for skill deployment and the existing `arm list --terminal` / `arm validate --quiet` surfaces, but this story adds wave accounting and bounded remediation on top.

## Out of Scope

- Automatic merge-conflict resolution. If integration conflicts require judgment, the coordinator stops and escalates.
- Changes to the `arm` binary's `ready`, `validate`, `list`, or `doctor` implementations.
- Any attempt to weaken the repo rule that `make check` must be green before commit and push.
- Claim TTL or heartbeat policy changes.
- GitHub Actions pipeline changes.

## Further Notes

The important correction in this PRD is scope honesty. A wave gate that does not know which tasks ran, which files changed, or how many times it has already tried to self-repair is not a gate; it is wishful thinking. Recording `WAVE_TASK_IDS` and `WAVE_BASE_SHA`, running `arm doctor`, and bounding remediation turns the coordinator into a real control loop instead of a best-effort checklist.

`arm list --terminal --parent STORY-ID` remains the right primitive for completion checks because it includes tasks already transitioned to `merged`. The coordinator must still compare that output against the recorded wave manifest; story-level terminal output alone is not enough.
