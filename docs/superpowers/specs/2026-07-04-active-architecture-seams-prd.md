# Active Architecture Seams PRD

Date: 2026-07-04

## Problem Statement

Armature has four active architecture seams where callers still need to know
too much implementation detail.

First, recording a Conformance Assessment is trust-critical but its rules are
orchestrated by the command. The caller must order structural validation,
Review Bundle identity checks, delivery and Task Contract fingerprint checks,
diff-citation validation, criterion coverage validation, duplicate detection,
Assessment Attestation construction, and op emission correctly. The existing
review module contains the individual rules, but the protocol that composes
them is not yet behind one deep interface.

Second, the Source lifecycle is split between command orchestration and shallow
manifest, cache, fingerprint, and provider functions. Callers understand the
storage choreography and independently interpret Source freshness. This weakens
locality for a core provenance concept used by planning, Render Context,
validation, stale review, and Harness Hook policy.

Third, Command Runtime work previously established explicit command execution
state, but process-global command state is present again alongside the explicit
seam. Command handlers can therefore obtain repository context and push posture
through two sources of truth. This regresses test isolation and makes the
runtime interface wider than the behavior it protects.

Fourth, Repository Snapshot work established a canonical module, but current
callers still choose among direct loading, cached loading, refresh, and direct
materialized-file reads. The module exposes freshness and storage policy to
callers, and its architecture guard explicitly cannot detect every premature
materialization path.

From the user's perspective, these seams create avoidable risk:

- semantic-review trust rules can drift when new evidence or validation is added
- Source freshness can be interpreted differently by different consumers
- commands and tests can observe shared process state from another execution
- read paths can trigger unnecessary materialization or consume stale state
- completed architecture work can regress without an enforceable highest seam

## Solution

Deepen four existing modules while preserving Armature's user-facing behavior:

1. Make Conformance Recording the single review decision seam for validating an
   assessment against its Task Contract, optional Review Bundle, existing
   attestations, and admissible evidence before producing a recording decision.
2. Make Source Lifecycle the single seam for registration, synchronization,
   cached content, fingerprints, and freshness, with filesystem and remote
   providers remaining adapters.
3. Restore Command Runtime as the only source of command execution state and
   remove process-global repository, pusher, and tracker state.
4. Collapse Repository Snapshot access modes so current repository truth hides
   validated-op loading, materialization, storage paths, caching, and freshness
   policy behind one seam; historical truth remains separate.

These changes should increase leverage for commands and policy consumers while
concentrating change, bugs, and verification inside the modules that own the
domain behavior.

## User Stories

