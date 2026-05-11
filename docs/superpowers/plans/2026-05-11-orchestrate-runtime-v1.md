# Orchestrate Runtime V1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the v1 deterministic worker runtime that drains ready work by polling, claiming, invoking the existing single-task orchestrator, recording runtime audit events, and applying cooldown, pause, stop, and escalation control flow.

**Architecture:** Add a new `internal/workerruntime` package that owns runtime state transitions and wraps existing `internal/ready`, `internal/claim`, and `internal/orchestrate` behavior. Add explicit runtime policy, gate-result, and audit-event types with conservative built-in defaults before exposing the runtime through a new `arm worker run` command. Update user-facing documentation and embedded skills in the same implementation slice so people and agents learn the new default workflow as soon as the command exists.

**Tech Stack:** Go, Cobra, Armature append-only ops, materialization, existing ready/claim/orchestrate packages, `go test`, `make check`

---

## File Map

### Created Files

- `internal/workerruntime/types.go` - runtime state, transition, gate result, worker loop, and run option types
- `internal/workerruntime/policy.go` - conservative built-in runtime policy defaults
- `internal/workerruntime/audit.go` - runtime audit event construction and append adapter
- `internal/workerruntime/runtime.go` - deterministic worker runtime loop and transition driver
- `internal/workerruntime/runtime_test.go` - queue-draining, no-ready-work, claim-loss, execution handoff, cooldown, pause, stop, and escalation tests
- `cmd/armature/worker_run.go` - `arm worker run` command and flags
- `cmd/armature/worker_run_test.go` - CLI parsing and command behavior tests

### Modified Files

- `cmd/armature/main.go` - register the new `worker run` command without disrupting existing `worker-init` or `workers`
- `internal/ready/compute.go` - expose or adapt structured candidate data only if current exports are insufficient
- `internal/claim/claim.go` - expose or adapt structured claim-gate decisions only if current exports are insufficient
- `internal/orchestrate/engine.go` - add minimal result mapping only if existing `Result` does not expose enough status for runtime transitions
- `internal/ops/types.go` - add runtime audit op/event payload types
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

