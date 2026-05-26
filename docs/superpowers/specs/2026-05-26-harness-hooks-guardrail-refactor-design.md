# Harness Hooks Guardrail Refactor Design

Date: 2026-05-26

## Summary

Armature will refactor harness guardrails so platform hooks become the primary
implementation for controls that the supported harnesses can enforce before a
tool executes. The refactor must preserve existing capability while removing
overlapping custom guardrail implementations. Where hooks and final verification
both enforce the same rule, they must call the same shared Go logic rather than
duplicating policy.

Supported harness platforms:

- Claude Code: https://code.claude.com/docs/en/hooks
- Codex: https://developers.openai.com/codex/hooks
- Devin CLI: https://cli.devin.ai/docs/extensibility/hooks/overview

## Goals

- Move task-aware harness controls into ephemeral per-run hook configuration.
- Keep the hook CLI surface minimal and derive task facts from Armature state.
- Use `arm` itself as the hook executable.
- Replace overlapping custom guardrails instead of retaining duplicate controls.
- Preserve defense-in-depth only when all layers share the same policy logic.
- Retain non-overlapping orchestration controls in Armature.
- Validate OS sandbox governance before removing or narrowing `bwrap` and
  `sandbox-exec`.

## Non-Goals

- Do not introduce broad shell safety policy such as generic `rm -rf`, secret
  scanning, package-install governance, network egress control, or arbitrary
  allow/deny command lists.
- Do not pass task scope, acceptance criteria, or citation data as hook command
  arguments.
- Do not create shell scripts as the hook implementation. Generated hook config
  must call the `arm` executable.
- Do not maintain separate path/scope matching implementations for hooks and
  final verification.

## Hook CLI Contract

Generated platform hook config will call:

```sh
arm harness-hook
```

The hook command reads the platform hook event JSON from stdin. The command has
no scope, acceptance, citation, or issue metadata flags.

The ephemeral harness launch environment supplies the active task identity:

```sh
ARMATURE_TASK_ID=<task-id>
ARMATURE_HOOK_PLATFORM=claude|codex|devin
```

`ARMATURE_HOOK_PLATFORM` is optional if the hook input format can be detected
reliably, but generated config should set it to keep output encoding explicit.

`arm harness-hook` resolves all task data from Armature state:

- Repository root and Armature context from the hook cwd.
- Materialized state, refreshing from ops when needed.
- Task scope from the DAG model.
- Acceptance criteria and citation state from the task/source model.
- Active harness policy from Armature configuration.

This avoids drift between generated hook configuration and the DAG.

## Replacement Boundary

### Controls Moved to Platform Hooks

Pre-tool hooks become the primary implementation for task-aware controls that
can be decided before side effects:

- Block file edits outside the active task scope.
- Block direct `git commit` during harness execution.
- Block direct staging/commit-path commands when they bypass Armature's
  zero-trust commit semantics.
- Provide task-aware feedback when a tool request is denied.

Stop hooks become the primary early verification trigger:

- Invoke the shared verification service before the harness stops.
- Block stopping when verification fails, where the platform supports blocking
  stop behavior.
- Return model-visible feedback derived from shared verification results.

### Controls Retained in Armature

These controls are not overlapping harness guardrails and remain in Armature:

- Ready queue computation.
- Claim ownership and claim-race handling.
- Active scope overlap scheduling gates.
- Retry budgets and retry-loop guards.
- Heartbeats and stale-claim handling.
- Runtime timeout and max-task loop control.
- DAG state transitions and op-log persistence.

### Defense-in-Depth Controls

Final commit verification remains acceptable because committing or pushing
out-of-scope files is sufficiently incorrect to justify a second trigger. The
final trigger must call the same shared scope policy used by hooks.

The zero-trust commit path remains responsible for commit construction:

1. Capture diff from the pre-dispatch ref.
2. Reset to the pre-dispatch ref.
3. Reapply the captured patch.
4. Verify the changed paths through shared scope policy.
5. Stage and commit through Armature.

The final scope verification is not a second implementation. It is a second
trigger of the same implementation.

### OS Sandbox Governance

Do not remove `bwrap` or `sandbox-exec` as part of the first refactor step.
They are powerful process-governance tools and are not necessarily equivalent
to platform hooks.

The validation step must classify current sandbox behavior into:

