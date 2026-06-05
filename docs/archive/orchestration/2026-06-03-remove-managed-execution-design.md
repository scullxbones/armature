# Remove Managed Execution Design

Date: 2026-06-03

## Summary

Armature will stop launching and supervising AI harnesses. The `arm orchestrate`
and `arm worker run` command surfaces, their supporting execution modules, their
configuration, and their op vocabulary will be removed in a backward-incompatible
transition.

Armature remains responsible for coordination truth: the task DAG, ready work,
claims, working memory, source traceability, explicit lifecycle transitions, and
validation. External workers remain responsible for implementation, repository
verification, commits, and reporting outcomes.

This removal does not abandon harness integration. Armature may expose
task-aware enforcement commands that external harnesses invoke through their
native hook systems. In particular, planned HKREFACT work may add
`arm harness-hook` so a harness can resolve the active task from Armature state
and block out-of-scope edits before they occur.

## Decision

Armature is a coordination kernel and policy authority, not a harness runtime.

The ownership rule is:

> Armature does not execute or supervise harnesses. Armature may expose
> enforcement commands that harness-native hooks invoke.

This rule separates two kinds of harness interaction:

- **Removed managed execution:** Armature chooses a harness, resolves its auth,
  launches it, supervises it, retries it, mutates git state, creates commits, or
  infers task lifecycle outcomes from harness behavior.
- **Retained policy enforcement:** An externally launched harness calls an
  `arm` command through its native hook system. Armature resolves task truth
  from the DAG and returns an allow/block/advisory decision.

No replacement execution or handoff command will be added in this transition.
The existing skills and explicit commands form the prototype workflow.

## Goals

- Delete managed harness execution and the worker runtime completely.
- Restore one task lifecycle expressed through ordinary Armature ops.
- Recommend the already delivered coordinator and worker skills as the
  prototype execution workflow.
- Preserve a clear path for harness-native hooks to enforce DAG policy.
- Remove obsolete orchestration op types from production code and dogfood logs.
- Archive abandoned orchestration design documents without presenting them as
  current architecture.
- Keep the dogfood graph structurally valid and source-backed after migration.

## Non-Goals

- Do not add a replacement `run`, `execute`, `dispatch`, or handoff command.
- Do not retain compatibility parsing for removed orchestration/runtime ops.
- Do not add a production migration command for external repositories.
- Do not launch harnesses to prove hook enforcement.
- Do not make Armature responsible for external worker commits or verification.
- Do not remove generic git-hook infrastructure or planned harness-hook policy
  modules.
- Do not solve all HKREFACT work during the managed-execution removal.

## Target Workflow

### Coordinator Skill

The `armature-coordinator` skill owns execution flow through explicit commands:

1. Inspect the DAG with `arm list`, `arm ready`, `arm validate`, and
   `arm doctor`.
2. Select independent work and assign log slots when parallel execution is
   appropriate.
3. Claim each task with `arm claim`.
4. Render canonical working memory with `arm render-context`.
5. Dispatch an external worker through the host platform's agent capability.
6. Integrate worker commits and mark completed tasks merged.
7. Re-run `arm validate --ci` and `arm doctor`.

### Worker Skill

The `armature-worker` skill is the only recommended task-execution skill:

1. Receive a pre-claimed task and rendered working memory.
2. Activate available harness-native Armature hooks when supported.
3. Implement the task using the external harness's normal execution model.
4. Record notes, decisions, and heartbeats through explicit Armature commands.
5. Run repository verification directly.
6. Transition the task with an explicit outcome.
7. Commit only scoped code changes.

### Harness-Native Enforcement

Future HKREFACT work may configure external harnesses to call:

```sh
arm harness-hook
```

The hook entrypoint resolves task truth from Armature state and evaluates the
harness's proposed action. It does not launch, supervise, retry, or complete the
harness run.

The active task identity is supplied by the external worker environment:

```sh
ARMATURE_TASK_ID=<task-id>
ARMATURE_HOOK_PLATFORM=claude|codex|devin
```

The coordinator/worker skills or an explicit future hook-installation command
may configure these environment values and platform-native hook files. That
configuration capability must remain separate from harness process launch.

## Architecture Boundary

### Retained Coordination Modules

