# Deterministic Quality Guardrails Design

Date: 2026-06-13

## Summary

Armature will adopt deterministic quality guardrails inside its own repository
and CI, with the design centered on one architectural extension: deepen
existing source-level provenance into requirement-level provenance.

Armature already tracks source registrations, source links, citation
acceptance, and traceability coverage at the issue graph level. This design
does not replace that system. It extends it so source-backed specifications can
declare stable requirement IDs, Armature can materialize those requirement
references, and issues can link to specific requirement references rather than
only to whole documents.

This design intentionally separates two phases:

1. Phase 1: source-backed requirement traceability inside Armature.
2. Phase 2: end-to-end requirement verification evidence reconciliation
   (`requirement -> issue -> verification evidence`).

Phase 1 is the implementation target of this spec. Phase 2 is a reserved
extension path, not hidden scope.

## Goals

- Adopt deterministic quality guardrails for the `armature` repo itself rather
  than treating the recommendations as generic guidance only.
- Reuse and deepen Armature's existing source-link and traceability model
  instead of introducing a second provenance system.
- Parse stable requirement IDs from registered source documents and materialize
  them as first-class requirement references.
- Allow issues to link to requirement references with deterministic validation
  and coverage reporting.
- Expose exact requirement references in worker-facing context so execution is
  grounded in specific spec clauses.
- Preserve a clean future path to full verification-evidence reconciliation.
- Capture the new domain language in `CONTEXT.md` as part of the implementation
  work derived from this spec.

## Non-Goals

- Do not implement phase-2 verification evidence ingestion in this slice.
- Do not claim that requirement linkage alone proves semantic correctness or
  implementation completeness.
- Do not replace existing whole-source `source-link` behavior for legacy or
  non-structured documents.
- Do not create a parallel traceability subsystem disconnected from current
  sources, source links, and coverage projections.
- Do not update `CONTEXT.md` or code as part of writing this spec.

## Current State

Armature already has deterministic provenance primitives:

- `arm sources add/sync/verify` registers authoritative source documents.
- `arm source-link` links issues to registered sources.
- `arm accept-citation` records explicit accepted-risk provenance.
- `traceability.json` and `arm validate` compute and report issue citation
  coverage.

That means the repo already owns source-level traceability. The missing layer is
requirement semantics inside those sources. Today a source-backed issue can be
correctly linked to a document while still remaining ambiguous about which
specific requirements it implements.

The design therefore extends the existing model from:

- `source -> issue`

to:

- `source -> requirement reference -> issue`

and later:

- `source -> requirement reference -> issue -> verification evidence`

## Design Principles

- Build on existing Armature seams. New requirement traceability must deepen
  source-backed provenance, not bypass it.
- Keep raw authority and derived projections separate. Cached source content is
  the authority; requirement references and coverage are materialized views.
- Preserve deterministic validation. Unknown IDs, orphaned mappings, and stale
  references must fail or warn through machine-checkable rules.
- Keep phase boundaries honest. Phase 1 is planning and provenance. Phase 2 is
  verification evidence reconciliation.
- Teach the vocabulary explicitly. New concepts must enter the repo's canonical
  glossary rather than remaining implied in implementation details.

## New Vocabulary

The implementation derived from this spec must update `CONTEXT.md` with, at
minimum, the following terms:

- `Requirement ID`: a stable identifier authored in a registered source
  document, such as `REQ-123`.
- `Requirement Reference`: Armature's normalized record tying a requirement ID
  to a specific registered source and its resolved location within that source.
- `Requirement Coverage`: the derived view showing which requirement references
  are linked to issues, unlinked, stale, or otherwise unresolved.
- `Verification Evidence`: a reserved future term for machine-readable
  execution or test evidence linked to requirement references in phase 2.

The glossary update is part of the implementation work for this design, not a
follow-up documentation clean-up.

## Architecture

Phase 1 adds a requirement-traceability layer on top of the current sources and
traceability system.

### Authority And Projections

- Registered sources remain the authoritative document artifacts.
- Cached source content remains the raw input used for synchronization and
  provenance.
- Requirement references are derived metadata extracted from cached sources.
- Requirement coverage is a derived projection computed from requirement
  references plus issue mappings.

This follows Armature's existing pattern: append-only facts and cached source
content remain the source of truth, while coverage and reconciliation stay in
materialized state.

### Ownership Boundary

The design splits ownership across three seams:

1. `sources` owns document registration, syncing, and cached source authority.
2. requirement traceability owns extraction, normalization, and reconciliation
   of requirement references derived from those sources.
3. issue provenance owns which requirement references justify a given issue.

The current whole-source citation path remains valid. Requirement-level
traceability is an extension for structured specs, not a mandatory replacement
for every source document in the repository.

## Data Model

### Requirement References

A requirement reference must be materialized from a registered source document
that declares stable IDs. The normalized record should carry enough identity to
survive source syncs and ordinary document edits without collapsing back to a
plain text search.

The exact storage shape is an implementation detail, but the model must support:

- source identity
- requirement ID
- resolved source location or anchor
- extracted text or summary needed for context display
- source fingerprint or other sync-era reconciliation metadata

### Issue Mapping