1. As an Armature reviewer, I want every Conformance Assessment checked by one recording protocol, so that the same trust rules apply regardless of how recording is invoked.
2. As an Armature reviewer, I want a mismatched Review Bundle rejected before an Assessment Attestation is emitted, so that evidence cannot be recorded against the wrong Assessable Delivery.
3. As an Armature reviewer, I want invalid diff citations rejected through the review module, so that citation integrity does not depend on command call order.
4. As an Armature reviewer, I want every Task Contract criterion accounted for, so that incomplete assessments cannot be recorded as if they were complete.
5. As an Armature coordinator, I want duplicate assessments recognized idempotently, so that retries do not create duplicate attestations.
6. As an Armature maintainer, I want Assessment Attestation construction to follow validated assessment state, so that new review rules have one place to integrate.
7. As an Armature maintainer, I want CLI file handling and output formatting kept outside Conformance Recording, so that trust rules remain independent of Cobra.
8. As an Armature maintainer, I want Execution Evidence additions to extend one review seam, so that ADR-0008 remains enforceable as the evidence protocol grows.
9. As an Armature planner, I want Source registration and freshness to have one owner, so that cited plans rely on consistent provenance state.
10. As an Armature worker, I want cached Source content and its fingerprint to be read together, so that Render Context does not consume content with ambiguous freshness.
11. As an Armature auditor, I want Source verification to use the same lifecycle semantics as synchronization, so that OK, STALE, MISSING, and CHANGED have one meaning.
12. As an Armature maintainer, I want Source provider variation behind a real seam, so that filesystem, Confluence, and SharePoint behavior can vary without leaking into callers.
13. As an Armature maintainer, I want partial Source synchronization failures represented consistently, so that every consumer sees the same last-known state.
14. As an Armature maintainer, I want Source lifecycle tests to use local adapters, so that freshness and failure behavior are deterministic.
15. As an Armature command author, I want one Command Runtime seam, so that I do not choose between explicit state and globals.
16. As an Armature test author, I want independent root commands to hold independent runtime state, so that command tests can run without shared-state leakage.
17. As an Armature maintainer, I want repository context, worker identity, push posture, and Snapshot access assembled once per execution, so that setup behavior has one owner.
18. As an Armature maintainer, I want an architecture guard against process-global command state, so that the completed runtime migration cannot silently regress again.
19. As an Armature command author, I want current repository truth through one Snapshot seam, so that I do not choose materialization timing or construct state paths.
20. As an Armature worker, I want `arm show`, `arm ready`, Render Context, and Harness Hook policy to agree on current Issue state, so that workflow decisions use one truth.
21. As an Armature maintainer, I want read-only lookups to avoid accidental rematerialization, so that performance and side effects are predictable.
22. As an Armature maintainer, I want Snapshot warnings preserved across every caller, so that excluded or invalid Ops remain visible.
23. As an Armature maintainer, I want historical state access distinct from current state access, so that time-travel behavior does not widen the ordinary Snapshot interface.
24. As an Armature maintainer, I want tests to cross the same highest interfaces as production callers, so that internal refactors do not invalidate behavior tests.
25. As an Armature planner, I want each deepening slice independently claimable and verifiable, so that workers can deliver narrow end-to-end improvements.
26. As an Armature coordinator, I want overlapping architecture slices explicitly ordered, so that parallel work does not collide in shared commands or guards.
27. As an Armature maintainer, I want prior ARCHIMP outcomes preserved as prior art, so that follow-up work describes the present regression or remaining friction rather than rewriting history.
28. As an Armature maintainer, I want the full repository gate green after every slice, so that architectural depth does not trade away behavior or governance.

## Implementation Decisions

- The work remains under the existing `ARCHIMP` epic. It does not create a new architecture epic or modify the completed outcomes of earlier stories.
- The four recommendations are implemented as independently verifiable vertical slices under one new story, with dependencies only where shared command surfaces require ordering.
- Conformance Recording deepens the existing review module rather than adding a parallel review package.
- The Conformance Recording interface owns the decision to reject, report an idempotent duplicate, or produce an Assessment Attestation. It receives already-decoded inputs and current Issue review state; the CLI remains responsible for stdin/file decoding, output rendering, and appending the resulting Op.
- Conformance Recording preserves ADR-0005: semantic review remains advisory and skill-driven. It does not run a model or gate delivery.
- Conformance Recording preserves ADR-0008: Execution Evidence remains upgrade-only and cannot suppress diff-supported contradictions.
- Source Lifecycle deepens the existing sources module. It owns Source registration, manifest persistence, content caching, fingerprint calculation, synchronization state, and freshness evaluation.
- Filesystem, Confluence, and SharePoint remain adapters at the Source provider seam. Provider-specific transport and credentials do not enter the Source Lifecycle interface.
- Consumers request registered Source facts or cached content through Source Lifecycle rather than opening manifest and cache files independently.
- Source fingerprint Ops remain durable provenance facts. The command runtime remains responsible for appending Ops after a successful lifecycle decision.
- Command Runtime remains an in-process seam associated with the Cobra execution context. All command handlers obtain execution state through that seam only.
- Process-global repository context, pusher, and tracker variables are removed. No compatibility fallback is retained.
- Repository Snapshot deepens the existing snapshot module. It owns validated Op loading, materialization, warnings, state paths, cached current truth, and the distinction between read-only access and refresh.
- Current repository truth and historical repository truth remain distinct seams. Historical materialization is not folded into the common current-state interface.
- The Snapshot slice extends architecture enforcement so direct loading and premature materialization cannot reappear unnoticed in command or TUI callers.
- The Source Lifecycle and Conformance Recording slices should land before any shared-file cleanup that depends on their final command shape.
- The Command Runtime hardening slice lands last because it sweeps the widest command surface and should consume the final shapes established by the other slices.
- No user-facing command names, flags, JSON fields, statuses, exit codes, or output meanings change as part of this work.

