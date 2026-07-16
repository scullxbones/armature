# Orchestrate Runtime V1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the v1 deterministic worker runtime in incremental slices that can be dogfooded early: first by restoring a clean imperative-shell / functional-core boundary, then by shipping a one-task `arm worker run`, then by expanding it into a queue-draining runtime with value-tiered runtime records and conservative recovery.

**Architecture:** Start with a precursor refactor that restores an imperative-shell, functional-core boundary around orchestration and adjacent runtime seams. Extract pure transition and decision logic from imperative code, push git, op-log, clock, process, filesystem, and config interactions behind explicit adapters or narrow ports, and make the CLI layer a thin shell over application services. Then build `internal/workerruntime` as a deterministic runtime core that wraps cleaned `internal/ready`, `internal/claim`, and `internal/orchestrate` ports. Runtime event persistence is deliberately value-tiered: durable ops are reserved for shared task truth, accountability, and cross-run decisions; checkpoint/snapshot records are bounded and discardable; loop telemetry stays ephemeral. Ship the runtime in vertical slices so we can dogfood it after the one-task loop and again after the repeat-until-empty loop, instead of waiting for the entire Phase 4 control model.

**Tech Stack:** Go, Cobra, Armature append-only ops, materialization, existing ready/claim/orchestrate packages, `go test`, `make check`

---

## Implementation Requirements

TDD is mandatory for this plan. For every code change, write the focused
failing test first, run it and confirm it fails for the intended reason, make
the smallest implementation change, then rerun the focused test and confirm it
passes. Focused `go test ... -run ...` commands are the red/green inner loop
only; they are never sufficient as a pre-commit gate.

Every commit step must run `make check` immediately before `git commit`, and it
must be clean. Do not commit after only selective tests, even when the task's
focused tests pass. If `make check` fails, fix the failure and rerun `make
check` until it passes before committing.

## Dogfood Strategy

This plan is intentionally sequenced so we can use the runtime before all v1
features land. Milestones name the installed user-facing command as
`arm worker run`; implementation steps use `go run ./cmd/armature worker run`
so dogfood checks exercise the just-built code even before a fresh binary is
installed.

### Dogfood Milestone A

After Task 0, the architecture should be cleaner but the user-facing workflow
is still the existing `arm orchestrate --issue`.

### Dogfood Milestone B

After Task 3, `arm worker run --max-tasks 1` should be usable for one real task
at a time. This is the first external dogfood checkpoint and should be used in
daily work immediately.

### Dogfood Milestone C

After Task 5, `arm worker run` should replace the manual
`ready -> orchestrate -> repeat` shell loop for ordinary queue draining.

### Dogfood Milestone D

After Task 7, durable decision records, bounded runtime snapshots, stop handling,
and richer recovery should make the runtime trustworthy for longer-lived
operation without filling the long-lived op log with loop noise.

## Coordination And Runtime Record Strategy

The runtime must ruthlessly protect Armature's durable, long-lived log from
low-value runtime churn. The permanent op log is for shared project truth and
accountability, not for every poll, idle delay, local pause, cooldown, or claim
miss observed by a worker process.

Runtime records are split into three concrete sinks:

- **Ops log**: durable append-only repo-visible records needed to explain shared
  task state, coordination authority, human accountability, or cross-run
  decisions. Examples: claim acquisition/release, human escalation
  created/resolved, policy decision that changes execution lane or claim posture,
  durable worker-run summary, and any runtime decision that changes committed
  task state beyond existing claim/orchestrate ops.
- **Runtime snapshot**: bounded state useful for resume or local coordination but
  not worth preserving forever. Examples: local pause, checkpointed stop context,
  worker-scoped cooldown, retry timer, and in-flight execution context. Snapshot
  records may be compacted, overwritten, or discarded once expired or superseded.
- **Trace output**: loop telemetry and process narration. Examples: poll started,
  no-ready-work, claim loss processed, idle timer fired, heartbeat-like liveness,
  and debug traces. These must not enter the durable op log by default.

A record may enter the durable ops log only if it answers at least one of:

- did this worker gain, lose, or release authority to act?
- did shared task or project state change?
- did a human-accountable decision happen?
- did a policy decision impose a shared constraint future workers must honor?
- is this the minimal run summary needed to explain outcomes without replaying
  ephemeral traces?

If the answer is no, the record must be snapshot-tier or trace output. Claims
remain durable coordination writes because they establish temporary authority in
a distributed, git-native system. They are not durable because execution
observability belongs in the ops log. Materialized views must collapse claim
history into current authority, stale-claim status, and the latest relevant
release/expiry facts so future decision logic does not drag around old claim
churn.

Pause and cooldown are snapshot-tier by default. They become durable only when
they publish a shared constraint that another worker or human must respect, or
when they are part of a human-accountable escalation decision. A worker-local
cooldown after no ready work, provider retry backoff, or local operator pause is
runtime/execution state and should expire through snapshot compaction rather
than live forever in task history.

## File Map

### Created Files

- `internal/orchestrate/core.go` - pure orchestration transition and effect-planning logic
- `internal/orchestrate/ports.go` - narrow orchestration boundary interfaces for git, op-log, harness, clock, and execution side effects
- `internal/orchestrate/service.go` - imperative shell that interprets core effects through adapters
- `internal/orchestrate/service_test.go` - tests for effect interpretation and shell-to-core integration
- `internal/workerruntime/types.go` - runtime state, transition, gate result, worker loop, and run option types
- `internal/workerruntime/policy.go` - conservative built-in runtime policy defaults
- `internal/workerruntime/audit.go` - durable coordination event construction and durable append adapter
- `internal/workerruntime/event_policy.go` - durable-admission, snapshot, and trace sink classification rules
- `internal/workerruntime/runtime.go` - deterministic worker runtime loop and transition driver
- `internal/workerruntime/runtime_test.go` - queue-draining, no-ready-work, claim-loss, execution handoff, event-tiering, snapshot cooldown/pause, stop, and escalation tests
- `internal/materialize/history.go` - read-only history port for replaying state at a commit without binding to a concrete git adapter
- `internal/platform/gitconfig.go` - git-config boundary helpers used by worker identity and repo-context resolution
- `cmd/armature/worker.go` - `worker` parent command for runtime-oriented worker subcommands
- `cmd/armature/worker_run.go` - `arm worker run` command and flags
- `cmd/armature/worker_run_test.go` - CLI parsing and command behavior tests