## Task 1: Runtime Types And Policy Defaults

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
git commit -m "feat(runtime): add worker runtime types and policy defaults"
```

## Task 2: Runtime Audit Events

- [ ] **Step 1: Write the failing test for append-only runtime audit events**

```go
func TestAuditEventCarriesCorrelationAndCooldownState(t *testing.T) {
	event := NewAuditEvent(EventCooldownStarted, AuditInput{
		IssueID:        "TASK-001",
		WorkerID:       "worker-a",
		RunID:          "run-123",
		CorrelationID:  "corr-123",
		CausationEvent: "evt-001",
		Outcome:        "cooldown",
		NextAction:     "repoll",
	})

	assert.Equal(t, EventCooldownStarted, event.EventType)
	assert.Equal(t, "corr-123", event.CorrelationID)
	assert.Equal(t, "evt-001", event.CausationEventID)
	assert.Equal(t, "repoll", event.NextAction)
	assert.NotEmpty(t, event.EventID)
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestAuditEventCarriesCorrelationAndCooldownState -count=1`
Expected: FAIL because `NewAuditEvent`, `AuditInput`, and runtime audit event types do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Create `internal/workerruntime/audit.go` and extend `internal/ops/types.go`
only enough to:

- define runtime audit event names for poll, claim, execution, recovery,
  cooldown, pause, escalation, and stop
- construct append-only audit events with shared fields from the Phase 2 audit
  model
- provide an adapter that can map runtime events onto the existing ops append
  path without deciding the final repo serialization format beyond what v1
  needs

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestAuditEventCarriesCorrelationAndCooldownState -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/audit.go internal/ops/types.go internal/workerruntime/runtime_test.go
git commit -m "feat(runtime): add runtime audit events"
```

## Task 3: Ready Poll Adapter

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

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/runtime.go internal/ready/compute.go internal/workerruntime/runtime_test.go
git commit -m "feat(runtime): add ready poll adapter"
```

## Task 4: Claim Gate Adapter

- [ ] **Step 1: Write the failing test for claim loss and inferred-node rejection**

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

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestClaimGateRejectsInferredNodesAndClassifiesClaimLoss -count=1`
Expected: FAIL because `NewClaimGate`, `ClaimInput`, and structured claim outcomes do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Adapt `internal/claim/claim.go` only as needed so runtime code can:

- classify claim win, claim loss, blocked preflight, and policy-routed overlap
  outcomes without using CLI stderr strings
- keep existing human `claim --force` behavior in the CLI while the runtime
  uses deterministic or escalation-only paths

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestClaimGateRejectsInferredNodesAndClassifiesClaimLoss -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/runtime.go internal/claim/claim.go internal/workerruntime/runtime_test.go
git commit -m "feat(runtime): add structured claim gate decisions"
```

## Task 5: Execution Handoff

- [ ] **Step 1: Write the failing test for `claim_won -> executing` handoff**

```go
func TestExecutionHandoffInvokesSingleTaskOrchestrator(t *testing.T) {
	engine := &fakeOrchestrateEngine{Result: ExecuteSucceeded}
	runtime := NewTestRuntime(t, RuntimeOptions{Engine: engine})

	result := runtime.ExecuteClaimedTask("TASK-001")

	require.NoError(t, result.Err)
	assert.Equal(t, StateIdle, result.FinalState)
	assert.Equal(t, 1, engine.RunCalls)
	assert.True(t, runtime.HasAuditEvent(EventWorkerExecutionCompleted))
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestExecutionHandoffInvokesSingleTaskOrchestrator -count=1`
Expected: FAIL because runtime execution adapters and result mapping do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Add the smallest runtime execution adapter needed to:

- invoke the existing `internal/orchestrate.Engine`
- map success, already-complete, already-escalated, failure, and cancellation
  into runtime outcomes
- extend `internal/orchestrate/engine.go` only if the current result shape does
  not expose enough status for runtime transitions

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestExecutionHandoffInvokesSingleTaskOrchestrator -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/runtime.go internal/orchestrate/engine.go internal/workerruntime/runtime_test.go
git commit -m "feat(runtime): add orchestrate execution handoff"
```

## Task 6: Recovery, Cooldown, Pause, And Stop

- [ ] **Step 1: Write the failing test for retry, pause, and stop control flow**

```go
func TestRecoveryPauseAndStopTransitionsRemainDeterministic(t *testing.T) {
	runtime := NewTestRuntime(t, RuntimeOptions{MaxTasks: 1})

	result := runtime.Recover(RecoveryInput{
		IssueID:      "TASK-003",
		FailureClass: FailureVerification,
		RetryBudget:  1,
		StopRequested: true,
	})

	assert.Equal(t, StatePaused, result.IntermediateState)
	assert.Equal(t, StateStopped, result.FinalState)
	assert.True(t, runtime.HasAuditEvent(EventRetryScheduled))
	assert.True(t, runtime.HasAuditEvent(EventWorkerPauseCheckpointed))
	assert.True(t, runtime.HasAuditEvent(EventWorkerStopRequested))
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

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestRecoveryPauseAndStopTransitionsRemainDeterministic -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/runtime.go internal/workerruntime/runtime_test.go
git commit -m "feat(runtime): add recovery pause and stop flow"
```

## Task 7: Worker Runtime Loop

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
	assert.True(t, runtime.HasAuditEvent(EventWorkerClaimLost))
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestRuntimeDrainsMultipleTasksAndHandlesClaimContention -count=1`
Expected: FAIL because the end-to-end worker loop and run result shape do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Implement the queue-draining loop in `internal/workerruntime/runtime.go` so it:

- polls for candidates
- attempts claims
- handles claim loss as ordinary control flow
- invokes execution handoff
- tracks correlation IDs across one runtime run
- stops on max-task or explicit stop conditions

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestRuntimeDrainsMultipleTasksAndHandlesClaimContention -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/runtime.go internal/workerruntime/runtime_test.go
git commit -m "feat(runtime): add worker runtime loop"
```

## Task 8: `arm worker run` Command

- [ ] **Step 1: Write the failing command test for flags and dry-run**

```go
func TestWorkerRunCommandParsesFlagsAndSupportsDryRun(t *testing.T) {
	cmd := newWorkerRunCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"run", "--dry-run", "--max-tasks", "1", "--idle-delay", "10s"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "dry_run")
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./cmd/armature -run TestWorkerRunCommandParsesFlagsAndSupportsDryRun -count=1`
Expected: FAIL because `newWorkerRunCmd` and the `worker run` subcommand do not exist yet.

- [ ] **Step 3: Implement the minimal code**

Create `cmd/armature/worker_run.go` and `cmd/armature/worker_run_test.go`, then
register the command in `cmd/armature/main.go` with:

- `--dry-run`
- `--max-tasks`
- `--idle-delay`
- worker ID resolution using the existing local worker identity path
- exit-code behavior that distinguishes clean stop from escalated failure

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./cmd/armature -run TestWorkerRunCommandParsesFlagsAndSupportsDryRun -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/worker_run.go cmd/armature/worker_run_test.go cmd/armature/main.go
git commit -m "feat(cli): add worker run command"
```

## Task 9: User-Facing Documentation And Diagrams

- [ ] **Step 1: Write the failing documentation test for the homepage call to action**

```bash
rg -n "arm worker run|Mermaid|flowchart" README.md docs/getting-started.md docs/commands.md docs/use-cases.md
```

Expected: FAIL because the docs do not yet consistently describe `arm worker run`
as the default workflow or include the required Mermaid diagrams.

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

Run: `rg -n "arm worker run|Mermaid|flowchart" README.md docs/getting-started.md docs/commands.md docs/use-cases.md`
Expected: PASS with matches in each file that needs the new default workflow or
diagram coverage.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/getting-started.md docs/commands.md docs/use-cases.md
git commit -m "docs: teach worker runtime as the default workflow"
```

## Task 10: Embedded Skill Updates

- [ ] **Step 1: Write the failing skill-surface check**

```bash
rg -n "arm worker run|arm orchestrate --issue|manual fallback" internal/skillsembed/skills/armature/SKILL.md internal/skillsembed/skills/armature-orchestrator/SKILL.md internal/skillsembed/skills/armature-coordinator/SKILL.md internal/skillsembed/skills/armature-worker/SKILL.md
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

Run: `rg -n "arm worker run|arm orchestrate --issue|manual fallback" internal/skillsembed/skills/armature/SKILL.md internal/skillsembed/skills/armature-orchestrator/SKILL.md internal/skillsembed/skills/armature-coordinator/SKILL.md internal/skillsembed/skills/armature-worker/SKILL.md`
Expected: PASS with the updated skills teaching `arm worker run` as the default
runtime path and `armature-worker` remaining the manual fallback.

- [ ] **Step 4: Commit**

```bash
git add internal/skillsembed/skills/armature/SKILL.md internal/skillsembed/skills/armature-orchestrator/SKILL.md internal/skillsembed/skills/armature-coordinator/SKILL.md
git commit -m "docs(skills): update skills for worker runtime"
```

## Task 11: Integration Verification

- [ ] **Step 1: Write the failing integration-style test for ready -> claim -> orchestrate**

```go
func TestWorkerRuntimeIntegratesReadyClaimAndOrchestrate(t *testing.T) {
	runtime := NewIntegrationRuntime(t)
	runtime.QueueReadyTask("TASK-001")

	result := runtime.Run(context.Background())

	require.NoError(t, result.Err)
	assert.Equal(t, 1, result.TasksCompleted)
	assert.True(t, runtime.HasAuditEvent(EventWorkerClaimWon))
	assert.True(t, runtime.HasAuditEvent(EventWorkerExecutionCompleted))
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestWorkerRuntimeIntegratesReadyClaimAndOrchestrate -count=1`
Expected: FAIL because the full adapter stack is not integrated yet.

- [ ] **Step 3: Implement the minimal code**

Add or refine integration-style coverage in `internal/workerruntime/runtime_test.go`
so the runtime proves:

- ready polling feeds claim selection
- claim outcomes feed execution handoff
- execution results feed audit events and final runtime state

- [ ] **Step 4: Run focused and full verification**

Run: `go test ./internal/workerruntime -run TestWorkerRuntimeIntegratesReadyClaimAndOrchestrate -count=1`
Expected: PASS.

Run: `go test ./...`
Expected: PASS.

Run: `make check`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workerruntime/runtime_test.go
git commit -m "test(runtime): verify worker runtime end to end"
```