## Testing Decisions

- Good tests cross the highest interface used by callers and assert observable decisions or state. Tests must not depend on helper call order or internal storage layout.
- Conformance Recording tests exercise rejection, duplicate, and attestation outcomes through one review interface. Coverage includes Issue mismatch, bundle and delivery fingerprint mismatch, Task Contract mismatch, citation-coordinate failure, missing criterion coverage, and valid recording.
- Existing review command tests remain as a small adapter contract for stdin/files, output format, and Op persistence. Protocol permutations move to focused review-module tests.
- Source Lifecycle tests exercise register, synchronize, verify, stale-after-failure, missing cache, changed cache, and partial success behavior through one interface.
- Source provider tests use the existing filesystem adapter and fake HTTP client prior art. Tests do not require live remote providers.
- Command Runtime tests create independent root commands with distinct repositories and execution dependencies, then prove there is no cross-command state leakage.
- An architecture guard fails when production command code declares or reads process-global execution state.
- Repository Snapshot tests prove current-truth consistency, warning preservation, read-only behavior, refresh behavior, and empty-repository behavior through the Snapshot interface.
- Command-level regression tests prove `show`, `ready`, Render Context, review, and Harness Hook consumers observe equivalent current Issue truth where their behavior overlaps.
- New requirement tests use the `_REQ_<ISSUE_ID>` naming convention so `make trace-report` can connect delivery evidence to the Issue contract.
- Every slice must pass focused package tests, `make check`, `arm validate --ci`, and `arm doctor` before completion.

## Out of Scope

- Changing the advisory status, rating algebra, reviewer role, or skill-driven execution model of semantic conformance review.
- Implementing the Activity Log, Activity Index, or Execution Evidence expansion described by ADR-0008.
- Adding a new Source provider, changing provider authentication, or introducing network-backed test infrastructure.
- Redesigning Source citation acceptance, traceability policy, or Source Link semantics.
- Reworking DAG graph facts, Issue type hierarchy, Ready Queue policy, claim policy, or materialization replay semantics.
- Changing Harness Hook binding, scope enforcement, platform adapters, or the in-progress worktree-path fixes on the current branch.
- Changing the persisted Op schema, materialized Issue schema, or Assessment Attestation schema unless a later implementation review proves it unavoidable.
- Reopening or editing completed `ARCHIMP` stories or their recorded outcomes.
- Broad command cleanup unrelated to execution-state ownership.
- User-interface redesign or new command output formats.

## Further Notes

- This PRD is based on the live codebase reviewed on 2026-07-04. The review deliberately distinguished new opportunities from regressions after completed ARCHIMP work.
- Conformance Recording is the recommended first slice because it concentrates the newest trust-critical protocol before evidence inputs expand.
- Source Lifecycle can proceed independently of Conformance Recording.
- Repository Snapshot should follow Conformance Recording where both affect review command state loading.
- Command Runtime should close the sequence after Source Lifecycle and Repository Snapshot so its broad command sweep does not churn concurrently with those slices.
- The current branch contains unrelated in-progress Harness Hook and claim changes. Planning artifacts must not alter or absorb those edits.
