# Orchestrate Runtime Phase 3 Gap Analysis

## Purpose

This document reconciles the Phase 2 orchestrate worker runtime model with
Armature's current commands, packages, skills, and git-native architecture. It
does not choose the final `v1` implementation slice or make code changes.

Phase 3 asks what the future runtime can reuse, what must change, what must be
added, and what should be deprecated or avoided before Phase 4 chooses the
thinnest valuable `v1`.

## Scope

Included:

- `ready`
- `claim`
- `orchestrate`
- `doctor`
- `validate`
- `dag-summary`
- planner and auditor skills
- existing orchestrator engine and retry logic
- existing op and materialization architecture where relevant to runtime audit

Excluded:

- final runtime CLI naming
- final on-disk policy syntax
- final audit serialization schema
- implementation task decomposition for `v1`
- production code changes

## Executive Summary

The proposed worker runtime fits Armature's current architecture if it is
treated as a deterministic queue-control layer above existing surfaces rather
than a replacement for them. The strongest reuse path is to keep `ready`,
`claim`, and the existing single-task `orchestrate` engine as the first
implementation substrate, then add explicit worker-runtime gates,
policy-result types, and runtime audit records around them.

The main gap is not missing execution machinery; it is missing runtime
ownership. Current commands can be run manually in a loop, but no embedded
runtime owns polling, claim contention, cooldowns, pause/resume, reroutes,
bounded recovery, or state-machine auditability end to end.

## Direct Reuse

| Surface | Current responsibility | Phase 2 runtime fit | Reuse posture |
| --- | --- | --- | --- |
| `arm ready` / `internal/ready` | Deterministically computes claimable work from materialized state, assignment-aware ordering, and stale-claim filtering. | Maps to `poll_gate` candidate discovery and no-ready-work behavior. | Reuse as the initial poll substrate; extend later with explicit DoR/DoE check IDs and structured outcomes. |
| `arm claim` / `internal/claim` | Writes claim ops, rejects inferred nodes, and checks scope overlap with optional human `--force` override. | Maps to `claim_gate`, claim contention, and scope/dependency policy routing. | Reuse claim mechanics; split reusable claim-gate logic from CLI presentation when implementing runtime. |
| `arm orchestrate` / `internal/orchestrate.Engine` | Runs one claimed task through dispatch, verification, retry, escalation, and completion with crash-resume via op replay. | Maps to `executing` state as the single-task executor the worker runtime wraps. | Reuse as execution handoff; do not replace in `v1`. |
| `arm validate` | Checks graph consistency, traceability, coverage, missing task fields, and scope warnings. | Supplies evidence for DoR, provenance review, and release/audit gates. | Reuse as validation evidence; do not make runtime call broad validation blindly on every idle loop without cost controls. |
| `arm doctor` | Checks repo health and stale claims through the D1-D6 diagnostic set. | Supplies operational health evidence for startup, resume, and escalation decisions. | Reuse selected checks as evidence; separate human diagnostics from runtime gates later. |
| `arm dag-summary` | Reviews and promotes draft nodes. | Supports readiness governance before tasks enter the executable pool. | Reuse as human governance surface; runtime should not auto-promote draft work. |
| Planner skill | Creates cited, validated, dependency-aware work DAGs and releases tasks to workers. | Upstream producer of strong DoR inputs. | Reuse as pre-runtime planning lane; runtime should not absorb planner duties in `v1`. |
| Auditor skill | Verifies completed work through citation, source freshness, scope, outcome, and repo-health gates. | Downstream governance and quality gate. | Reuse as post-execution audit lane; runtime can emit evidence but should not replace auditor judgment in `v1`. |

## Required Changes

| Area | Current behavior | Required change before runtime implementation |
| --- | --- | --- |
| Ready classification | Ready queue mostly returns eligible entries or explain strings derived from a small eligibility gate. | Introduce explicit DoR/DoE check IDs and outcome classes: `pass`, `informational`, `policy_evaluable`, `blocked`. |
| Claim gate | Claim CLI owns overlap presentation, inferred-node rejection, and force behavior. | Extract reusable claim-gate decision logic that can return structured outcomes without CLI-only strings. |
| Orchestrator execution | Single-task engine has its own phases and verify-failure retry budget. | Keep it as the execution engine, but wrap it with worker runtime states such as `claim_won`, `executing`, `recovering`, `paused`, `escalated`, and `stopped`. |
| Retry and escalation | Retry budget is command-level and verify-failure oriented. | Align retry classes with runtime policy groups: verification persistence, timeout, quota, harness, model, provenance, and task-contract classes. |
| Audit surface | Existing orchestration ops record dispatch, check results, retry, escalate, and complete. | Add runtime audit records or op payloads for policy evaluation, cooldown, reroute, bounded exception-agent invocation, pause/resume, and human escalation resolution. |
| Policy surface | Current config supports orchestrator defaults, but not the Phase 2 runtime policy groups. | Add an explicit runtime policy model later, with conservative built-in defaults and repo overrides. |
| Command docs | Commands document manual and single-task surfaces. | Add future runtime command docs only after Phase 4 chooses CLI shape. |

