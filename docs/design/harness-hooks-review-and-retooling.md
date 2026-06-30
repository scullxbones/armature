# Harness Hooks Review And Retooling

## Status

Proposed for a future Armature story and a dedicated future session.

## Source

This write-up captures concerns raised on 2026-06-08 after reviewing the current harness-hook integration documents and comparing them against the likely reality of current harness invocation paths.

## Goal

Perform a major review of the harness-hook integration surface, validate how supported harnesses actually invoke hooks today, and retool the implementation and docs so Armature's hook-native enforcement path is both truthful and dogfoodable.

## Problem Statement

The current harness-hook documentation should not be assumed correct merely because it exists.

The repo already contains:

- a public integration guide in `docs/harness-hook.md`
- a smoke runbook in `docs/provider-smoke-tests.md`
- platform adapters under `internal/harnesshook`
- a hidden `arm harness-hook` command

But the current review raised a stronger concern: the documented invocation and configuration model may not match how the harnesses are actually invoked now.

If that is true, the current system has two problems at once:

1. the implementation may be making incorrect assumptions about real hook events
2. the docs may be teaching an invalid setup path

## Current Friction

- The main integration guide still frames setup as a manual configuration path.
- The docs and smoke runbook are only as good as their assumptions about live harness behavior.
- The repo has adapter-level config writers, but no trusted evidence here that the documented path is being exercised in real dogfood runs.
- Devin remains deferred in smoke coverage, and Claude/Codex truth still needs to be revalidated against current harness behavior.
- The hook surface is important enough that stale docs are actively dangerous: they can make the system appear integrated when it is not.

## Scope

This future story should:

1. Revalidate real hook event payloads and invocation semantics for supported harnesses.
2. Compare the live harness behavior against:
   - `docs/harness-hook.md`
   - `docs/provider-smoke-tests.md`
   - `internal/harnesshook/*`
   - `cmd/armature/harness_hook.go`
3. Identify which assumptions are wrong, stale, incomplete, or no longer supported.
4. Retool the hook integration surface accordingly.
5. Dogfood the retained hook model with real provider runs and capture evidence.

## Recommended First Cut

The first implementation story here should focus on **live dogfooding and truth recovery**, not installers.

That means:

- validate real Claude and Codex behavior first
- capture the actual event shapes and launch posture
- make the implementation and docs truthful
- defer installation automation until the live path is known-good

## Non-Goals

- Reintroducing managed execution
- Generic harness abstraction beyond the supported provider set
- Broad docs cleanup unrelated to hook truth
- Installer automation before the live invocation model is revalidated

## Success Criteria

- The supported harnesses' actual hook invocation paths are known and documented.
- The implementation assumptions in `internal/harnesshook` match those real paths.
- The integration guide no longer teaches a stale or invalid setup model.
- Real dogfood evidence exists for the retained hook model, at minimum for the first supported providers selected for live validation.
- Any deferred providers or unsupported behaviors are called out explicitly rather than implied.

## Handoff Notes

This story should begin from a skeptical posture:

- treat current docs as hypotheses, not truth
- verify actual harness behavior before editing architecture claims
- keep the story focused on truth recovery and dogfooding before automation
