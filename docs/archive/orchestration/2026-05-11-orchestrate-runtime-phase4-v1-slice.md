# Orchestrate Runtime Phase 4 V1 Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete Phase 4 of the orchestrate runtime roadmap as one docs-first chunk: choose the thinnest valuable v1 runtime slice, record the scope decision, and write the implementation plan for building that runtime.

**Architecture:** Phase 4 remains a planning and design stabilization pass, not production runtime implementation. It consumes the Phase 1-3 runtime notes, chooses the v1 product boundary, records the decision in a focused scope note, then writes a separate implementation plan for the future code slice. The chosen v1 should bias toward a deterministic embedded worker runtime that wraps existing `ready`, `claim`, and single-task `orchestrate` surfaces, with audit and policy seams explicit enough for implementation.

**Tech Stack:** Markdown, Go source reading, git, ripgrep, sed, Armature design docs

**Inputs:**
- `docs/design/orchestrate-runtime-index.md`
- `docs/design/orchestrate-runtime-roadmap.md`
- `docs/design/orchestrate-runtime-direction.md`
- `docs/design/orchestrate-runtime-decisions.md`
- `docs/design/orchestrate-readiness-and-executability.md`
- `docs/design/orchestrate-policy-subworkflow-specification.md`
- `docs/design/orchestrate-worker-runtime-state-machine.md`
- `docs/design/orchestrate-runtime-policy-model.md`
- `docs/design/orchestrate-audit-model.md`
- `docs/design/orchestrate-runtime-gap-analysis.md`
- `docs/design/orchestrate-runtime-oss-review.md`
- `README.md`
- `docs/getting-started.md`
- `docs/commands.md`
- `docs/use-cases.md`
- `docs/design/architecture.md`
- `internal/skillsembed/skills/armature/SKILL.md`
- `internal/skillsembed/skills/armature-orchestrator/SKILL.md`
- `internal/skillsembed/skills/armature-coordinator/SKILL.md`
- `internal/skillsembed/skills/armature-worker/SKILL.md`
- `cmd/armature/ready.go`
- `cmd/armature/claim.go`
- `cmd/armature/orchestrate.go`
- `internal/ready/compute.go`
- `internal/claim/claim.go`
- `internal/orchestrate/engine.go`
- `internal/orchestrate/state.go`
- `internal/ops/types.go`

---

## File Map

### Created Files

- `docs/design/orchestrate-runtime-v1-scope.md` - Phase 4 scope note that chooses the v1 runtime slice, included sub-workflows, deferred sub-workflows, CLI posture, exception-agent posture, audit posture, policy posture, and non-goals.
- `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md` - implementation plan for building the selected v1 runtime slice after Phase 4 is approved.

### Modified Files

- `docs/design/orchestrate-runtime-index.md` - add the Phase 4 v1 scope note, update current position, and replace Phase 4 open follow-ups with implementation-focused follow-ups.
- `docs/design/orchestrate-runtime-roadmap.md` - mark Phase 4 complete once the v1 scope note and implementation plan exist.

### No-Change Files

- `cmd/armature/**` - do not add runtime commands during Phase 4.
- `internal/**` - do not add runtime implementation during Phase 4.
- `internal/ops/types.go` - do not add audit op types during Phase 4.
- `README.md`, `docs/getting-started.md`, `docs/commands.md`, `docs/use-cases.md`, and embedded skills - do not update user-facing docs or skills during Phase 4; instead, require those updates in the v1 runtime implementation plan.
- Phase 1-3 design notes - do not rewrite previous decisions except by adding the new Phase 4 scope note and index/roadmap updates.

---

## Chunk 1: Reconfirm Phase 4 Inputs

### Task 1: Read The Required Runtime Context