Issues must be able to link to one or more requirement references explicitly.

This should not be modeled as an accidental side effect of whole-source
`source-link`. Linking an issue to a source document and linking an issue to a
specific requirement within that document are related but different semantics.

The implementation should therefore prefer one of these approaches:

1. a dedicated op for requirement-reference linkage, or
2. a distinct payload shape that makes requirement-level linkage explicit

The design rejects silently overloading whole-source linkage in a way that
would make later validation and phase-2 evidence reconciliation ambiguous.

### Materialized State

Requirement metadata should be materialized as a sibling to existing
traceability state rather than scattered ad hoc across issue files.

The implementation may still expose requirement mappings in per-issue state for
fast issue lookups, but the canonical derived view should support whole-repo
coverage and validation for requirement-bearing sources.

## Phase 1 Behavior

Phase 1 makes requirement traceability visible and enforceable at the
planning/provenance layer.

### Source Ingestion

For registered source documents that declare stable requirement IDs, Armature
must:

- parse the IDs deterministically
- materialize requirement references from the cached source content
- detect and report malformed or duplicate IDs within a source
- reconcile requirement references across source re-syncs

Sources without stable requirement IDs remain valid as ordinary source-backed
documents and continue to participate in whole-source traceability only.

### Issue Authoring

For source-backed work that depends on structured requirements, Armature should
support linking issues to specific requirement references during creation or
amendment flows rather than forcing the worker to rely on whole-document links
alone.

`arm render-context` should surface the linked requirement references so workers
see the exact clauses they are implementing, not just the source document title
or source UUID.

### Coverage And Reconciliation

Armature should expose requirement-level coverage for requirement-bearing
sources, including deterministic reporting for states such as:

- requirement reference exists but no issue links to it
- issue links to unknown requirement reference
- issue links to stale requirement reference after source updates
- requirement reference is linked by multiple issues where that is suspicious or
  policy-relevant

The design does not require every requirement to map one-to-one with exactly
one issue. It requires the mappings to be explicit and machine-checkable.

## Validation

`arm validate` should extend the current citation and traceability checks with
requirement-traceability checks for requirement-bearing sources.

Phase-1 validation should cover:

- unknown requirement references
- duplicate or malformed requirement IDs in source material
- orphaned issue mappings
- stale mappings after source re-sync
- requirement coverage summaries for applicable sources

Phase 1 should not fail the repo on the absence of verification evidence. That
would incorrectly collapse the boundary between provenance and execution
verification.

## CLI And UX Direction

The exact command surface can be refined during implementation, but the design
requires these user-facing outcomes:

- users can tell which registered sources expose structured requirement IDs
- users can link issues to requirement references explicitly
- users can inspect requirement coverage deterministically
- workers can see linked requirement references in rendered context
- validation output distinguishes source-level citation coverage from
  requirement-level coverage

The repo should prefer extending current source and traceability workflows over
inventing disconnected commands with overlapping meaning.

## Guardrail Adoption In The Armature Repo

This design is not only about product capability. It also changes how the
`armature` repo itself should operate.

For internal repo work, the intended end state is:

- design and PRD documents that declare stable requirement IDs where
  requirement-level traceability matters
- Armature issues linked to requirement references rather than only whole-source
  citations for that work
- deterministic validation of source-backed requirement coverage in CI
- worker context grounded in exact requirement references

This is the repo-local adoption story for the deterministic guardrails
recommendation around specification traceability.

## Phase 2 Extension Path

Phase 2 is the stronger final position and is intentionally reserved by this
design.

Phase 2 adds machine-readable verification evidence so Armature can reconcile:

- known requirement references
- linked issues
- reported test or verification artifacts

That future system should support the full deterministic question:

- does every requirement reference have corresponding implementation work and at
  least one recognized verification artifact?

But phase 2 must build on the same requirement-reference model introduced in
phase 1. It should not invent a second identifier scheme or a second mapping
layer.

## Risks And Tradeoffs

- Requirement ID extraction can create false confidence if teams treat presence
  of a link as proof of semantic adequacy. This is why phase 1 stops at
  provenance and reserves evidence reconciliation for phase 2.
- Overloading `source-link` too aggressively would reduce short-term CLI churn
  but make long-term validation semantics weaker.
- Making requirement IDs mandatory for every source would overfit the system to
  structured specs and penalize useful but informal sources.
- Requirement coverage can surface ambiguous many-to-many mappings that need
  explicit policy later. The design should report them before it tries to ban
  them.

## Implementation Notes

Implementation work derived from this design should:

- preserve compatibility with existing source-link and accept-citation flows
- add requirement traceability as an extension for structured sources
- add the new terminology to `CONTEXT.md`
- keep phase-2 verification evidence explicitly out of the first slice
- verify the new behavior through deterministic repo-local validation paths

## Open Questions Reserved For Implementation

- the exact serialized op shape for requirement-reference linkage
- the exact materialized state file names and projection layout
- the exact syntax Armature will use to recognize requirement IDs in source
  documents
- whether some requirement-coverage states should be warnings first and become
  errors only after ratcheted adoption

These are implementation questions inside the approved design boundary, not
scope holes in the design itself.
