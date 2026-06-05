# Orchestrate Auth and Disclosure Hardening (Pre-Orchestrate Usability)

## Goal
Make `arm orchestrate` usable without hidden auth failures by supporting both API-key and OAuth/session-based credentials, and by surfacing clear pre-dispatch network/auth disclosures.

## Problem
- Users currently hit runtime `401` failures from wrapped harness CLIs with little preflight guidance.
- Subscription users relying on CLI OAuth/session login (not API keys) are not first-class in orchestration auth handling.
- Escalation prompts and dry-run output do not provide enough detail to assess network/data exposure.

## Non-Goals
- Reworking the full orchestrator state machine.
- Changing existing scope/sandbox enforcement behavior.
- Introducing provider-specific secret storage beyond environment/session integration.

## Deliverables
1. Auth resolution layer for harness invocation:
   - `api-key` path (env or env-file).
   - `oauth-session` path (reuse existing CLI login/session).
   - `auto` mode with deterministic fallback order.
2. Preflight auth check that fails before dispatch with actionable remediation.
3. `--show-network-plan` style disclosure output from `arm orchestrate`.
4. Docs updates for setup and smoke verification for both auth modes.
5. Tests covering auth resolution, preflight failures, and disclosure output.

## Implementation Plan
1. Extend config model:
   - Add orchestrator auth config in `internal/config/config.go`.
   - Support modes: `auto`, `inherit-env`, `env-file`, `oauth-session`.
2. Add auth resolver package:
   - New `internal/orchestrate/auth.go` (or `internal/orchestrate/auth/`).
   - Resolve selected harness auth source and return redacted diagnostics.
3. Wire preflight:
   - Extend `internal/orchestrate/preflight.go` input/result to include auth checks.
   - Ensure failure occurs pre-dispatch with explicit remediation commands.
4. Add disclosure output:
   - Add orchestrate flag (e.g. `--show-network-plan`) in `cmd/armature/orchestrate.go`.
   - Print: harness, provider endpoint family, auth source type, outbound data classes, mutation/commit intent.
5. Update harness launch path:
   - Inject resolved env/session context into `internal/orchestrate/harness.go` launch logic.
   - Preserve existing sandbox behavior.
6. Tests:
   - `cmd/armature/orchestrate_test.go` for new flags and output.
   - `internal/orchestrate/preflight_test.go` for auth branch coverage.
   - `internal/orchestrate/harness_test.go` for resolver + launch environment behavior.
7. Documentation:
   - Update `docs/commands.md` orchestrate section.
   - Update `docs/provider-smoke-tests.md` with API-key and OAuth/session paths.

## Acceptance Criteria
1. `arm orchestrate --issue <ID> --dry-run --show-network-plan` prints auth source and network disclosure without exposing secrets.
2. `arm orchestrate` fails pre-dispatch when required auth is unavailable, with remediation guidance per harness.
3. API-key users continue to work without migration changes.
4. OAuth/session-only users can orchestrate when their harness CLI is logged in.
5. Unit tests for new auth/disclosure behavior pass in CI.

## Legacy Execution Mode (Next Step)
After this plan is accepted and injected, implementation proceeds in manual/legacy flow (pre-orchestrate automation), with direct code edits + test runs instead of `arm orchestrate` task execution.
