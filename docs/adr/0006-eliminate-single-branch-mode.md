# ADR: Eliminate Single-Branch Mode

## Status

Accepted

## Principles touched

I1, I3

## Context

Armature originally supported two operating modes: single-branch mode, where `.armature/` coordination data lived on the same branch as code, and dual-branch mode, where coordination data lived on a dedicated `_armature` ops branch accessed through a separate ops worktree. `Mode` was a config field (`internal/config/config.go`), defaulting to `"single-branch"`, with both modes threaded through config probing, materialization, sync, hooks, the TUI, and most of `docs/`.

In practice, single-branch mode sees no real use: coordination history and code history compete for the same branch, which is the exact problem the ops-branch/ops-worktree split was introduced to solve. Carrying both modes doubles the paths that config probing, doctor checks, sync, and the harness hook have to handle, for a mode nobody exercises.

There is no known deployment of this code with live `.armature/` state in single-branch layout, so there is no real migration burden to weigh against the simplification.

## Decision

Single-branch mode is removed entirely. Dual-branch mode is not renamed or reframed as an option — it is simply how Armature works: coordination data always lives on the `_armature` ops branch (literal name, unchanged), accessed through the ops worktree. The `Mode` config field, `single-branch`/`dual-branch` branching throughout config, materialize, sync, hooks, the TUI, and docs are deleted rather than deprecated.

`CONTEXT.md`'s `Single-Branch Mode` and `Dual-Branch Mode` glossary entries are removed; there is no longer a mode distinction to name.

As a safety net for any pre-existing clone that predates this change, `arm bootstrap`/`arm doctor` gets a forced migration path that detects single-branch layout and moves coordination data onto the `_armature` ops branch, rather than erroring out unmigrated.

## Consequences

Every reference to single-branch mode (and, since it's no longer a meaningful contrast, "dual-branch mode" as a named concept) is deleted from code, skills, and docs — this is a broad, multi-file change, not a flag flip. Any code path that branched on `Mode` collapses to the single remaining behavior. Config schemas, bootstrap, and doctor need a migration/compatibility check for stray old-layout clones, but no supported feature is preserved behind a flag.