- Task-aware write-scope enforcement that overlaps with hooks.
- Process isolation or non-hook-visible write governance that hooks do not
  replace.

Only overlapping behavior should be removed. Non-overlapping governance should
remain until an equivalent replacement exists. This is especially important for
Codex because its hook documentation describes `PreToolUse` as an incomplete
guardrail for shell interception.

## Dangerous Shell Commands

Armature will not add general destructive-command policy in this refactor.

In scope:

- Blocking `git commit` during harness execution.
- Blocking direct staging/commit commands when they attempt to bypass the
  Armature-owned commit path.
- Blocking shell commands that clearly target writes outside task scope only
  when the platform exposes enough structured command data and the decision can
  use shared scope policy.

Out of scope:

- Generic destructive command blocking.
- Secrets scanning.
- Network policy.
- Package manager governance.
- Organization-specific command allowlists or denylists.

## Verification Pipeline

Armature already has verification concepts in `internal/orchestrate/verify.go`:

- Ordered adapter checks.
- Stop-on-hard-failure semantics.
- Warning checks that do not halt the pipeline.
- Acceptance criteria verification.
- Citation verification.

The live engine path must be reconciled with this shared verification pipeline.
Current code comments describe a `RunPipeline` step in the zero-trust sequence,
but the engine path currently records the harness result and proceeds to commit.
The refactor should consolidate this into a single `VerificationService` used
by both:

- Harness stop hooks.
- The orchestrator before final commit completion.

Hook-triggered verification may provide faster feedback, but final
orchestrator-triggered verification remains the authoritative completion gate.
Both triggers use the same service and result model.

## Architecture

### TaskPolicyResolver

Responsibilities:

- Load repository and Armature context from cwd.
- Materialize state if needed.
- Resolve `ARMATURE_TASK_ID`.
- Load task scope, acceptance criteria, source/citation state, and relevant
  config.
- Return immutable task policy input to hook and verification components.

### ScopePolicy

Responsibilities:

- Normalize repository-relative paths.
- Match changed paths against task scope.
- Support glob and directory semantics consistent with existing task scope
  behavior.
- Classify violations with actionable messages.

Consumers:

- `arm harness-hook` pre-tool evaluation.
- Zero-trust final changed-path verification.
- Any future scope-aware command policy.

There must be one implementation and one test suite for this logic.

### HarnessHookEvaluator

Responsibilities:

- Convert platform hook input to generic events:
  - `PreToolUse`
  - `Stop`
  - `PostToolUse` if needed for advisory feedback
- Extract candidate file paths or command intent.
- Call `ScopePolicy` and `VerificationService`.
- Return a generic decision:
  - allow
  - block with reason
  - add context
  - no decision

### PlatformHookEncoder

Responsibilities:

- Encode generic decisions for Claude Code.
- Encode generic decisions for Codex.
- Encode generic decisions for Devin CLI.
- Keep platform differences isolated to input/output shape, exit-code behavior,
  and matcher naming.

The encoder must not contain scope matching, task lookup, or verification logic.

### VerificationService

Responsibilities:

- Run configured build/lint/test/coverage/mutation checks.
- Run acceptance criteria checks.
- Run citation checks.
- Preserve ordered check results and stop-on-hard-failure behavior.
- Produce results usable by both hook feedback and orchestration state.

## Platform Mapping

### Claude Code

Use ephemeral project-local hook config for:

- `PreToolUse` matching edit/write tools and shell tools needed for commit
  blocking.
- `Stop` for verification before the agent stops.

Claude supports blocking `PreToolUse` and `Stop` events. Denials should provide
model-visible reasons that tell the harness which task scope or verification
rule failed.

### Codex

Use ephemeral project-local Codex hook config for:

- `PreToolUse` matching `apply_patch`, `Edit`, or `Write`.
- `PreToolUse` matching supported `Bash` calls for direct commit blocking.
- `Stop` for shared verification before the turn ends.

Codex documentation states that `PreToolUse` is a guardrail rather than a
complete enforcement boundary because shell interception and equivalent tool
paths are incomplete. Therefore:

- Do not remove OS sandbox governance solely because Codex hooks exist.
- Keep final shared scope verification before commit.
- Treat Codex shell command blocking as best-effort unless a specific command
  path is proven visible to hooks.

### Devin CLI

Use ephemeral project-local Devin hook config for:

- `PreToolUse` matching `edit`.
- `PreToolUse` matching `exec` for direct commit blocking.
- `Stop` for verification before the agent stops.

Devin uses a Claude-compatible hook shape, but output encoding should still be
kept behind the platform encoder so future differences remain localized.

## Data Flow

1. `arm orchestrate` or `arm worker run` selects a task using existing Armature
   scheduling logic.
2. Armature records dispatch state and pre-dispatch ref.
3. Armature writes ephemeral platform hook config into the harness workdir.
4. Armature launches the harness with `ARMATURE_TASK_ID` and
   `ARMATURE_HOOK_PLATFORM`.
5. The harness proposes a tool call.
6. The platform invokes `arm harness-hook` with event JSON on stdin.
7. `arm harness-hook` resolves task state from Armature.
8. `HarnessHookEvaluator` calls shared policy/services.
9. `PlatformHookEncoder` returns the platform-specific decision.
10. If allowed, the tool proceeds. If blocked, the harness receives the reason.
11. On stop, the hook path invokes `VerificationService` and blocks or allows
    stopping based on results.
12. After harness exit, Armature performs final verification and zero-trust
    commit using the same shared policy/services.

## Error Handling

- Missing `ARMATURE_TASK_ID`: block with a clear configuration error.
- Unknown task ID: block with a clear state resolution error.
- State materialization failure: block, because policy cannot be evaluated.
- Unparseable hook input: block for `PreToolUse`; fail closed for events that
  control side effects.
- Unsupported platform event: no decision unless the event is expected for a
  configured matcher, in which case return a configuration error.
- Verification command failure: return structured failure results and block
  completion when severity is error.
- Hook timeout: configure short, explicit hook timeouts for pre-tool checks;
  verification stop hooks may use longer timeouts aligned with project config.

## Testing Strategy

Unit tests:

- `TaskPolicyResolver` resolves task data from materialized state.
- `ScopePolicy` accepts in-scope paths and rejects out-of-scope paths.
- `ScopePolicy` handles globs, directories, path cleaning, and path traversal.
- `HarnessHookEvaluator` blocks out-of-scope edits.
- `HarnessHookEvaluator` blocks direct `git commit`.
- `HarnessHookEvaluator` does not block unrelated shell commands outside the
  defined Armature policy.
- `VerificationService` preserves current pipeline ordering and failure
  semantics.
- Platform encoders produce expected Claude, Codex, and Devin outputs.

Integration tests:

- Generated Claude config calls `arm harness-hook`.
- Generated Codex config calls `arm harness-hook`.
- Generated Devin config calls `arm harness-hook`.
- A simulated pre-edit event outside task scope is blocked before execution.
- A simulated pre-edit event inside task scope is allowed.
- A simulated direct `git commit` event is blocked.
- Stop-hook verification invokes `VerificationService`.
- Orchestrator final verification invokes the same `VerificationService`.
- Final commit path rejects an out-of-scope diff through shared `ScopePolicy`.

Regression tests:

- Hook CLI has no scope or acceptance flags.
- Hook command derives scope from DAG state.
- Hook and final commit scope decisions remain identical for the same path set.
- OS sandbox behavior is not removed before classification tests document what
  it currently enforces.

## Migration Plan

1. Extract shared `ScopePolicy` from existing scope/path behavior.
2. Add final changed-path verification in the zero-trust commit path using
   `ScopePolicy`.
3. Consolidate verification into `VerificationService` and wire the live engine
   to it.
4. Add `arm harness-hook` with generic hook evaluation and platform encoders.
5. Generate ephemeral hook config for Claude, Codex, and Devin harness runs.
6. Remove overlapping harness config writes that implement task write scope
   directly, after equivalent hook behavior is tested.
7. Classify `bwrap` and `sandbox-exec` behavior before removing or narrowing
   any OS sandbox usage.
8. Update documentation and provider smoke tests.

## Open Implementation Notes

- The hook config location should be platform-native and ephemeral. Generated
  files should either be overwritten per run or written under an Armature-owned
  temporary harness directory if the platform supports loading from there.
- The hook path must avoid creating persistent user-level hook configuration.
- Hook trust/review behavior differs by platform and must be covered in provider
  smoke tests.
- If a platform cannot guarantee a hook is active for a control, the control
  cannot be considered replaced for that platform.

