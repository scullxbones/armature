# Graph Facts Refactor Follow-Up

## Status

Proposed for a new Armature story.

## Source

This write-up captures follow-up work identified on 2026-06-08 while red-teaming a newer DAG refactor plan against the current Armature codebase, the completed `ARCHIMP-S3` lane, and the repo's existing architecture documents.

## Goal

Create a new story that deepens `internal/dag` into the canonical owner of pure graph facts derived from materialized task state, while fixing the immediate documentation mismatches that would otherwise become more misleading after the refactor.

## Background

`ARCHIMP-S3` is already completed work and should be treated as prior art, not as an open lane to extend informally. A newer refactor plan proposed moving more DAG behavior into `internal/dag`, but the review surfaced a sharper boundary:

- `internal/dag` should own pure graph knowledge only.
- `internal/dag` should not own queue DTOs, claim-TTL policy, or human-facing validation wording.
- The repo currently has an inconsistent type lattice around `feature`, and centralizing hierarchy checks will force that inconsistency into the open.

This follow-up story exists to make the seam explicit and truthful without reopening broad architectural or documentation work.

## Current Friction

- `internal/dag` is still shallow compared to the graph knowledge duplicated in `validate`, `ready`, and some CLI consumers.
- The newer refactor plan overreached by proposing `ReadyQueue` outputs and validation strings from `internal/dag`, which would blur the pure-graph boundary.
- `feature` is already a real node type in the CLI and planner-facing surfaces, but hierarchy documentation and validation descriptions still reflect the older lattice.
- Some consumer behavior still depends on local traversal helpers rather than a shared graph-facts seam.

## Story Scope

This story should do the following:

1. Define the canonical `internal/dag` boundary as **pure graph facts from materialized state**.
2. Refactor consumers to use those facts where doing so removes duplicated traversal or hierarchy knowledge.
3. Make `internal/dag` the canonical owner of parent/child type legality, including the current `feature` node type.
4. Repair the immediately identified documentation mismatches required to keep the repo truthful after the code change.

## Explicit Boundary

`internal/dag` should own facts such as:

- ancestry
- descendants
- parent-chain traversal facts
- graph depth
- cycle detection
- unresolved graph links
- hierarchy legality facts such as `IsValidParentChild(parentType, childType)`

`internal/dag` should **not** own:

- `ready.ReadyEntry` or any queue-specific DTO
- claim staleness / TTL policy
- assignment-aware queue ordering policy
- `arm validate` diagnostic code ownership or user-facing wording
- generic dry-run plan validation over non-materialized nodes

## Immediate Documentation Repair Included In This Story

This story should repair only the documentation that becomes immediately untruthful because of the code work. That includes the places where the repo describes hierarchy legality without acknowledging `feature` as a node type.

This story should **not** attempt a repo-wide DAG terminology cleanup. That broader audit belongs to a separate future documentation deep-dive story.

## Non-Goals

- Reopening or rewriting `ARCHIMP-S3`
- Broad documentation audit across all DAG-related docs and skills
- Planner/runtime unification for dry-run or non-materialized graph projections
- Harness-hook work
- Queue-policy redesign

## Success Criteria

- `internal/dag` exposes graph facts rather than queue DTOs or validation strings.
- Consumer modules stop duplicating the targeted traversal and hierarchy knowledge.
- `feature` hierarchy legality has one canonical implementation seam.
- The immediately affected documentation no longer contradicts the implemented type lattice.
- Verification for the story includes:
  - targeted tests for the changed graph-facts seam
  - `go test ./...`
  - `go run ./cmd/armature validate --ci`
  - `go run ./cmd/armature doctor`

## Handoff Notes

The implementation session for this story should begin by re-checking the live consumers of graph facts in:

- `internal/validate`
- `internal/ready`
- `internal/context`
- `cmd/armature/create.go`
- `cmd/armature/reparent.go`
- any remaining CLI filtering paths that still own descendant traversal logic