**Files:**
- Read: `docs/design/orchestrate-runtime-index.md`
- Read: `docs/design/orchestrate-runtime-roadmap.md`
- Read: `docs/design/orchestrate-runtime-direction.md`
- Read: `docs/design/orchestrate-runtime-decisions.md`
- Read: `docs/design/orchestrate-readiness-and-executability.md`
- Read: `docs/design/orchestrate-policy-subworkflow-specification.md`
- Read: `docs/design/orchestrate-worker-runtime-state-machine.md`
- Read: `docs/design/orchestrate-runtime-policy-model.md`
- Read: `docs/design/orchestrate-audit-model.md`
- Read: `docs/design/orchestrate-runtime-gap-analysis.md`
- Read: `docs/design/orchestrate-runtime-oss-review.md`

- [ ] **Step 1: Read the roadmap and index**

Run:

```bash
sed -n '1,220p' docs/design/orchestrate-runtime-index.md
sed -n '1,260p' docs/design/orchestrate-runtime-roadmap.md
```

Expected: Phase 4 has two outputs: a v1 scope note and an implementation plan suitable for Armature work items.

- [ ] **Step 2: Read the locked boundaries and decision ledger**

Run:

```bash
sed -n '1,280p' docs/design/orchestrate-runtime-direction.md
sed -n '1,300p' docs/design/orchestrate-runtime-decisions.md
```

Expected: the v1 runtime wraps the single-task orchestrator, keeps routine queue draining deterministic, avoids persistent agent supervisors, excludes autonomous decomposition, and allows only narrow bounded recovery when explicitly chosen.

- [ ] **Step 3: Read the Phase 2 control model**

Run:

```bash
sed -n '1,260p' docs/design/orchestrate-readiness-and-executability.md
sed -n '1,360p' docs/design/orchestrate-policy-subworkflow-specification.md
sed -n '1,360p' docs/design/orchestrate-worker-runtime-state-machine.md
sed -n '1,320p' docs/design/orchestrate-runtime-policy-model.md
sed -n '1,320p' docs/design/orchestrate-audit-model.md
```

Expected: the worker runtime state machine, gate vocabulary, policy groups, shared sub-workflow result shape, and audit event model are clear enough to choose a v1 subset.

- [ ] **Step 4: Read the Phase 3 reconciliation notes**

Run:

```bash
sed -n '1,360p' docs/design/orchestrate-runtime-gap-analysis.md
sed -n '1,320p' docs/design/orchestrate-runtime-oss-review.md
```

Expected: Phase 3 recommends building the v1 runtime directly and reusing existing `ready`, `claim`, and `orchestrate` surfaces as the first implementation substrate.

- [ ] **Step 5: Confirm there are no production-code edits in progress**

Run:

```bash
git status --short
```

Expected: any existing unrelated changes are left untouched. Phase 4 edits should be limited to `docs/design/**` and `docs/superpowers/plans/**`.

---

## Chunk 2: Inspect Current Runtime-Relevant Code Surfaces

### Task 2: Map The Reuse Boundary

**Files:**
- Read: `docs/commands.md`
- Read: `docs/design/architecture.md`
- Read: `cmd/armature/ready.go`
- Read: `cmd/armature/claim.go`
- Read: `cmd/armature/orchestrate.go`
- Read: `internal/ready/compute.go`
- Read: `internal/claim/claim.go`
- Read: `internal/orchestrate/engine.go`
- Read: `internal/orchestrate/state.go`
- Read: `internal/ops/types.go`

- [ ] **Step 1: Read the current command docs**

Run:

```bash
sed -n '430,510p' docs/commands.md
sed -n '70,95p' docs/commands.md
sed -n '700,730p' docs/commands.md
sed -n '1,260p' docs/design/architecture.md
```

Expected observations:

```text
ready: current queue-front command.
claim: current manual claim command.
orchestrate: current single-task execution command.
worker-init/workers: existing worker identity and visibility surfaces, not queue-draining runtime surfaces.
architecture: Armature remains git-native, append-only, and materialization-driven.
```

- [ ] **Step 2: Read command entry points**

Run:

```bash
sed -n '1,240p' cmd/armature/ready.go
sed -n '1,260p' cmd/armature/claim.go
sed -n '1,280p' cmd/armature/orchestrate.go
```

Expected observations:

```text
ready can provide candidate discovery for the runtime poll gate.
claim can provide claim mechanics but needs reusable structured gate logic outside CLI presentation.
orchestrate can remain the single-task execution handoff for v1.
```

- [ ] **Step 3: Read internal packages that v1 should wrap**

Run:

```bash
sed -n '1,260p' internal/ready/compute.go
sed -n '1,240p' internal/claim/claim.go
sed -n '1,360p' internal/orchestrate/engine.go
sed -n '1,240p' internal/orchestrate/state.go
sed -n '1,220p' internal/ops/types.go
```

Expected observations:

```text
internal/ready computes eligible work deterministically.
internal/claim contains scope overlap primitives and claim support.
internal/orchestrate.Engine owns single-task execution, retry, verification, completion, and escalation.
internal/orchestrate/state derives task execution state from ops.
internal/ops/types has orchestration op names but no worker-runtime audit event catalog yet.
```

- [ ] **Step 4: Search for prior runtime, claim, and op references**

Run:

```bash
rg -n "worker runtime|OpOrchestrate|ready|claim|orchestrate|worker-init|workers" docs cmd internal
```

Expected: enough source references to anchor the Phase 4 v1 implementation plan in existing files and avoid inventing a parallel architecture.

---

## Chunk 3: Map User-Facing Documentation And Skill Surfaces

### Task 3: Identify Required Docs And Skill Updates

**Files:**
- Read: `README.md`
- Read: `docs/getting-started.md`
- Read: `docs/commands.md`
- Read: `docs/use-cases.md`
- Read: `internal/skillsembed/skills/armature/SKILL.md`
- Read: `internal/skillsembed/skills/armature-orchestrator/SKILL.md`
- Read: `internal/skillsembed/skills/armature-coordinator/SKILL.md`
- Read: `internal/skillsembed/skills/armature-worker/SKILL.md`

- [ ] **Step 1: Read the homepage and onboarding docs**

Run:

```bash
sed -n '1,220p' README.md
sed -n '1,220p' docs/getting-started.md
sed -n '430,515p' docs/commands.md
sed -n '1,140p' docs/use-cases.md
```

Expected observations:

```text
README.md is the homepage and needs a user-facing hook plus a clear call to action for the new worker-runtime flow.
docs/getting-started.md must teach the default v1 path as `arm worker run`, while preserving `arm orchestrate --issue` as the single-task fallback.
docs/commands.md must document `arm worker run` and update the `ready`/`orchestrate` sections so users understand the old loop is now owned by the runtime.
docs/use-cases.md must update persona walkthroughs that currently tell users to repeat `arm ready` and `arm orchestrate` manually.
```

- [ ] **Step 2: Read the embedded skills that teach agent behavior**

Run:

```bash
sed -n '1,180p' internal/skillsembed/skills/armature/SKILL.md
sed -n '1,220p' internal/skillsembed/skills/armature-orchestrator/SKILL.md
sed -n '1,220p' internal/skillsembed/skills/armature-coordinator/SKILL.md
sed -n '1,180p' internal/skillsembed/skills/armature-worker/SKILL.md
```

Expected observations:

```text
armature: quick reference should point routine execution to `arm worker run`.
armature-orchestrator: should become the runtime-operation skill for supervising `arm worker run`, handling pause/escalation, and falling back to `arm orchestrate --issue` for one task.
armature-coordinator: should describe `arm worker run` as the default queue-draining mechanism, with manual concurrent orchestrator processes as a fallback.
armature-worker: should remain the manual fallback skill unless the v1 implementation changes manual worker responsibilities.
```

- [ ] **Step 3: Record documentation requirements in the v1 scope note**

When writing `docs/design/orchestrate-runtime-v1-scope.md`, include a
`User-Facing Documentation And Skills` section that requires:

```markdown
- `README.md` homepage update with a hook, a concise value proposition, and a call to action centered on `arm worker run`
- `README.md` high-level Mermaid architecture and data-flow diagrams
- `docs/getting-started.md` quickstart update for `arm worker run`
- `docs/commands.md` command reference for `arm worker run`
- `docs/use-cases.md` persona updates for the runtime-owned loop
- embedded skill updates for `armature`, `armature-orchestrator`, and `armature-coordinator`
- confirmation that `armature-worker` remains the manual fallback skill or an explicit update if the implementation changes that role
```

Diagrams should stay high-level and readable. Use Mermaid `flowchart` diagrams
for architecture and data flow. Use sequence diagrams only when a short
interaction needs ordering clarity that a flowchart cannot provide.

---

## Chunk 4: Choose The V1 Slice

### Task 4: Write The V1 Scope Note

**Files:**
- Create: `docs/design/orchestrate-runtime-v1-scope.md`

- [ ] **Step 1: Create the scope note with the selected v1 posture**

Create `docs/design/orchestrate-runtime-v1-scope.md` with this structure:

```markdown
# Orchestrate Runtime V1 Scope

## Purpose

This Phase 4 note chooses the thinnest valuable v1 runtime slice. It translates
the Phase 1-3 runtime direction into an implementation boundary without making
production code changes.

## V1 Product Boundary

V1 should build a deterministic embedded worker runtime that owns the normal
`ready -> claim -> orchestrate -> repeat` loop and wraps the existing
single-task orchestrator.

The runtime should surface as a new `arm worker run` command in the
implementation plan. This gives queue-draining lifecycle ownership its own
mental model while preserving `arm orchestrate` as the single-task execution
surface.

## Included In V1

- deterministic queue polling using the existing ready computation as the first poll substrate
- deterministic claim attempts and claim win/loss handling
- execution handoff to the existing single-task orchestrator
- explicit runtime state machine package with `idle`, `polling`, `claim_pending`, `claim_lost`, `claim_won`, `executing`, `recovering`, `paused`, `escalated`, and `stopped`
- structured gate result types for `poll_gate`, `claim_gate`, `execute_gate`, `recovery_gate`, `resume_gate`, and `stop_gate`
- conservative built-in runtime policy defaults for retry, cooldown, worker, model, harness, quota, escalation, and sub-workflow posture
- append-only runtime audit events sufficient to reconstruct poll, claim, execution, recovery, cooldown, pause, escalation, and stop decisions
- cooldown and pause state materialized from runtime audit events
- human escalation as the fallback for blocked, exhausted, or ambiguous recovery

## V1 Sub-Workflow Scope

V1 should implement deterministic hooks and result shapes for all five
sub-workflows, but only `execution_lane_routing` and `runtime_recovery` should
be active runtime decision paths in the first implementation slice.

`task_contract_review`, `scope_and_dependency_resolution`, and
`provenance_review` remain represented in shared types and audit vocabulary so
future work can add them without changing runtime contracts.

## Exception-Agent Scope

V1 should not execute bounded exception agents. It should implement the
deterministic hooks, policy result fields, allowed-action envelope shape, audit
events, and escalation behavior needed to add bounded exception-agent execution
later.

This keeps the first implementation valuable and auditable while avoiding a
larger permission-enforcement surface in the first code slice.

## User-Facing Documentation And Skills

V1 implementation must update the user-facing documentation and embedded skills
that teach the new runtime-owned workflow.

Required documentation updates:

- `README.md` homepage hook, concise value proposition, and call to action for `arm worker run`
- `README.md` high-level Mermaid architecture diagram
- `README.md` high-level Mermaid data-flow diagram
- `docs/getting-started.md` default quickstart path using `arm worker run`
- `docs/commands.md` command reference for `arm worker run`
- `docs/use-cases.md` persona walkthrough updates for runtime-owned queue draining

Required skill updates:

- `internal/skillsembed/skills/armature/SKILL.md`
- `internal/skillsembed/skills/armature-orchestrator/SKILL.md`
- `internal/skillsembed/skills/armature-coordinator/SKILL.md`

`internal/skillsembed/skills/armature-worker/SKILL.md` should remain the manual
fallback skill unless implementation changes the manual worker role.

Diagrams should stay high-level and readable. Use Mermaid `flowchart` diagrams
for architecture and data flow. Use sequence diagrams only when ordering is the
main point and a flowchart would be less clear.

## Deferred From V1

- persistent agent supervisors
- autonomous decomposition or replanning
- broad semantic task rewriting
- distributed workflow infrastructure
- full repo-specific policy file syntax
- runtime execution of bounded exception agents
- automatic draft promotion or planner-owned governance decisions
- replacing `arm orchestrate`

## Acceptance Criteria

- a worker can run a deterministic queue-draining loop without an LLM-operated outer loop
- claim collisions are handled as ordinary deterministic control flow
- no-ready-work and cooldown behavior do not spin aggressively
- successful execution delegates to the existing single-task orchestrator
- runtime state transitions emit repo-visible audit events
- pause, stop, and human escalation states are reconstructable from materialized audit state
- no hidden service state is required to understand worker progress
- user-facing docs explain the new default workflow with a homepage hook, call to action, and high-level Mermaid architecture and data-flow diagrams
- embedded skills teach agents when to use `arm worker run`, when to supervise it, and when to fall back to `arm orchestrate --issue` or manual worker flow
```