### Modified Files

- `cmd/armature/orchestrate.go` - reduce to CLI parsing, app-context loading, and service invocation
- `cmd/armature/helpers.go` - share orchestration and runtime service construction helpers instead of embedding application logic in commands
- `cmd/armature/main.go` - register the new `worker` parent command without disrupting existing `worker-init` or `workers`
- `internal/ready/compute.go` - expose or adapt structured candidate data only if current exports are insufficient
- `internal/claim/claim.go` - expose or adapt structured claim-gate decisions only if current exports are insufficient
- `internal/orchestrate/engine.go` - split pure orchestration decisions from imperative effect execution and expose structured execution results
- `internal/orchestrate/harness.go` - keep harness invocation as an adapter boundary rather than a decision surface
- `internal/ops/types.go` - add only durable coordination/runtime-summary payload types; keep trace and snapshot-tier runtime records out of permanent ops
- `internal/worker/identity.go` - move git-config I/O behind a boundary instead of calling adapter shell helpers directly
- `internal/doctor/doctor.go` - separate environment collection from deterministic finding evaluation where practical
- `internal/config/context.go` - split repo probing I/O from context derivation logic
- `internal/materialize/atsha.go` - depend on a read-only history port instead of `*adapters.Client`
- `README.md` - update the homepage hook, value proposition, call to action, quickstart, and high-level Mermaid architecture/data-flow diagrams
- `docs/getting-started.md` - make `arm worker run` the default execution path and keep `arm orchestrate --issue` as the single-task fallback
- `docs/commands.md` - document `arm worker run`
- `docs/use-cases.md` - update persona walkthroughs for runtime-owned queue draining
- `docs/design/orchestrate-runtime-index.md` - note implementation status after v1 lands
- `internal/skillsembed/skills/armature/SKILL.md` - update command quick reference for `arm worker run`
- `internal/skillsembed/skills/armature-orchestrator/SKILL.md` - update runtime-operation flow for `arm worker run`
- `internal/skillsembed/skills/armature-coordinator/SKILL.md` - update coordinator workflow to supervise runtime workers by default

### No-Change Files

- `internal/orchestrate/prompt.go` - v1 runtime should not change prompt construction
- `internal/skillsembed/skills/armature-worker/SKILL.md` - keep as the manual fallback skill unless the implementation changes manual worker responsibilities
- `internal/skillsembed/skills/armature-planner/SKILL.md` - planner decomposition remains upstream of runtime queue draining
- `internal/skillsembed/skills/armature-auditor/SKILL.md` - auditor review remains downstream of runtime execution
- `docs/design/orchestrate-runtime-v1-scope.md` - implementation follows this scope note rather than rewriting it

## Task 0: Architectural Boundary Refactor (Precursor Dependency)

- [ ] **Step 1: Write the failing test for pure orchestration transition planning**

```go
func TestPlanNextStepReturnsEffectsWithoutTouchingAdapters(t *testing.T) {
	state := OrchestrateState{Phase: "pending"}

	next, effects := PlanNextStep(state, DecisionInput{
		TaskID:      "TASK-001",
		WorkerID:    "worker-a",
		NowUnix:     1700000000,
		RetryBudget: 3,
		ActiveScopes: map[string][]string{
			"TASK-999": []string{"internal/other/file.go"},
		},
	})

	assert.Equal(t, "dispatched", next.Phase)
	assert.Len(t, effects, 1)
	assert.Equal(t, EffectAppendDispatchOp, effects[0].Kind)
	assert.Equal(t, "TASK-001", effects[0].TaskID)
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/orchestrate -run TestPlanNextStepReturnsEffectsWithoutTouchingAdapters -count=1`
Expected: FAIL because `PlanNextStep`, `DecisionInput`, and effect-planning types do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Create `internal/orchestrate/core.go` and `internal/orchestrate/ports.go` with:

- a pure planner that takes current orchestration state plus structured inputs
  and returns next state plus effect requests
- typed effect requests for append-op, read-head-sha, run-harness, stage-add,
  commit, retry, and escalate behavior
