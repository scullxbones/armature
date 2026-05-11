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

- deterministic queue polling using the existing ready computation as the first
  poll substrate
- deterministic claim attempts and claim win/loss handling
- execution handoff to the existing single-task orchestrator
- explicit runtime state machine package with `idle`, `polling`,
  `claim_pending`, `claim_lost`, `claim_won`, `executing`, `recovering`,
  `paused`, `escalated`, and `stopped`
- structured gate result types for `poll_gate`, `claim_gate`, `execute_gate`,
  `recovery_gate`, `resume_gate`, and `stop_gate`
- conservative built-in runtime policy defaults for retry, cooldown, worker,
  model, harness, quota, escalation, and sub-workflow posture
- append-only runtime audit events sufficient to reconstruct poll, claim,
  execution, recovery, cooldown, pause, escalation, and stop decisions
- cooldown and pause state materialized from runtime audit events
- human escalation as the fallback for blocked, exhausted, or ambiguous
  recovery

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

- `README.md` homepage hook, concise value proposition, and call to action for
  `arm worker run`
- `README.md` high-level Mermaid architecture diagram
- `README.md` high-level Mermaid data-flow diagram
- `docs/getting-started.md` default quickstart path using `arm worker run`
- `docs/commands.md` command reference for `arm worker run`
- `docs/use-cases.md` persona walkthrough updates for runtime-owned queue
  draining

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

- a worker can run a deterministic queue-draining loop without an LLM-operated
  outer loop
- claim collisions are handled as ordinary deterministic control flow
- no-ready-work and cooldown behavior do not spin aggressively
- successful execution delegates to the existing single-task orchestrator
- runtime state transitions emit repo-visible audit events
- pause, stop, and human escalation states are reconstructable from materialized
  audit state
- no hidden service state is required to understand worker progress
- user-facing docs explain the new default workflow with a homepage hook, call
  to action, and high-level Mermaid architecture and data-flow diagrams
- embedded skills teach agents when to use `arm worker run`, when to supervise
  it, and when to fall back to `arm orchestrate --issue` or manual worker flow
