# Orchestrate Runtime Phase 1 Decision Record Design

**Date:** 2026-05-09
**Status:** Approved

---

## Overview

This document captures the approved design for Phase 1 step 2 of the orchestrate
runtime roadmap: creating a compact decision record from the current runtime
brainstorming notes.

The decision record should be a standalone design document at
`docs/design/orchestrate-runtime-decisions.md`. Its job is to summarize the
current design state after Phase 1 step 1 without reopening the product
boundary, introducing runtime implementation details, or settling deferred
questions early.

---

## Goal

Create a concise, durable decision ledger that future runtime planning can use
to answer:

- what has already been decided
- what has been derived from those decisions
- what the design currently assumes
- what remains open
- what is explicitly deferred
- where the rationale lives

The ledger should reduce context load for future work while keeping the deeper
design notes as the source of rationale.

---

## Artifact Shape

The output should be a standalone Markdown document:

```text
docs/design/orchestrate-runtime-decisions.md
```

The document should be compact and scannable. It should not become a narrative
essay, a full architecture decision record, or a planning document.

The existing index may be updated to point at the new decision record if that
improves discoverability, but the canonical step 2 artifact is the standalone
ledger.

---

## Ledger Structure

### Locked Decisions

This section should list decisions already made directly in the source design
notes.

Expected topics include:

- Armature should move toward an embedded deterministic worker runtime.
- The runtime should wrap the existing single-task orchestrator rather than
  replace it in `v1`.
- The final CLI surface remains undecided.
- `v1` is a value-first bounded hybrid runtime.
- Routine queue draining, claim handling, retry, backoff, worker routing, and
  provenance refresh remain deterministic.
- Definition of Ready and Definition of Executability are separate concerns.
- DoR/DoE outcomes should use `pass`, `informational`, `policy_evaluable`, and
  `blocked`.
- Exception agents are allowed only as a narrow bounded recovery mechanism.
- Human escalation should remain narrower than the exception-agent lane.
- Policy-evaluable conditions should flow through reusable sub-workflows rather
  than bespoke one-off workflows.

Each item should include a short source note.

### Derived Decisions

This section should capture synthesis that follows from the source notes but is
not stated as a single explicit sentence in one source document.

Derived decisions must be labeled clearly so future readers can distinguish
them from already-locked source decisions.

Expected topics include:

- `policy_evaluable` does not automatically mean agent-worthy.
- Normal runtime control-plane outcomes are not exceptions.
- Deterministic policy resolution should be attempted before invoking an
  exception agent.
- Prompt instructions are not the policy boundary; runtime controls are.
- A shared sub-workflow result shape is part of keeping runtime integration
  simple.

Derived decisions should remain conservative. They should not introduce new
state-machine, audit-schema, CLI, or implementation decisions.

### Assumptions

This section should list design assumptions that later planning may need to
validate.

Expected topics include:

- Existing `ready`, `claim`, and `orchestrate` surfaces can provide enough
  reuse for a thin `v1` runtime.
- Existing git-native coordination remains the right persistence and audit
  foundation.
- DoR/DoE checks can be made enforceable without turning every ambiguous task
  quality concern into a human review.
- A small set of reusable policy sub-workflows is sufficient for the first
  runtime planning pass.
- One bounded recovery attempt is enough for selected `v1` exception-agent
  cases before escalation or cooldown.

### Open Questions

This section should list questions that must still be answered by later roadmap
steps before implementation planning is complete.

Expected topics include:

- exact DoR and DoE signals
- owning command or subsystem for each check
- when each check runs
- exact policy outcomes for each check
- complete inputs, deterministic rules, policy knobs, and outputs for each
  reusable sub-workflow
- worker runtime state machine and transitions
- audit event schema
- policy configuration surface
- full list of exception-agent recovery entry conditions

### Deferred Questions

This section should list topics intentionally postponed by the current roadmap
or the Phase 1 boundary note.

Expected topics include:

- final CLI command shape
- persistent agent supervisors
- autonomous decomposition or replanning
- broad semantic task rewriting
- distributed workflow infrastructure
- Go OSS embedded workflow library evaluation
- final v1 implementation slice

### Source Notes

This section should map the ledger back to the source documents:

- `docs/design/orchestrate-runtime-direction.md`
- `docs/design/orchestrate-readiness-and-executability.md`
- `docs/design/orchestrate-exception-taxonomy.md`
- `docs/design/orchestrate-policy-subworkflows.md`
- `docs/design/orchestrate-runtime-roadmap.md`
- `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

The source notes should be short. They should help future readers find
rationale without duplicating the source documents.

---

## Constraints

- Do not make a final CLI decision.
- Do not design the runtime state machine.
- Do not design the audit schema.
- Do not define the detailed policy configuration model.
- Do not expand the exception-agent lane beyond the approved Phase 1 boundary.
- Do not turn the ledger into an implementation plan.
- Keep derived decisions visibly separate from locked decisions.

---

## Acceptance Criteria

The decision record is complete when:

- it exists as `docs/design/orchestrate-runtime-decisions.md`
- it separates locked decisions from derived decisions
- it lists assumptions, open questions, and deferred questions
- each decision has a concise source note
- it preserves the Phase 1 boundary note without broadening scope
- a future reader can use it to begin roadmap step 3 without rereading every
  source note first

---

## Out Of Scope

This step does not implement the decision record itself. It only defines the
approved design for that implementation slice.

The actual decision record should be created in the follow-up implementation
plan for Phase 1 step 2.
