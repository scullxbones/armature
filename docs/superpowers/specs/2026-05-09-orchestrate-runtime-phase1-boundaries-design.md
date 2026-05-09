# Orchestrate Runtime Phase 1 Boundary Design

**Date:** 2026-05-09
**Status:** Approved

---

## Overview

This document captures the approved Phase 1 boundary design for the Armature
orchestrate runtime work. Its purpose is to stabilize the design surface before
implementation planning by locking the product edges for `v1` without forcing
early decisions on CLI naming, full state-machine detail, or complete audit
schema design.

Phase 1 should produce a short boundary decision note appended to
`docs/design/orchestrate-runtime-direction.md`. This spec defines the content
that note should freeze.

---

## Phase 1 Intent

Phase 1 is intentionally boundary-first. It should answer:

- what the runtime is allowed to do in `v1`
- what must remain deterministic
- when an exception agent may run
- what is explicitly out of scope

It should not attempt to fully specify the runtime control model, complete
recovery taxonomy, or final command surface.

---

## Boundary Decisions

### CLI Surface

The runtime CLI surface remains intentionally undecided in Phase 1.

Phase 1 should lock runtime semantics first and defer the naming choice between
shapes such as `arm worker run` and `arm orchestrate --loop` until later design
work. Future CLI decisions must fit the semantic boundary established here
rather than reopening it.

### V1 Runtime Posture

`v1` should be a value-first bounded hybrid runtime.

Its purpose is not only to prove architectural coherence, but to deliver
practical end-user value by draining clearly executable work without requiring a
human-operated loop. Architecture still matters, but user-visible queue-draining
value takes priority.

### V1 Product Promise

In `v1`, Armature should be able to run a worker that:

- continuously identifies clearly executable work
- claims work deterministically
- invokes the existing single-task orchestrator as the execution engine
- handles routine queue-control behavior without spending agent tokens
- makes one bounded recovery attempt for selected policy-evaluable failures
- escalates unresolved or overly ambiguous situations with audit traceability

The new runtime wraps the existing single-task orchestrator. It does not replace
that orchestrator in `v1`.

---

## Deterministic Core

The following `v1` responsibilities must remain strictly deterministic and
outside exception-agent discretion:

- queue polling
- claim attempts
- claim win/loss handling
- retry and backoff behavior
- worker routing
- provenance refresh
- Definition of Executability fallback choice when policy already defines a safe
  option

These behaviors are part of the runtime control plane and should be enforced by
deterministic rules and policy evaluation, not by agent judgment.

---

## Exception-Agent Lane

### Purpose

The exception-agent lane is allowed in `v1`, but only as a narrow recovery
mechanism. It is not a second general execution mode, not a persistent
supervisor, and not a planner that may reinterpret tasks.

Its purpose is to help recover from selected `policy_evaluable` situations after
deterministic rules or configured policy cannot safely choose among a small set
of permitted actions.

### Guardrails

The `v1` exception lane must follow these guardrails:

- entry is allowed only from explicitly enumerated recovery cases
- at most one bounded recovery attempt is allowed per triggering condition
  before escalation or cooldown
- recovery actions must stay within a policy-approved action envelope
- the agent may choose among permitted recovery actions and explain its choice
- the agent may not broaden scope, rewrite task intent, or decompose work
  autonomously
- every invocation must emit an audit record with trigger, options considered,
  chosen action, and escalation outcome if any

### Policy Enforcement Envelope

In `v1`, exception-agent safety must be enforced by deterministic controls
outside the agent, not by prompt-only behavioral guidance.

Prompts may restate the boundary, but they are not the security mechanism and
must not be treated as the policy boundary. The real enforcement surface is the
runtime's hard control layer: constrained permissions, a fixed allowed-action
contract, deterministic entry conditions, and deterministic validation of any
proposed recovery action.

The enforcement requirements are:

- the exception agent runs under a deterministic permission profile selected by
  policy
- the permission profile must be narrow enough to make out-of-scope actions
  impossible or rejectable by the runtime
- prompts may describe the policy, but policy enforcement must live in runtime
  controls outside the agent
- the runtime must not dynamically widen permissions during a recovery attempt
- if the bounded permission profile is insufficient, the runtime escalates
  instead of widening scope
- any proposed recovery action must pass deterministic validation against the
  allowed-action contract before the runtime accepts it
- the audit record must capture the permission profile, allowed-action contract,
  and validation outcome

---

## Explicit V1 Non-Goals

The following are out of scope for `v1`:

- deciding the final CLI surface
- persistent agent supervisors
- autonomous decomposition or replanning
- broad semantic rewriting of tasks
- distributed workflow infrastructure

These exclusions are part of the Phase 1 boundary and should be treated as
design constraints for later phases.

---

## Deferred By Design

This boundary note intentionally does not settle:

- the final CLI command shape
- the full worker runtime state machine
- the complete audit schema
- the full list of exception-agent recovery cases
- the detailed policy configuration surface

Those topics should be designed in later phases, but only within the boundary
defined by this document.

---

## Acceptance Test For The Boundary Note

The Phase 1 boundary output is good enough when a reader can answer the
following without inferring intent from multiple documents:

- what is this runtime allowed to do in `v1`?
- what must remain deterministic?
- when may an exception agent run?
- what is definitely out of scope?

If the note does not make those answers obvious, the Phase 1 boundary work is
not yet complete.