- narrow interfaces for git, op-log, harness, and clock dependencies so the
  imperative shell interprets effects instead of the core performing I/O

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/orchestrate -run TestPlanNextStepReturnsEffectsWithoutTouchingAdapters -count=1`
Expected: PASS.

- [ ] **Step 5: Write the failing test for the imperative shell interpreter**

```go
func TestServiceExecutesPlannedEffectsThroughPorts(t *testing.T) {
	service := NewService(ServiceConfig{
		Git:   &fakeGit{Head: "abc123"},
		OpLog: &fakeOpLog{},
		Clock: fixedClock{Unix: 1700000000},
	})

	state, err := service.Run(context.Background(), RunInput{
		TaskID:       "TASK-001",
		WorkerID:     "worker-a",
		RetryBudget:  3,
		ActiveScopes: map[string][]string{},
	})

	require.NoError(t, err)
	assert.Equal(t, "dispatched", state.Phase)
	assert.Equal(t, 1, service.Config.OpLog.(*fakeOpLog).AppendCalls)
}
```

- [ ] **Step 6: Run the focused test to verify it fails**

Run: `go test ./internal/orchestrate -run TestServiceExecutesPlannedEffectsThroughPorts -count=1`
Expected: FAIL because `NewService`, `ServiceConfig`, and `RunInput` do not exist yet.

- [ ] **Step 7: Implement the minimal code**

Create `internal/orchestrate/service.go` and refactor `internal/orchestrate/engine.go`
so:

- pure transition planning lives in `core.go`
- imperative effect interpretation lives in `service.go`
- `engine.go` becomes a compatibility wrapper or thin facade over the new
  service instead of owning state decisions directly

- [ ] **Step 8: Run the focused tests to verify they pass**

Run: `go test ./internal/orchestrate -run "TestPlanNextStepReturnsEffectsWithoutTouchingAdapters|TestServiceExecutesPlannedEffectsThroughPorts" -count=1`
Expected: PASS.

- [ ] **Step 9: Write the failing command-level test for a thin orchestrate shell**

```go
func TestOrchestrateCommandDelegatesToService(t *testing.T) {
	service := &fakeOrchestrateService{State: orchestrate.OrchestrateState{Phase: "complete", Run: 1}}
	cmd := newOrchestrateCmdForService(service)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--issue", "TASK-001", "--dry-run"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, 1, service.RunCalls)
	assert.Contains(t, buf.String(), "\"phase\":\"complete\"")
}
```

- [ ] **Step 10: Run the focused test to verify it fails**

Run: `go test ./cmd/armature -run TestOrchestrateCommandDelegatesToService -count=1`
Expected: FAIL because the command still assembles and runs orchestration logic directly.

- [ ] **Step 11: Implement the minimal code**

Refactor `cmd/armature/orchestrate.go` and `cmd/armature/helpers.go` so:

- the Cobra command parses flags and loads app context
- service construction lives in one reusable helper
- command code no longer derives active scopes, builds imperative orchestration
  control flow, or depends on concrete adapters except through helper-created
  service ports

- [ ] **Step 12: Run the focused test to verify it passes**

Run: `go test ./cmd/armature -run TestOrchestrateCommandDelegatesToService -count=1`
Expected: PASS.

- [ ] **Step 13: Write the failing test for read-only materialization history ports**

```go
func TestMaterializeAtSHAUsesHistoryPortInsteadOfConcreteAdapter(t *testing.T) {
	history := fakeHistory{
		Files: []string{".armature/ops/worker-a.log"},
		Contents: map[string][]byte{
			".armature/ops/worker-a.log": []byte("[\"create\",\"TASK-001\",1700000000,\"worker-a\",{\"title\":\"T\",\"type\":\"task\"}]"),
		},
	}

	state, err := MaterializeAtSHA(history, "abc123", ".armature/ops")

	require.NoError(t, err)
	assert.Contains(t, state.BuildIndex(), "TASK-001")
}
```

- [ ] **Step 14: Run the focused test to verify it fails**

Run: `go test ./internal/materialize -run TestMaterializeAtSHAUsesHistoryPortInsteadOfConcreteAdapter -count=1`
Expected: FAIL because `MaterializeAtSHA` currently requires `*adapters.Client`.

- [ ] **Step 15: Implement the minimal code**

Create `internal/materialize/history.go` and refactor `internal/materialize/atsha.go`
to depend on a small read-only history interface that exposes only:

- `ListFilesAtCommit(sha string) ([]string, error)`
- `ShowFileAtCommit(sha, path string) ([]byte, error)`

- [ ] **Step 16: Run the focused test to verify it passes**

Run: `go test ./internal/materialize -run TestMaterializeAtSHAUsesHistoryPortInsteadOfConcreteAdapter -count=1`
Expected: PASS.

- [ ] **Step 17: Write the failing tests for explicit git-config and repo-probe boundaries**

```go
func TestWorkerIdentityUsesGitConfigPort(t *testing.T) {
	port := &fakeGitConfigPort{}

	id, err := InitWorkerWithPort(port)

	require.NoError(t, err)
	assert.NotEmpty(t, id)
	assert.Equal(t, 1, port.SetCalls)
}

