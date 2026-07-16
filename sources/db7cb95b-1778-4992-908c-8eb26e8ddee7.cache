# Codebase Architecture Improvements

## Status

Proposed for Armature task tracking.

## Source

This document captures the architecture review run on 2026-06-06. The review focused on modular architecture and deepening opportunities for continued health and maintainability.

## Goal

Improve Armature's modular architecture by turning shallow modules into deeper modules with smaller interfaces, stronger locality, and tests that cross the same interface callers use.

## Architectural Vocabulary

Use the repo architecture-review vocabulary when implementing this work:

- A module is anything with an interface and an implementation.
- An interface is every fact a caller must know to use the module correctly.
- A seam is where an interface lives.
- An adapter is a concrete thing satisfying an interface at a seam.
- A deep module hides substantial implementation behind a small interface.
- Locality means change, bugs, and verification concentrate in one place.
- Leverage means callers get more behaviour per unit of interface.

## Epic: Improve Codebase Architecture

Create an epic that collects the codebase architecture improvements below. Each proposed change should be represented as a story so the work can be planned, claimed, reviewed, and completed independently.

## Story 1: Deepen the context_files lifecycle

`context_files` is an explicit curation field, separate from `scope` and `context`. The accepted ADR defines the intended lifecycle, but the implementation currently advertises the field without carrying it through the append-only op interface.

### Current friction

- `materialize.Issue` has `ContextFiles`.
- `validate` reads `ContextFiles` for W5.
- `ops.Payload` cannot carry context files.
- `arm create` has no repeated `--context-file` flag.
- `arm amend` has no replacement path and no `--clear-context-files` path.
- `decompose.PlanIssue` has no `context_files` field.
- `applyCreate` and `applyAmend` do not materialize the field.
- Existing tests cover synthetic validation state, not the real create/amend/decompose to ops to materialization to validation interface.

### Desired shape

Create one lifecycle module that carries context files through command flags, plan parsing, op payloads, materialization, validation, and rendered working memory. The interface should make validation truthful: operators must have a direct way to satisfy W5 without weakening scope.

## Story 2: Move op-log authentication into materialization input

The architecture document says filename-worker-ID validation happens during materialization and invalid ops are excluded with warnings. The current materialization interface accepts pre-read `[]ops.Op`, so it has no filename context.

### Current friction

- `ops.ReadLogValidated` exists below the real interface.
- Common command readers use plain `ops.ReadLog`.
- The TUI has a separate reader path.
- Materialization cannot enforce the filename-worker-ID invariant because callers stripped file identity before passing ops.
- Tests cover the lower-level reader but not command, TUI, or materialization flows that should uphold the invariant.

### Desired shape

Create a deeper validated op-stream module. Its interface should return validated ops, offsets, and invalid-op warnings while preserving filename context. Materialization callers should cross that interface instead of remembering validation themselves.

## Story 3: Make DAG readiness and validation share a real seam

The architecture names one DAG, but the implementation has several graph views. `internal/dag` is tested in isolation while validation, readiness, claiming, and context rendering reimplement traversal against materialized state or index data.

### Current friction

- `internal/dag.DAG` owns graph checks that production validation duplicates.
- `validate` performs its own cycle, hierarchy, and dependency traversal over `materialize.State`.
- `ready` computes gates, sorting, ancestry depth, and descendants from mixed `Index` and full issue maps.
- `render-context` walks parents and blockers itself.
- Tests hand-build each view rather than verifying one materialized graph interface.

### Desired shape

Create a graph projection from materialization that exposes the DAG invariants consumers need: cycles, hierarchy, blockers, descendants, ancestry, readiness gates, and claim/context traversal helpers. Consumers should use the projection rather than each owning partial graph logic.

## Story 4: Deepen the harness-hook command into a hook runner module

The managed-execution removal left harness launch/supervision outside Armature, but hook-native policy enforcement remains important. `cmd/armature/harness_hook.go` now owns too much implementation behind the Cobra interface.

### Current friction

- The command body handles stdin decoding, environment preflight, ops reading, materialization, task policy resolution, evaluator wiring, platform adapter selection, output, and exit mapping.
- Tests must cross the Cobra command seam to verify hook policy behaviour.
- Platform adapter selection is duplicated between command-local logic and internal hook launcher logic.

### Desired shape

Move hook execution semantics into an internal hook runner module. Cobra should pass input and render output. The runner should own state loading, policy evaluation, adapter selection, structured result, and exit-code mapping while preserving hook-native enforcement rather than reintroducing managed execution.

## Validation Expectations

Each story should be implemented with tests that cross the intended module interface. Final closeout should include:

- `go test ./...`
- `go run ./cmd/armature validate --ci`
- `go run ./cmd/armature doctor`
- targeted tests for the changed module interface