## Required Additions

| Addition | Why it is needed | Likely owner |
| --- | --- | --- |
| Worker runtime package | Owns deterministic queue draining and the Phase 2 state machine above existing commands. | New `internal/workerruntime` or similarly named package in Phase 4/implementation. |
| Structured gate result types | Lets `poll_gate`, `claim_gate`, `execute_gate`, `recovery_gate`, `resume_gate`, and `stop_gate` share result semantics. | Runtime package plus supporting internal packages. |
| Policy evaluation types | Represents built-in defaults, tunables, selected policy clauses, and sub-workflow results. | New policy-focused package or runtime subpackage. |
| Runtime audit writer | Emits append-only, repo-visible runtime audit events aligned with the Phase 2 audit model. | Existing ops layer or a new audit adapter, depending on Phase 4 scope. |
| Cooldown and pause state materialization | Allows the worker to avoid aggressive loops and resume safely. | Materialization plus runtime state derivation. |
| Bounded recovery envelope type | Encodes allowed exception-agent actions outside prompts. | Runtime policy and audit integration. |

## Deprecate Or Avoid

| Candidate | Recommendation | Rationale |
| --- | --- | --- |
| Manual shell loops around `arm ready`, `arm claim`, and `arm orchestrate` | Avoid as the long-term runtime strategy. | Shell loops cannot reliably own pause/resume, policy evaluation, cooldowns, and audit correlation. |
| Treating `policy_evaluable` as agent-worthy by default | Deprecate as a design posture. | Phase 2 requires deterministic and policy reasoning before any bounded exception-agent lane. |
| Broad exception-agent supervisors | Keep out of `v1`. | Violates the Phase 1 bounded hybrid runtime boundary. |
| Runtime-driven autonomous decomposition | Keep out of `v1`. | Planner owns decomposition; runtime may detect pressure but should escalate or require upstream scope change. |
| Hidden service state | Avoid. | Armature remains git-native and repo-visible; runtime state must be reconstructable from materialized state and audit records. |

## Contradictions And Tensions

| Tension | Why it matters | Phase 3 conclusion |
| --- | --- | --- |
| `ready` currently knows less than the Phase 2 DoR/DoE matrix. | Runtime needs structured check outcomes, not just queue inclusion. | This is an implementation gap, not an architecture contradiction. |
| `claim --force` allows human override of overlap warnings. | Runtime must not silently choose risky overlap behavior. | Runtime should route overlap ambiguity through policy/sub-workflow or escalation, not copy CLI force semantics. |
| Existing `orchestrate` escalation is retry-budget exhaustion. | Phase 2 human escalation is broader and more structured. | Preserve current engine escalation as one evidence source; add runtime-level escalation semantics later. |
| `doctor` and `validate` are human- and CI-facing commands. | Runtime needs cheap, targeted gates and cannot run expensive diagnostics indiscriminately. | Reuse check logic selectively instead of shelling out to every command on every loop. |
| Current orchestration ops are task-execution oriented. | Phase 2 audit model needs worker-run and policy-decision correlation. | Add runtime audit events in a future implementation slice. |

## Command Surface Gap

| Option | Fit | Tradeoff |
| --- | --- | --- |
| New `arm worker run` command | Cleanly separates queue-draining runtime from single-task `arm orchestrate`. | Adds a new top-level concept and docs burden. |
| Expanded `arm orchestrate --loop` mode | Reuses the existing command name users already know. | Risks conflating single-task execution with worker lifecycle ownership. |
| Temporary internal runtime with no public command | Lets implementation evolve behind tests before CLI choice. | Delays dogfooding and may hide product-shape decisions too long. |

Phase 3 does not choose among these. Phase 4 should choose based on the
thinnest valuable `v1` and the desired user mental model.

## Recommended Phase 4 Inputs

Phase 4 should use this gap analysis to decide:

- whether `v1` exposes a new command or expands `arm orchestrate`
- which DoR/DoE checks become structured runtime gates first
- whether runtime audit events are new op types or structured payloads on an existing audit lane
- which policy groups are needed in the first implementation slice
- whether bounded exception-agent execution is included in `v1` or deferred behind deterministic hooks
- how much of `doctor` and `validate` becomes reusable internal check logic versus human-facing command output