func TestResolveContextSeparatesRepoProbeFromContextDerivation(t *testing.T) {
	probe := fakeRepoProbe{
		Mode:         "dual-branch",
		WorktreePath: "/repo/.arm",
	}

	ctx, err := ResolveContextWithProbe("/repo", probe, Config{})

	require.NoError(t, err)
	assert.Equal(t, "/repo", ctx.RepoPath)
	assert.Equal(t, "/repo/.arm/.armature", ctx.IssuesDir)
}
```

- [ ] **Step 18: Run the focused tests to verify they fail**

Run: `go test ./internal/worker ./internal/config -run "TestWorkerIdentityUsesGitConfigPort|TestResolveContextSeparatesRepoProbeFromContextDerivation" -count=1`
Expected: FAIL because worker identity and context resolution still call package-level adapter helpers directly.

- [ ] **Step 19: Implement the minimal code**

Create `internal/platform/gitconfig.go` and refactor `internal/worker/identity.go`
plus `internal/config/context.go` so:

- git-config access sits behind an explicit boundary
- repo probing and context derivation are split into separate functions
- deterministic path derivation can be tested without filesystem or git I/O

- [ ] **Step 20: Run the focused tests to verify they pass**

Run: `go test ./internal/worker ./internal/config -run "TestWorkerIdentityUsesGitConfigPort|TestResolveContextSeparatesRepoProbeFromContextDerivation" -count=1`
Expected: PASS.

- [ ] **Step 21: Write the failing test for doctor check separation**

```go
func TestEvaluateD1GitDivergenceConsumesCollectedSignals(t *testing.T) {
	finding := EvaluateD1GitDivergence([]string{
		"feat(TASK-001): commit",
	}, map[string]string{
		"TASK-001": "open",
	})

	assert.Equal(t, SeverityWarning, finding.Severity)
	assert.Contains(t, finding.Items[0], "TASK-001")
}
```

- [ ] **Step 22: Run the focused test to verify it fails**

Run: `go test ./internal/doctor -run TestEvaluateD1GitDivergenceConsumesCollectedSignals -count=1`
Expected: FAIL because finding evaluation is still mixed with git log collection.

- [ ] **Step 23: Implement the minimal code**

Refactor `internal/doctor/doctor.go` so:

- pure finding evaluation functions accept collected inputs
- adapter-backed collection functions gather git log, directory entries, and
  issue files
- the public doctor entrypoint remains imperative while deterministic check
  logic becomes reusable

- [ ] **Step 24: Run the focused test to verify it passes**

Run: `go test ./internal/doctor -run TestEvaluateD1GitDivergenceConsumesCollectedSignals -count=1`
Expected: PASS.

- [ ] **Step 25: Run the precursor verification suite**

Run: `go test ./internal/orchestrate ./internal/materialize ./internal/worker ./internal/config ./internal/doctor -count=1`
Expected: PASS.

- [ ] **Step 26: Commit**

```bash
git add internal/orchestrate internal/materialize internal/worker internal/config internal/doctor internal/platform cmd/armature/orchestrate.go cmd/armature/helpers.go
make check
git commit -m "refactor(runtime): restore imperative shell functional core boundaries"
```

## Task 1: Runtime Types, Policy Defaults, And Ports

- [ ] **Step 1: Write the failing test for runtime states, gates, and defaults**

```go
func TestDefaultPolicyAndStatesExposeV1RuntimeSurface(t *testing.T) {
	policy := DefaultPolicy()

	assert.Equal(t, StateIdle, InitialState())
	assert.Equal(t, StateClaimPending, NextState(StatePolling, TriggerClaimSelected))
	assert.Equal(t, "execution_lane_routing", policy.Subworkflows.ExecutionLaneRouting.Workflow)
	assert.False(t, policy.Subworkflows.ExecutionLaneRouting.ExceptionAgentEnabled)
	assert.Equal(t, 1, policy.Retry.MaxVerificationFailures)
	assert.Equal(t, 30*time.Second, policy.Cooldown.NoReadyWorkDelay)
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestDefaultPolicyAndStatesExposeV1RuntimeSurface -count=1`
Expected: FAIL because `DefaultPolicy`, `InitialState`, `NextState`, state constants, and policy structs do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Create `internal/workerruntime/types.go` and `internal/workerruntime/policy.go`
with:

- runtime state constants covering `idle`, `polling`, `claim_pending`,
  `claim_lost`, `claim_won`, `executing`, `recovering`, `paused`,
  `escalated`, and `stopped`
- trigger constants and a typed `NextState` helper for the deterministic
  transitions used later
- gate-result structs for `poll_gate`, `claim_gate`, `execute_gate`,
  `recovery_gate`, `resume_gate`, and `stop_gate`
- conservative `DefaultPolicy()` values for retry, cooldown, worker, model,
  harness, quota, escalation, and sub-workflow posture with exception-agent
  execution disabled

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestDefaultPolicyAndStatesExposeV1RuntimeSurface -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/types.go internal/workerruntime/policy.go internal/workerruntime/runtime_test.go
make check
git commit -m "feat(runtime): add worker runtime types and policy defaults"
```

## Task 2: Claim And Poll Adapters For A Single Runtime Cycle

- [ ] **Step 1: Write the failing test for no-ready-work cooldown**

```go
func TestPollAdapterStartsCooldownWhenReadyQueueIsEmpty(t *testing.T) {
	adapter := NewPollAdapter(fakeReadySource{})

	result := adapter.Poll(PollInput{
		WorkerID: "worker-a",
		Policy:   DefaultPolicy(),
		Now:      time.Unix(1700000000, 0),
	})

	assert.Equal(t, PollNoReadyWork, result.Outcome)
	assert.Equal(t, StateIdle, result.NextState)
	assert.Equal(t, 30*time.Second, result.RecheckAfter)
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestPollAdapterStartsCooldownWhenReadyQueueIsEmpty -count=1`
Expected: FAIL because `NewPollAdapter`, `PollInput`, and poll outcomes do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Adapt `internal/ready/compute.go` only as needed so `internal/workerruntime`
can:

- ask for structured ready candidates without scraping CLI text
- distinguish candidate-present, no-ready-work, and worker-disabled cases
- apply deterministic no-work cooldown using built-in defaults

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestPollAdapterStartsCooldownWhenReadyQueueIsEmpty -count=1`
Expected: PASS.

- [ ] **Step 5: Write the failing test for claim loss and inferred-node rejection**

```go
func TestClaimGateRejectsInferredNodesAndClassifiesClaimLoss(t *testing.T) {
	gate := NewClaimGate(fakeClaimAdapter{
		RejectInferred: true,
		ClaimWon:       false,
	})

	blocked := gate.Attempt(ClaimInput{IssueID: "TASK-INF", Confidence: "inferred"})
	lost := gate.Attempt(ClaimInput{IssueID: "TASK-002", Confidence: "verified"})

	assert.Equal(t, ClaimBlocked, blocked.Outcome)
	assert.Equal(t, "inferred_node", blocked.ReasonCode)
	assert.Equal(t, ClaimLost, lost.Outcome)
	assert.Equal(t, StateClaimLost, lost.NextState)
}
```

- [ ] **Step 6: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestClaimGateRejectsInferredNodesAndClassifiesClaimLoss -count=1`
Expected: FAIL because `NewClaimGate`, `ClaimInput`, and structured claim outcomes do not exist yet.

- [ ] **Step 7: Implement the minimal code**

Adapt `internal/claim/claim.go` only as needed so runtime code can:

- classify claim win, claim loss, blocked preflight, and policy-routed overlap
  outcomes without using CLI stderr strings
- keep existing human `claim --force` behavior in the CLI while the runtime
  uses deterministic or escalation-only paths

- [ ] **Step 8: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestClaimGateRejectsInferredNodesAndClassifiesClaimLoss -count=1`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/workerruntime/runtime.go internal/ready/compute.go internal/claim/claim.go internal/workerruntime/runtime_test.go
make check
git commit -m "feat(runtime): add poll and claim adapters"
```

## Task 3: Dogfood Slice B - One-Task `arm worker run`

- [ ] **Step 1: Write the failing test for one-task runtime execution**

```go
func TestWorkerRunMaxTasksOneExecutesSingleTask(t *testing.T) {
	runtime := NewTestRuntime(t, RuntimeOptions{MaxTasks: 1})
	runtime.QueueReadyTask("TASK-001")

	result := runtime.Run(context.Background())

	require.NoError(t, result.Err)
	assert.Equal(t, 1, result.TasksCompleted)
	assert.Equal(t, StateStopped, result.FinalState)
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestWorkerRunMaxTasksOneExecutesSingleTask -count=1`
Expected: FAIL because the runtime loop and max-task stop behavior do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Create `internal/workerruntime/runtime.go` logic for a single runtime cycle:

- poll one candidate
- attempt claim
- hand off to orchestration service
- stop when `MaxTasks` is reached

- [ ] **Step 4: Write the failing command test for one-task dogfood**

```go
func TestWorkerRunCommandSupportsSingleTaskDogfood(t *testing.T) {
	runtime := &fakeWorkerRuntime{
		Result: workerruntime.RunResult{TasksCompleted: 0, FinalState: workerruntime.StateStopped},
	}
	cmd := newWorkerCmdForRuntime(runtime)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"run", "--max-tasks", "1", "--dry-run"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, 1, runtime.RunCalls)
	assert.Contains(t, buf.String(), "dry_run")
}
```

- [ ] **Step 5: Run the focused test to verify it fails**

Run: `go test ./cmd/armature -run TestWorkerRunCommandSupportsSingleTaskDogfood -count=1`
Expected: FAIL because the command does not exist yet.

- [ ] **Step 6: Implement the minimal code**

Create `cmd/armature/worker.go`, `cmd/armature/worker_run.go`,
`cmd/armature/worker_run_test.go`, and register the `worker` parent command in
`cmd/armature/main.go` with:

- `worker run` as the runtime-owned execution path
- `--dry-run`
- `--max-tasks`
- an injectable runtime interface so command tests do not need git or filesystem
  setup
- worker ID resolution
- JSON and human output suitable for daily use

- [ ] **Step 7: Run focused verification**

Run: `go test ./internal/workerruntime ./cmd/armature -run "TestWorkerRunMaxTasksOneExecutesSingleTask|TestWorkerRunCommandSupportsSingleTaskDogfood" -count=1`
Expected: PASS.

- [ ] **Step 8: Dogfood Milestone B**

Run: `go run ./cmd/armature worker run --max-tasks 1`
Expected: one real task is polled, claimed, executed, and the command exits.

- [ ] **Step 9: Commit**

```bash
git add internal/workerruntime cmd/armature/worker.go cmd/armature/worker_run.go cmd/armature/worker_run_test.go cmd/armature/main.go
make check
git commit -m "feat(runtime): ship one-task worker run"
```

## Task 4: Durable Coordination Admission And Runtime Record Sinks

- [ ] **Step 1: Write the failing test for durable-log admission rules**

```go
func TestDurableAdmissionKeepsExecutionNoiseOutOfOpsLog(t *testing.T) {
	policy := DefaultEventPolicy()

	assert.Equal(t, TierEphemeral, policy.Tier(EventWorkerPollStarted))
	assert.Equal(t, TierEphemeral, policy.Tier(EventWorkerNoReadyWork))
	assert.Equal(t, TierEphemeral, policy.Tier(EventWorkerClaimLossProcessed))
	assert.Equal(t, TierEphemeral, policy.Tier(EventWorkerExecutionCompleted))
	assert.Equal(t, TierSnapshot, policy.Tier(EventCooldownStarted))
	assert.Equal(t, TierSnapshot, policy.Tier(EventWorkerPaused))
	assert.Equal(t, TierSnapshot, policy.Tier(EventRetryScheduled))
	assert.Equal(t, TierSnapshot, policy.Tier(EventWorkerPauseCheckpointed))
	assert.Equal(t, TierSnapshot, policy.Tier(EventWorkerStopRequested))
	assert.Equal(t, TierDurable, policy.Tier(EventClaimAcquired))
	assert.Equal(t, TierDurable, policy.Tier(EventClaimReleased))
	assert.Equal(t, TierDurable, policy.Tier(EventHumanEscalationCreated))
	assert.Equal(t, TierDurable, policy.Tier(EventHumanEscalationResolved))
	assert.Equal(t, TierDurable, policy.Tier(EventPolicyDecisionRecorded))
	assert.Equal(t, TierDurable, policy.Tier(EventWorkerRunSummary))

	writer := NewDurableEventWriter(&fakeDurableAppender{}, policy)
	require.NoError(t, writer.Append(context.Background(), RuntimeEvent{Type: EventWorkerNoReadyWork}))
	require.NoError(t, writer.Append(context.Background(), RuntimeEvent{Type: EventCooldownStarted}))
	require.NoError(t, writer.Append(context.Background(), RuntimeEvent{Type: EventWorkerStopRequested}))
	require.NoError(t, writer.Append(context.Background(), RuntimeEvent{Type: EventWorkerExecutionCompleted}))
	require.NoError(t, writer.Append(context.Background(), RuntimeEvent{Type: EventClaimAcquired}))
	require.NoError(t, writer.Append(context.Background(), RuntimeEvent{Type: EventHumanEscalationCreated}))
	require.NoError(t, writer.Append(context.Background(), RuntimeEvent{Type: EventWorkerRunSummary}))

	assert.Equal(t, 3, writer.Appender.(*fakeDurableAppender).AppendCalls)
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestDurableAdmissionKeepsExecutionNoiseOutOfOpsLog -count=1`
Expected: FAIL because durable admission rules, runtime record sinks, and the
durable writer do not exist yet.

- [ ] **Step 3: Implement the minimal record-sink code**

Create `internal/workerruntime/event_policy.go` and
`internal/workerruntime/audit.go` with:

- runtime event names for poll, claim, execution, recovery, cooldown, pause,
  escalation, and stop
- `TierDurable`, `TierSnapshot`, and `TierEphemeral`
- durable admission rules that allow only coordination authority, shared state,
  human-accountable decisions, shared policy constraints, and minimal run
  summaries into the ops log
- a default event policy that classifies loop telemetry and execution lifecycle
  narration as ephemeral, pause and cooldown as snapshot-tier by default, claims
  as durable coordination authority, and human-accountable/shared-state
  decisions as durable
- a durable writer that appends only `TierDurable` events and drops or routes
  lower-tier events to runtime snapshots or trace output

Extend `internal/ops/types.go` only for durable runtime records that must survive
forever. Do not add permanent op variants for no-ready-work, ordinary poll
starts, worker-local cooldown, local pause, claim-loss processing, or idle-loop
telemetry.

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestDurableAdmissionKeepsExecutionNoiseOutOfOpsLog -count=1`
Expected: PASS.

- [ ] **Step 5: Write the failing test for durable decision event shape**

```go
func TestDurableRuntimeEventCarriesCorrelationForSharedDecision(t *testing.T) {
	event := NewDurableRuntimeEvent(EventPolicyDecisionRecorded, DurableEventInput{
		IssueID:        "TASK-001",
		WorkerID:       "worker-a",
		RunID:          "run-123",
		CorrelationID:  "corr-123",
		CausationEvent: "evt-001",
		Outcome:        "reroute",
		NextAction:     "execute_with_fallback_model",
		PolicyRef:      "default.execution_lane_routing",
	})

	assert.Equal(t, EventPolicyDecisionRecorded, event.Type)
	assert.Equal(t, TierDurable, event.Tier)
	assert.Equal(t, "corr-123", event.CorrelationID)
	assert.Equal(t, "evt-001", event.CausationEventID)
	assert.Equal(t, "execute_with_fallback_model", event.NextAction)
	assert.NotEmpty(t, event.EventID)
}
```

- [ ] **Step 6: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestDurableRuntimeEventCarriesCorrelationForSharedDecision -count=1`
Expected: FAIL because durable event construction does not exist yet.

- [ ] **Step 7: Implement the minimal durable event construction**

Extend `internal/workerruntime/audit.go` so durable runtime events:

- carry shared audit fields needed for cross-run reconstruction and human review
- reference existing claim/orchestrate ops instead of duplicating their state
- include normalized input digests or evidence references instead of copying
  verbose runtime traces into the durable log
- reject construction for event types whose default tier is not durable unless
  an explicit shared-constraint reason is provided

- [ ] **Step 8: Run the focused tests to verify they pass**

Run: `go test ./internal/workerruntime -run "TestDurableAdmissionKeepsExecutionNoiseOutOfOpsLog|TestDurableRuntimeEventCarriesCorrelationForSharedDecision" -count=1`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/workerruntime/audit.go internal/workerruntime/event_policy.go internal/ops/types.go internal/workerruntime/runtime_test.go
make check
git commit -m "feat(runtime): admit only durable coordination records"
```

## Task 5: Dogfood Slice C - Repeat-Until-Empty Queue Draining

- [ ] **Step 1: Write the failing test for draining multiple tasks with contention**

```go
func TestRuntimeDrainsMultipleTasksAndHandlesClaimContention(t *testing.T) {
	runtime := NewTestRuntime(t, RuntimeOptions{MaxTasks: 2})
	runtime.QueueReadyTask("TASK-001")
	runtime.QueueReadyTask("TASK-002")
	runtime.MarkFirstClaimLost("TASK-001")

	result := runtime.Run(context.Background())

	require.NoError(t, result.Err)
	assert.Equal(t, 2, result.TasksCompleted)
	assert.Equal(t, StateStopped, result.FinalState)
	assert.True(t, runtime.HasEphemeralTrace(EventWorkerClaimLossProcessed))
	assert.False(t, runtime.HasDurableEvent(EventWorkerClaimLossProcessed))
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestRuntimeDrainsMultipleTasksAndHandlesClaimContention -count=1`
Expected: FAIL because the end-to-end worker loop and repeat behavior do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Extend the runtime loop so it:

- repeats after successful execution
- handles claim loss as ordinary deterministic control flow
- stops on empty queue, `MaxTasks`, or escalation
- supports a basic idle delay between empty-queue polls
- records claim contention and empty-queue churn in ephemeral or summary output,
  not in durable ops

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestRuntimeDrainsMultipleTasksAndHandlesClaimContention -count=1`
Expected: PASS.

- [ ] **Step 5: Dogfood Milestone C**

Run: `go run ./cmd/armature worker run`
Expected: the command replaces the manual `ready -> orchestrate -> repeat`
loop for ordinary queue draining.

- [ ] **Step 6: Commit**

```bash
git add internal/workerruntime/runtime.go internal/workerruntime/runtime_test.go cmd/armature/worker_run.go cmd/armature/worker_run_test.go
make check
git commit -m "feat(runtime): drain queue until empty"
```

## Task 6: Snapshot Cooldown, Pause, Stop, And Human Escalation

- [ ] **Step 1: Write the failing test for retry, pause, and stop control flow**

```go
func TestRecoveryPauseAndStopTransitionsRemainDeterministic(t *testing.T) {
	runtime := NewTestRuntime(t, RuntimeOptions{MaxTasks: 1})

	result := runtime.Recover(RecoveryInput{
		IssueID:       "TASK-003",
		FailureClass:  FailureVerification,
		RetryBudget:   1,
		StopRequested: true,
	})

	assert.Equal(t, StatePaused, result.IntermediateState)
	assert.Equal(t, StateStopped, result.FinalState)
	assert.True(t, runtime.HasSnapshotEvent(EventRetryScheduled))
	assert.True(t, runtime.HasSnapshotEvent(EventWorkerPauseCheckpointed))
	assert.True(t, runtime.HasSnapshotEvent(EventWorkerStopRequested))
	assert.False(t, runtime.HasDurableEvent(EventWorkerPauseCheckpointed))
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestRecoveryPauseAndStopTransitionsRemainDeterministic -count=1`
Expected: FAIL because recovery transitions, pause handling, and stop handling do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Extend `internal/workerruntime/runtime.go` to add deterministic:

- retry scheduling by failure class
- cooldown and repoll decisions
- pause and resume transitions
- checkpointed stop from `executing`
- human escalation fallback without bounded exception-agent execution
- snapshot compaction rules for retry timers, worker-local cooldown, local pause,
  and checkpointed stop context
- durable escalation records only when a human-accountable decision or shared
  task constraint is created

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestRecoveryPauseAndStopTransitionsRemainDeterministic -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/runtime.go internal/workerruntime/runtime_test.go
make check
git commit -m "feat(runtime): add cooldown pause and stop flow"
```

## Task 7: Dogfood Slice D - Richer Recovery And Runtime Policies

- [ ] **Step 1: Write the failing test for execution handoff result mapping**

```go
func TestExecutionHandoffInvokesSingleTaskOrchestrator(t *testing.T) {
	engine := &fakeOrchestrateEngine{Result: ExecuteSucceeded}
	runtime := NewTestRuntime(t, RuntimeOptions{Engine: engine})

	result := runtime.ExecuteClaimedTask("TASK-001")

	require.NoError(t, result.Err)
	assert.Equal(t, StateIdle, result.FinalState)
	assert.Equal(t, 1, engine.RunCalls)
	assert.True(t, runtime.HasEphemeralTrace(EventWorkerExecutionCompleted))
	assert.False(t, runtime.HasDurableEvent(EventWorkerExecutionCompleted))
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestExecutionHandoffInvokesSingleTaskOrchestrator -count=1`
Expected: FAIL because structured execution result mapping is not complete yet.

- [ ] **Step 3: Implement the minimal code**

Add the smallest runtime execution adapter needed to:

- invoke the cleaned `internal/orchestrate` service through ports
- map success, already-complete, already-escalated, failure, and cancellation
  into runtime outcomes
- activate conservative `execution_lane_routing` and `runtime_recovery`
  policy handling without bounded exception-agent execution
- preserve durable history by relying on existing orchestrator completion ops
  plus durable summaries or shared policy decisions, not per-execution lifecycle
  duplicates

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestExecutionHandoffInvokesSingleTaskOrchestrator -count=1`
Expected: PASS.

- [ ] **Step 5: Dogfood Milestone D**

Run: `go run ./cmd/armature worker run`
Expected: queue draining works with tiered runtime controls and richer recovery
while still escalating rather than invoking bounded exception agents. Durable
history should contain only shared decisions and summaries; ordinary pause,
cooldown, poll, and contention records should remain snapshot-tier or ephemeral.

- [ ] **Step 6: Commit**

```bash
git add internal/workerruntime internal/orchestrate
make check
git commit -m "feat(runtime): add richer recovery and execution routing"
```

## Task 8: User-Facing Documentation And Diagrams

- [ ] **Step 1: Write the failing documentation check for the homepage call to action**

```bash
rg -n "arm worker run" README.md
rg -n "arm worker run" docs/getting-started.md
rg -n "arm worker run" docs/commands.md
rg -n "arm worker run" docs/use-cases.md
rg -n "Mermaid|flowchart" README.md
```

Expected: FAIL because at least one required file does not yet describe
`arm worker run` as the default workflow or README does not yet include the
required Mermaid diagrams.

- [ ] **Step 2: Implement the minimal documentation updates**

Update:

- `README.md` with a homepage hook, value proposition, call to action centered
  on `arm worker run`, and two high-level Mermaid `flowchart` diagrams
- `docs/getting-started.md` so the default execution path becomes
  `arm worker run` and `arm orchestrate --issue` remains the single-task
  fallback
- `docs/commands.md` with a new `worker run` section and refreshed `ready` and
  `orchestrate` descriptions
- `docs/use-cases.md` so persona walkthroughs describe the runtime-owned loop

- [ ] **Step 3: Run the documentation check to verify it passes**

Run:
```bash
rg -n "arm worker run" README.md
rg -n "arm worker run" docs/getting-started.md
rg -n "arm worker run" docs/commands.md
rg -n "arm worker run" docs/use-cases.md
rg -n "Mermaid|flowchart" README.md
```
Expected: PASS with matches in each file that needs the new default workflow
and README diagram coverage.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/getting-started.md docs/commands.md docs/use-cases.md
make check
git commit -m "docs: teach worker runtime as the default workflow"
```

## Task 9: Embedded Skill Updates

- [ ] **Step 1: Write the failing skill-surface check**

```bash
rg -n "arm worker run" internal/skillsembed/skills/armature/SKILL.md
rg -n "arm worker run" internal/skillsembed/skills/armature-orchestrator/SKILL.md
rg -n "arm worker run" internal/skillsembed/skills/armature-coordinator/SKILL.md
rg -n "arm orchestrate --issue|manual fallback" internal/skillsembed/skills/armature-worker/SKILL.md
```

Expected: FAIL because the quick reference and orchestration skills still teach
the manual `ready` plus `orchestrate` loop as the default runtime-owned path.

- [ ] **Step 2: Implement the minimal skill updates**

Update:

- `internal/skillsembed/skills/armature/SKILL.md` so the quick reference leads
  with `arm worker run`
- `internal/skillsembed/skills/armature-orchestrator/SKILL.md` so the default
  operator behavior becomes supervising `arm worker run`
- `internal/skillsembed/skills/armature-coordinator/SKILL.md` so coordinated
  execution defaults to runtime workers and uses `arm orchestrate --issue` as
  the single-task fallback

Leave `internal/skillsembed/skills/armature-worker/SKILL.md` unchanged unless
manual worker responsibilities change during implementation.

- [ ] **Step 3: Run the skill check to verify it passes**

Run:
```bash
rg -n "arm worker run" internal/skillsembed/skills/armature/SKILL.md
rg -n "arm worker run" internal/skillsembed/skills/armature-orchestrator/SKILL.md
rg -n "arm worker run" internal/skillsembed/skills/armature-coordinator/SKILL.md
rg -n "arm orchestrate --issue|manual fallback" internal/skillsembed/skills/armature-worker/SKILL.md
```
Expected: PASS with the updated skills teaching `arm worker run` as the default
runtime path and `armature-worker` remaining the manual fallback.

- [ ] **Step 4: Commit**

```bash
git add internal/skillsembed/skills/armature/SKILL.md internal/skillsembed/skills/armature-orchestrator/SKILL.md internal/skillsembed/skills/armature-coordinator/SKILL.md
make check
git commit -m "docs(skills): update skills for worker runtime"
```

## Task 10: Integration Verification

- [ ] **Step 1: Write the failing materialization test for collapsed claim history**

```go
func TestMaterializedStateCollapsesHistoricalClaims(t *testing.T) {
	state := materialize.NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "TASK-001",
		Timestamp: 1699999900,
		WorkerID:  "worker-seed",
		Payload:   ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "TASK-001",
		Timestamp: 1700000000,
		WorkerID:  "worker-a",
		Payload:   ops.Payload{TTL: 60},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type:      ops.OpTransition,
		TargetID:  "TASK-001",
		Timestamp: 1700000100,
		WorkerID:  "worker-a",
		Payload:   ops.Payload{To: ops.StatusOpen},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "TASK-001",
		Timestamp: 1700000200,
		WorkerID:  "worker-b",
		Payload:   ops.Payload{TTL: 60},
	}))

	claim := state.CurrentClaim("TASK-001")

	require.NotNil(t, claim)
	assert.Equal(t, "worker-b", claim.WorkerID)
	assert.False(t, claim.Stale)
	assert.Len(t, state.ActiveClaimHistory("TASK-001"), 1)
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/materialize -run TestMaterializedStateCollapsesHistoricalClaims -count=1`
Expected: FAIL because materialized state does not yet prove that durable claim
history is collapsed away from the active decision model.

- [ ] **Step 3: Implement the minimal materialization compaction**

Refine materialized claim state so durable claim ops remain causal proof in the
source log while active read models expose only:

- current claim owner, if any
- claim freshness or stale-claim status
- latest release or expiry fact relevant to authority
- enough source-op references for audit drill-down without making old claim
  churn first-class active state

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/materialize -run TestMaterializedStateCollapsesHistoricalClaims -count=1`
Expected: PASS.

- [ ] **Step 5: Write the failing integration-style test for ready -> claim -> orchestrate**

```go
func TestWorkerRuntimeIntegratesReadyClaimAndOrchestrate(t *testing.T) {
	runtime := NewIntegrationRuntime(t)
	runtime.QueueReadyTask("TASK-001")

	result := runtime.Run(context.Background())

	require.NoError(t, result.Err)
	assert.Equal(t, 1, result.TasksCompleted)
	assert.True(t, runtime.ObservedExistingClaimOp("TASK-001"))
	assert.True(t, runtime.ObservedExistingOrchestrateCompletion("TASK-001"))
	assert.True(t, runtime.HasDurableEvent(EventWorkerRunSummary))
	assert.False(t, runtime.HasDurableEvent(EventWorkerPollStarted))
}
```

- [ ] **Step 6: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestWorkerRuntimeIntegratesReadyClaimAndOrchestrate -count=1`
Expected: FAIL because the full adapter stack is not integrated yet.

- [ ] **Step 7: Implement the minimal runtime integration coverage**

Add or refine integration-style coverage in `internal/workerruntime/runtime_test.go`
so the runtime proves:

- ready polling feeds claim selection
- claim outcomes feed execution handoff
- execution results feed existing claim/orchestrate ops, a durable run summary,
  and final runtime state without adding durable poll or idle-loop telemetry

- [ ] **Step 8: Run focused and full verification**

Run: `go test ./internal/workerruntime -run TestWorkerRuntimeIntegratesReadyClaimAndOrchestrate -count=1`
Expected: PASS.

Run: `go test ./internal/materialize -run TestMaterializedStateCollapsesHistoricalClaims -count=1`
Expected: PASS.

Run: `go test ./...`
Expected: PASS.

Run: `make check`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/workerruntime/runtime_test.go internal/materialize
make check
git commit -m "test(runtime): verify worker runtime end to end"
```