- [ ] **Step 2: Self-review the scope note**

Run:

```bash
rg -n "T[B]D|T[O]DO|implement later|m[a]ybe|pr[o]bably|uncl[e]ar" docs/design/orchestrate-runtime-v1-scope.md
```

Expected: no matches. If matches appear, replace them with explicit decisions.

- [ ] **Step 3: Check scope note consistency against locked boundaries**

Run:

```bash
rg -n "persistent agent|autonomous decomposition|replace|exception-agent|arm worker run|arm orchestrate" docs/design/orchestrate-runtime-v1-scope.md docs/design/orchestrate-runtime-direction.md docs/design/orchestrate-runtime-decisions.md
```

Expected: the new scope note preserves the locked Phase 1 decisions and makes Phase 4 decisions explicit.

---

## Chunk 5: Write The Future Runtime Implementation Plan

### Task 5: Create The V1 Runtime Implementation Plan

**Files:**
- Create: `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md`

- [ ] **Step 1: Create the plan header and file map**

Create `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md` with the required plan header:

```markdown
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
```

- [ ] **Step 2: Add implementation tasks in this order**

Append these top-level tasks to the runtime implementation plan:

```markdown
## Task 1: Runtime Types And Policy Defaults

Add tests first for runtime states, gate outcomes, policy defaults, and sub-workflow hook representation. Implement the minimum `internal/workerruntime` types needed by later tasks.

## Task 2: Runtime Audit Events

Add tests for append-only runtime audit event construction, shared fields, event type validation, correlation IDs, causation IDs, and materializable cooldown or pause state. Implement audit event types and writer adapters.

## Task 3: Ready Poll Adapter

Add tests for polling ready work, no-ready-work cooldown, stopped workers, and quota-limited polling. Implement a poll adapter around existing ready computation.

## Task 4: Claim Gate Adapter

Add tests for claim win, claim loss, inferred-node rejection, blocked claim preflight, and overlap policy routing. Implement structured claim-gate decisions around existing claim mechanics.

## Task 5: Execution Handoff

Add tests proving `claim_won -> executing` invokes the existing single-task orchestrator and maps success, already-complete, already-escalated, failure, and cancellation into runtime outcomes.

## Task 6: Recovery, Cooldown, Pause, And Stop

Add tests for deterministic retry scheduling, cooldown, pause, resume, checkpointed stop, and human escalation fallback. Implement recovery transitions without bounded exception-agent execution.

## Task 7: Worker Runtime Loop

Add tests for draining multiple tasks, handling claim contention, respecting idle delay, stopping cleanly, and preserving audit correlation across a run. Implement the runtime loop using the adapters from earlier tasks.

## Task 8: `arm worker run` Command

Add command tests for flags, dry-run behavior, max-task limits, idle delay, worker ID resolution, and exit codes. Implement the Cobra command.

## Task 9: User-Facing Documentation And Diagrams

Update `README.md`, `docs/getting-started.md`, `docs/commands.md`, and `docs/use-cases.md`. The homepage must include a hook, a clear call to action, a high-level Mermaid architecture diagram, and a high-level Mermaid data-flow diagram. Keep diagrams readable; use sequence diagrams only if a short interaction genuinely needs ordering clarity.

## Task 10: Embedded Skill Updates

Update the `armature`, `armature-orchestrator`, and `armature-coordinator` skills so agents know when to use `arm worker run`, how to supervise runtime workers, and when to fall back to `arm orchestrate --issue` or manual worker flow. Confirm whether `armature-worker` remains unchanged as manual fallback.

## Task 11: Integration Verification

Add or update integration-style tests that exercise ready -> claim -> orchestrate through the worker runtime. Run `go test ./...` and `make check` before committing.
```