- `internal/ops`: ordinary task and source ops.
- `internal/materialize`: derived task state and DAG projections.
- `internal/ready`: ready-task computation and diagnostics.
- `internal/claim`: claim and scope-overlap mechanics.
- `internal/context`: working-memory assembly and rendering.
- `internal/validate`, `internal/doctor`, and `internal/audit`: structural and
  operational checks.
- Existing git-hook infrastructure under `cmd/armature/init.go`,
  `cmd/armature/hook.go`, and `internal/hooks`.
- Planned `internal/harnesspolicy` and `internal/harnesshook` modules.
- Planned `arm harness-hook` command and platform hook adapters.

### Removed Execution Modules

- `cmd/armature/orchestrate.go` and its tests.
- `cmd/armature/worker_run.go` and its tests.
- The empty `arm worker` command group after `worker run` is removed.
- `internal/orchestrate`.
- `internal/workerruntime`.
- Harness launch adapters and harness auth/model resolution.
- Harness sandbox/config generation used to launch managed processes.
- Managed-execution retry, phase, heartbeat, diagnostics, and lifecycle logic.
- Zero-trust diff/reset/reapply/stage/commit behavior.

### Removed Configuration

Remove the `orchestrator` section from repository configuration, including:

- default model
- auth mode and env file
- harness adapter commands
- sandbox and parallelism settings

Hooks that need policy configuration must define their own narrow configuration
surface when HKREFACT implements them. They must not reuse an
execution-oriented `orchestrator` configuration object.

### Removed Op Vocabulary

Delete these op types:

- `orchestrate-start`
- `orchestrate-dispatch`
- `orchestrate-dispatch-complete`
- `orchestrate-verify-fail`
- `orchestrate-retry`
- `orchestrate-escalate`
- `orchestrate-complete`
- `orchestrate-check-result`
- `worker-runtime-decision`

Delete payload fields used only by these ops:

- `worktree_path`
- `pre_dispatch_ref`
- `retry_budget`
- `run`
- `correlation_id`
- `causation_id`
- `decision_class`

Materialization must reject these removed op types as unknown after the dogfood
logs are migrated.

## HKREFACT Preservation And Re-Scope

The HKREFACT epic remains active because harness-native enforcement strengthens
the DAG without making Armature a harness runtime.

### Retain

- **HKREFACT-T01 Shared Scope Policy:** canonical scope evaluation for hook and
  command consumers.
- **HKREFACT-T03 Generic Harness Hook Types and Evaluator:** generic
  pre-tool/stop event evaluation.
- **HKREFACT-T04 Platform Hook Adapters:** decode and encode native hook
  payloads for supported harnesses.
- **HKREFACT-T05 Task Policy Resolver:** resolve task scope and policy from
  materialized Armature state.
- **HKREFACT-T06 `arm harness-hook` CLI:** hook entrypoint invoked by external
  harnesses.
- **HKREFACT-T11 Documentation and Provider Smoke Tests:** rewritten around
  independently launched harnesses.
- **HKREFACT-T12 Full Verification:** validates hook policy and the repository,
  not managed execution.

### Re-Scope

- **HKREFACT-T02 Shared Verification Service:** move outside
  `internal/orchestrate`. It may provide reusable verification for hook stop
  events or explicit commands, but it does not decide task completion.
- **HKREFACT-T07 Harness Launch Integration:** rename to **Harness Hook
  Installation and Configuration**. It generates or installs platform-native
  hooks without launching a harness.
- **HKREFACT-T08 Final Shared Scope Verification in Zero-Trust Commit:** replace
  with shared scope enforcement for harness hooks and existing git pre-commit
  hooks. There is no Armature-owned final commit path.

### Cancel As Obsolete

- **HKREFACT-T09 Wire VerificationService Into Engine Completion**
- **HKREFACT-T10 Pass Verification Inputs From Commands**

Completed HKREFACT tasks remain historical evidence even when their delivered
managed-execution behavior is deleted.

## Dogfood State Migration

There is no external compatibility requirement. The internal dogfood state will
be rewritten directly.

### Ops Logs

Remove only lines whose op type is in the removed op vocabulary. Current
dogfood inspection found removed ops in two worker logs.

Retain:

- creates and amends
- claims and heartbeats
- notes and decisions
- source links and citation acceptances
- ordinary lifecycle transitions
- completed orchestration-related tasks as historical evidence

After editing the logs:

1. Delete local materialized state and checkpoints.
2. Materialize from the rewritten logs using the new binary.
3. Confirm no removed op vocabulary remains.
4. Run `arm validate --ci`.
5. Run `arm doctor`.

### Graph Changes

- Keep HKREFACT active and apply the retain/re-scope/cancel classification
  above.
- Cancel active ORCRUN and worker-runtime implementation work with an explicit
  outcome that managed execution was removed after dogfooding.
- Do not delete historical task nodes.
- Update scopes and dependencies that reference deleted execution modules.

### Source Registrations

Retained source registrations and source links must point to their new archive
paths after documents move. Source caches and fingerprints must be synchronized
and verified after the move.

## Documentation Archive

Move abandoned orchestration/runtime records under:

```text
docs/archive/orchestration/design/
docs/archive/orchestration/specs/
docs/archive/orchestration/plans/
```

Archive:

- orchestration runtime direction, policy, state-machine, audit, roadmap, and
  run-contract documents
- orchestration/runtime specs
- orchestration/runtime implementation plans
- managed-execution auth and provider-run documentation

Do not archive the HKREFACT hook design and plan. Rewrite them in place so they
describe external harness ownership and hook-native enforcement.

Add `docs/archive/orchestration/README.md` explaining:

- the documents are retained as dogfood history
- managed execution was abandoned because harness behavior and inversion of
  control created excessive complexity
- current architecture uses explicit coordination commands and skills
- harness-native policy enforcement remains an active direction

Current docs must not recommend `arm orchestrate`, `arm worker run`, managed
harness execution, or an orchestrator fleet.

## Skills

### `armature`

Update the command reference to list explicit coordination commands only.
Remove managed execution commands and fallback terminology.

### `armature-coordinator`

Make explicit command-driven dispatch the default workflow. Remove runtime-loop
supervision. Teach hook activation as an optional enforcement step before
dispatching a worker.

### `armature-worker`

Remove "manual fallback" and orchestrator-first language. Make the skill the
default worker execution procedure. Teach workers to use available
harness-native hooks but remain responsible for verification, transitions, and
commits.

### `armature-orchestrator`

Delete this skill. No current skill should imply that Armature owns harness
execution.

## Error Handling

- Removed commands return Cobra's normal unknown-command error.
- Removed ops in any unmigrated log cause validation/materialization failure;
  there is no compatibility fallback.
- `arm harness-hook` must fail closed for side-effecting hook events when task
  policy cannot be resolved.
- Hook failures must not transition tasks or create execution lifecycle ops.
- External workers report blocked work explicitly through normal notes and
  transitions.

## Testing Strategy

### Deletion Tests

- Root command cannot resolve `orchestrate`.
- Root command cannot resolve `worker` or `worker run`.
- Configuration round-trip has no orchestrator configuration.
- Ops parser/validation rejects removed op names.
- Materialization has no removed-op no-op branch.
- Repository search finds no production imports of `internal/orchestrate` or
  `internal/workerruntime`.

### Retained Workflow Tests

- Ready, claim, render-context, heartbeat, note, decision, transition, merged,
  validate, and doctor tests remain green.
- Skill installation deploys coordinator and worker skills but not an
  orchestrator skill.
- Skill content recommends explicit command-driven dispatch.

### Hook Direction Tests

The managed-execution removal does not need to implement HKREFACT. It must
ensure:

- the revised HKREFACT source documents do not depend on managed execution
- planned hook modules and `arm harness-hook` remain valid future work
- no removal task deletes generic hook or task-policy infrastructure

### Full Verification

The hard transition is complete when:

```sh
go test ./...
make check
go run ./cmd/armature validate --ci
go run ./cmd/armature doctor
```

all pass against migrated dogfood state.

Repository searches must also confirm:

- no current docs or skills recommend managed execution
- no production code references removed commands, modules, config, or ops
- removed op names do not exist in dogfood logs
- orchestration documents exist only under the archive, except the revised
  current HKREFACT hook design and plan

## Consequences

Armature gives up built-in autonomous queue draining and single-task harness
execution. The prototype workflow becomes more explicit and relies on host
platform skills for dispatch.

In exchange, Armature removes a large unstable execution surface, restores a
single task lifecycle, and concentrates its implementation around coordination
truth. Harness-native hooks can still make the DAG enforceable at the point of
action without requiring Armature to control the harness process.
