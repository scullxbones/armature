# Orchestrate Runtime Go OSS Review

## Purpose

This document evaluates Go libraries that could influence or support the future
orchestrate worker runtime. The review is intentionally scoped to Phase 3: it
recommends an implementation posture, but does not select the `v1` runtime
slice or introduce dependencies.

## Evaluation Criteria

| Criterion | Meaning |
| --- | --- |
| Embedded fit | Can it run inside the `arm` binary without requiring a server, daemon, database, or distributed control plane? |
| Deterministic state-machine friendliness | Does it make explicit state transitions and replayable decisions straightforward? |
| Auditability | Can Armature keep repo-visible, append-only audit records as the source of truth? |
| Operational complexity | Does it preserve Armature's single-binary, git-native deployment posture? |
| Maintenance | Is the project active and credible enough to depend on or borrow from? |
| Architecture fit | Does it complement Armature's existing commands, op logs, materialization, and worker model? |

## Sources Reviewed

Maintenance notes below reflect the official sources reviewed on 2026-05-10.
Release recency is used only as a lightweight maintenance signal, not as a full
project-health audit.

- Temporal Go SDK README and releases: <https://github.com/temporalio/sdk-go>
- Temporal Go SDK docs: <https://docs.temporal.io>
- Cadence Go client README and releases: <https://github.com/cadence-workflow/cadence-go-client>
- go-workflows docs: <https://cschleiden.github.io/go-workflows/>
- go-workflows repository and releases: <https://github.com/cschleiden/go-workflows>
- River README and releases: <https://github.com/riverqueue/river>
- River documentation: <https://riverqueue.com>
- Watermill README and releases: <https://github.com/ThreeDotsLabs/watermill>
- Watermill documentation: <https://watermill.io>
- stateless README and releases: <https://github.com/qmuntal/stateless>
- looplab/fsm README and releases: <https://github.com/looplab/fsm>

## Candidate Summary

| Candidate | Category | Embedded fit | Operational complexity | Recommendation |
| --- | --- | --- | --- | --- |
| Temporal Go SDK | Distributed workflow engine | Low | High | Do not integrate for `v1`; borrow workflow-history and retry vocabulary where useful. |
| Cadence Go client | Distributed workflow engine | Low | High | Do not integrate; similar mismatch to Temporal with weaker strategic fit. |
| go-workflows | Go-native workflow engine | Medium | Medium | Consider only as pattern research unless Phase 4 demands durable replay semantics beyond Armature ops. |
| River | Durable job queue | Low to medium | Medium | Do not integrate for runtime control; database-backed jobs conflict with git-native state. |
| Watermill | Event/message workflow toolkit | Low to medium | Medium | Do not integrate for `v1`; messaging abstractions are broader than Armature needs. |
| stateless | In-process finite state machine | High | Low | Selectively borrow state-machine structure if it remains maintained and dependency risk is acceptable. |
| looplab/fsm | In-process finite state machine | High | Low | Selectively borrow concepts or evaluate as a small dependency; avoid if hand-rolled transitions stay clearer. |

## Detailed Notes

### Temporal Go SDK

Temporal describes itself as a distributed, scalable, durable, highly available
orchestration engine with a Go SDK for workflows and activities. That is a
strong fit for service-backed durable execution, but a weak fit for Armature's
single-binary and repo-visible state model.

Maintenance signal: the GitHub release page showed `v1.43.0` dated
2026-04-30 when reviewed on 2026-05-10, so current activity appears healthy.

Phase 4 posture: do not integrate. Borrow concepts such as durable event
history, replay-safe workflow decisions, retry policy vocabulary, and activity
boundaries if useful.

### Cadence Go Client

Cadence positions itself similarly: a framework for workflows and activities on
top of a distributed orchestration engine. That keeps its core strengths close
to Temporal's and its architectural mismatch with Armature similarly high.

Maintenance signal: the GitHub release page showed `v1.3.0` dated 2025-07-08
when reviewed on 2026-05-10. That is still maintained enough to study, but it
looks less strategically aligned than Temporal for future pattern borrowing.