- [ ] **Step 3: Expand each task into bite-sized TDD steps**

For each task, add explicit steps using this format:

```markdown
- [ ] **Step N: Write the failing test for a concrete behavior**

```go
func TestRuntimeStopsWhenMaxTasksReached(t *testing.T) {
    runtime := NewTestRuntime(t, RuntimeOptions{MaxTasks: 1})
    runtime.QueueReadyTask("TASK-001")
    runtime.QueueReadyTask("TASK-002")

    result := runtime.Run(context.Background())

    require.NoError(t, result.Err)
    assert.Equal(t, 1, result.TasksCompleted)
    assert.Equal(t, StateStopped, result.FinalState)
    assert.True(t, runtime.HasAuditEvent(EventWorkerStopRequested))
}
```

- [ ] **Step N+1: Run the focused test to verify it fails**

Run: `go test ./internal/workerruntime -run TestRuntimeStopsWhenMaxTasksReached -count=1`
Expected: FAIL because `NewTestRuntime`, `RuntimeOptions`, `Run`, or the audit event support does not exist yet.

- [ ] **Step N+2: Implement the minimal code**

Add the smallest production change that satisfies the failing assertion. For
the example above, that means adding `RuntimeOptions.MaxTasks`, returning a
`RunResult` with `TasksCompleted` and `FinalState`, and emitting
`EventWorkerStopRequested` when the max-task limit stops the loop.

- [ ] **Step N+3: Run the focused test to verify it passes**

Run: `go test ./internal/workerruntime -run TestRuntimeStopsWhenMaxTasksReached -count=1`
Expected: PASS.

- [ ] **Step N+4: Commit**

```bash
git add internal/workerruntime
git commit -m "feat: add worker runtime specific behavior"
```
```

The final v1 implementation plan should contain concrete tests of this same
specificity for every task: named behavior, exact file paths, focused command,
expected failure, minimum implementation, expected pass, and commit command.

- [ ] **Step 4: Self-review the implementation plan**

Run:

```bash
rg -n "T[B]D|T[O]DO|pl[a]ceholder|S[i]milar to|appr[o]priate error handling|write t[e]sts for" docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md
```

Expected: no matches. If matches appear, replace them with concrete test names, code snippets, file paths, commands, and expected outcomes.

---

## Chunk 6: Update Runtime Index And Roadmap

### Task 6: Record Phase 4 Completion In Existing Design Docs

**Files:**
- Modify: `docs/design/orchestrate-runtime-index.md`
- Modify: `docs/design/orchestrate-runtime-roadmap.md`

- [ ] **Step 1: Update the runtime index**

In `docs/design/orchestrate-runtime-index.md`, add `orchestrate-runtime-v1-scope.md` to the document list with this summary:

```markdown
- `orchestrate-runtime-v1-scope.md`
  - Phase 4 v1 scope note selecting the thinnest valuable runtime slice,
    included and deferred sub-workflows, CLI posture, audit posture, policy
    posture, exception-agent posture, and acceptance criteria.
```

Also update `Current Position` so it says Phase 4 has selected the v1 runtime slice and points future implementation to `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md`.

- [ ] **Step 2: Update the roadmap Phase 4 status**

In `docs/design/orchestrate-runtime-roadmap.md`, add this status block after the Phase 4 section:

```markdown
### Phase 4 Status

Phase 4 is completed as a docs-first v1 slicing pass:

- `docs/design/orchestrate-runtime-v1-scope.md` selects the thinnest valuable
  v1 runtime slice.
- `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md` decomposes the
  selected slice into implementation work.

Phase 4 intentionally does not add production runtime code, command behavior,
policy parsing, or audit op schemas. Those belong to the v1 implementation
plan.
```

- [ ] **Step 3: Verify the docs reference real files**

Run:

```bash
rg -n "orchestrate-runtime-v1-scope|2026-05-11-orchestrate-runtime-v1" docs/design/orchestrate-runtime-index.md docs/design/orchestrate-runtime-roadmap.md docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md
```

Expected: all references point to files created by this Phase 4 chunk.

---

## Chunk 7: Final Verification And Commit

### Task 7: Verify And Commit Phase 4

**Files:**
- Read: `docs/design/orchestrate-runtime-v1-scope.md`
- Read: `docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md`
- Read: `docs/design/orchestrate-runtime-index.md`
- Read: `docs/design/orchestrate-runtime-roadmap.md`

- [ ] **Step 1: Run red-flag scans**

Run:

```bash
rg -n "T[B]D|T[O]DO|pl[a]ceholder|S[i]milar to|appr[o]priate error handling|write t[e]sts for|m[a]ybe|pr[o]bably|uncl[e]ar" docs/design/orchestrate-runtime-v1-scope.md docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md docs/design/orchestrate-runtime-index.md docs/design/orchestrate-runtime-roadmap.md
```

Expected: no matches.

- [ ] **Step 2: Run documentation consistency checks**

Run:

```bash
rg -n "arm worker run|arm orchestrate|exception-agent|bounded exception|persistent agent|autonomous decomposition|runtime implementation|README.md|hook|call to action|Mermaid|diagram|skill" docs/design/orchestrate-runtime-v1-scope.md docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md docs/design/orchestrate-runtime-index.md docs/design/orchestrate-runtime-roadmap.md
```

Expected: the chosen CLI posture, orchestrator reuse boundary, exception-agent deferral, v1 non-goals, user-facing documentation requirements, homepage hook and call to action, high-level Mermaid diagrams, and embedded skill updates are consistent across all Phase 4 outputs.

- [ ] **Step 3: Run the repo check**

Run:

```bash
make check
```

Expected: lint, test, coverage-check, and mutation stages all pass. If `make check` fails because this is docs-only and an unrelated test is already failing, capture the failing command and output in the final handoff instead of editing unrelated code.

- [ ] **Step 4: Review the final diff**

Run:

```bash
git diff -- docs/design/orchestrate-runtime-v1-scope.md docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md docs/design/orchestrate-runtime-index.md docs/design/orchestrate-runtime-roadmap.md
```

Expected: the diff only contains the Phase 4 docs and plan changes described above.

- [ ] **Step 5: Commit the Phase 4 docs**

Run:

```bash
git add docs/design/orchestrate-runtime-v1-scope.md docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md docs/design/orchestrate-runtime-index.md docs/design/orchestrate-runtime-roadmap.md
git commit -m "docs(plan): phase4 orchestrate runtime v1 slice"
```

Expected: one commit containing only the Phase 4 scope note, v1 runtime implementation plan, and index/roadmap updates.
