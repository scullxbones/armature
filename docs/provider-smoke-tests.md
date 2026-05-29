# Provider Smoke Tests (Claude + Codex)

This runbook verifies live harness execution paths for `arm orchestrate` and
runtime command behavior for `arm worker run`.

Devin is intentionally deferred for a later phase.

## Preconditions

- `arm` binary is built from current branch.
- `arm doctor` is green.
- Worker identity exists:
  - `arm worker-init --check || arm worker-init`
- A test issue exists with valid scope and acceptance criteria.
- Required platform sandbox tools are present (`bwrap`/`socat` on Linux or
  `sandbox-exec` on macOS).

## Shared Runtime Smoke

1. Dry-run queue loop:
   - `arm worker run --max-tasks 1 --dry-run --format json`
2. Live queue loop:
   - `arm worker run --max-tasks 1 --format json`
3. Verify output fields:
   - `tasks_completed`
   - `final_state`
   - `max_tasks`
   - `dry_run`

## Claude Harness Smoke

1. Validate adapter path:
   - `arm orchestrate --issue <ISSUE-ID> --harness claude --dry-run --show-network-plan`
2. Validate auth paths:
   - API key mode: set `ANTHROPIC_API_KEY`, run dry-run again.
   - OAuth/session mode: `claude auth status`, then run dry-run with API key unset.
3. Run live single-task orchestration:
   - `arm orchestrate --issue <ISSUE-ID> --harness claude --timeout 900`
4. Verify:
   - command exits 0 on success
   - task transitions as expected
   - no sandbox/preflight errors

## Codex Harness Smoke

1. Validate adapter path:
   - `arm orchestrate --issue <ISSUE-ID> --harness codex --dry-run --show-network-plan`
2. Validate auth paths:
   - API key mode: set `OPENAI_API_KEY`, run dry-run again.
   - OAuth/session mode: `codex login status`, then run dry-run with API key unset.
3. Run live single-task orchestration:
   - `arm orchestrate --issue <ISSUE-ID> --harness codex --timeout 900`
4. Verify:
   - command exits 0 on success
   - task transitions as expected
   - no sandbox/preflight errors

## Evidence Capture

Record these in task notes or PR description:

- command lines used
- exit codes
- final issue status
- any escalation output
- environment notes (OS, shell, adapter versions)

## Known Follow-Up

- Add Devin smoke section after Devin harness path is enabled in runtime policy.