Phase 4 posture: do not integrate.

### go-workflows

`go-workflows` is much closer to Armature's target shape because it is framed as
an embedded durable workflow engine written in Go. It still introduces a
workflow-runtime abstraction, pluggable backends, and non-determinism rules
borrowed from Temporal and Cadence, so it is not a free architectural fit.

Maintenance signal: the GitHub release page showed `v1.4.1` dated 2025-11-01
when reviewed on 2026-05-10. The official docs also describe multiple backends
such as SQLite, MySQL, PostgreSQL, and Redis, which is useful pattern material
but wider than Armature's current git-native persistence.

Phase 4 posture: selectively borrow ideas only unless the `v1` slice explicitly
requires a workflow engine abstraction beyond append-only ops and deterministic
state derivation.

### River

River is a durable job queue for Go and Postgres. Its README emphasizes
transactional enqueueing in the same database as application data, which is a
solid ordinary-service posture but a mismatch for Armature's repo-native state
coordination.

Maintenance signal: the GitHub release page showed `v0.36.0` dated 2026-05-10
when reviewed on 2026-05-10, which suggests active maintenance. Even so, its
database-backed queue identity conflicts with Armature's source of truth.

Phase 4 posture: do not integrate for `v1`.

### Watermill

Watermill is a message-stream and pub/sub toolkit for event-driven systems. Its
strength is flexibility across Kafka, RabbitMQ, SQL, NATS, Redis, HTTP, and
other transports, plus patterns like sagas and CQRS. That is broader than the
local deterministic worker runtime Armature is trying to add.

Maintenance signal: the GitHub release page showed `v1.5.1` dated 2025-09-02
when reviewed on 2026-05-10. It appears active and credible, but its main value
is for message topology, not embedded repo-visible runtime control.

Phase 4 posture: do not integrate for `v1`.

### stateless

`stateless` is an in-process state-machine library that explicitly supports
state machines and lightweight state-machine-based workflows directly in Go
code. Its support for external state storage, guarded transitions, hierarchical
states, and graph export aligns more closely with the Phase 2 worker state
machine than the distributed workflow engines do.

Maintenance signal: the GitHub release page showed `v1.8.0` dated 2026-02-10
when reviewed on 2026-05-10, which is a healthy sign for a small library.

Phase 4 posture: consider as a pattern or small dependency only after comparing
against a direct typed transition table.

### looplab/fsm

`looplab/fsm` is a small in-process finite-state-machine library. Its event and
callback model could map to the runtime state catalog, transition validation,
and transition hooks, but Armature may still be clearer with direct typed
transitions because audit records and op-log state derivation are
domain-specific.

Maintenance signal: the GitHub release page showed `v1.0.3` dated 2025-05-07
when reviewed on 2026-05-10. That is less recent than `stateless`, but still
recent enough to treat as live pattern material.

Phase 4 posture: consider as a pattern or small dependency; prefer direct code
unless the implementation becomes transition-boilerplate heavy.

## Recommendation

Build the Phase 4 `v1` runtime directly, while selectively borrowing patterns
from small in-process state-machine libraries and distributed workflow systems.

Do not integrate a distributed workflow engine for `v1`. Temporal, Cadence,
River, and Watermill solve larger service-infrastructure problems than Armature
currently has, and they conflict with the single-binary, git-native,
repo-visible state model.

The most promising implementation path is:

1. encode the Phase 2 worker states and transitions directly in Go
2. persist runtime decisions through Armature ops or runtime audit records
3. keep materialized state derivable from repo-visible artifacts
4. revisit a small FSM dependency only if direct transition code becomes noisy
5. borrow retry, replay, and activity-boundary language from mature workflow
   engines without inheriting their infrastructure model

## Phase 4 Decision Inputs

Phase 4 should decide:

- whether direct typed transitions are sufficient for `v1`
- whether a small FSM dependency is worth adding
- whether runtime audit events require a new package or can sit beside existing ops
- whether durable replay needs remain satisfied by existing op materialization
- whether any library should be vendored, depended on, or only cited as pattern research
