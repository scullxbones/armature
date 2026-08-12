# Theme: render-context Omits Acceptance Criteria and Context Layers

## Summary

`arm render-context --format agent` is documented as the complete worker dispatch spec, but its `core_spec` layer only includes the Definition of Done — not the named acceptance-criteria tests (`TestX_REQ_<TASK-ID>`) that `arm review prepare` bundles enforce, and it can silently drop entire context layers (`context_files`, `blocker_outcomes`, `sibling_outcomes`) for a single task mid-story with no other tasks affected. Workers implement to a spec that looks complete and get rated red or yellow for requirements they never saw.

## Evidence

- [render-context omits acceptance criteria that review bundles enforce](../../raw/2026-07-03T1810Z-claude-workflow-render-context-omits-acceptance-criteria.md) — Workers dispatched with full `render-context --format agent` output never saw contracted test names like `TestBindingResolutionChain_REQ_HOOKBIND_T2` that the review bundle checks against, causing red/yellow reviews.
- [`arm show` exposes acceptance criteria render-context omits](../../raw/2026-07-03T1950Z-claude-workflow-arm-show-workaround-for-acceptance-criteria.md) — After three consecutive red/yellow reviews, the coordinator found `arm show <TASK>` prints an `Acceptance:` field with the exact test names — the workaround for a gap in the command the skill mandates as the source of truth.
- [Workers rated red for tests named in acceptance criteria they never saw](../../raw/2026-07-05T2227Z-claude-workflow-render-context-omits-named-acceptance-tests.md) — Same gap recurring on ARCHIMP-S18: `render-context --format agent` emitted only core_spec; the reviewer (who sees acceptance criteria via `review prepare`) rated deliveries red for missing required test functions.
- [render-context returned only the core_spec layer for one task mid-story](../../raw/2026-07-06T1258Z-claude-tooling-render-context-single-layer-for-t2.md) — For EXECEV-T2 specifically, `context_files`, `blocker_outcomes`, and `sibling_outcomes` were all missing even though T1 was already merged with a recorded outcome and the same source was linked story-wide; every sibling task (T1, T3, T4, T5) rendered the full multi-layer context correctly.

- [`render-context --format agent` omitted the issue's acceptance array, causing two remediation rounds](../../raw/2026-07-23T2205Z-claude-workflow-render-context-missing-acceptance.md) — LNGHZN-S3-T1. The task carried six exact required test names in its `Acceptance` array; `render-context` returned only a `core_spec` layer with the DoD. Because the coordinator skill says to pass that output verbatim as "the worker's complete task spec," the worker never saw the names and wrote differently-named tests. Caught two review rounds later by the reviewer reading the acceptance array from the review bundle, which *does* include it.
- [`render-context --format agent` has no `branch` field](../../raw/2026-07-24T1055Z-claude-tooling-worktree-sandbox-friction.md) (item 3) — The coordinator skill's Dispatch Protocol step 5 instructs telling the worker its "task-specific branch (from render-context)", but no such field exists; the coordinator must derive it via `git -C <worktree> rev-parse --abbrev-ref HEAD`.

## Pattern

`render-context --format agent` is asserted by the coordinator skill to be "the worker's complete context," but two independent gaps break that assertion:

1. **Acceptance criteria never included in any layer**: named required test functions live only in the review-side artifact (`arm review prepare` bundle / `arm show`), never in the worker-side artifact. Workers and reviewers are working from two different specs of the same task, and the worker's copy is silently incomplete.
2. **Layer selection is unreliable for at least one task per story**: a task with an otherwise-identical dependency shape to its siblings can render with only the core_spec layer, dropping context that's necessary to reproduce sibling behavior or reference cited sources.

## Impact

- Workers implement fully correct code against the Definition of Done and are still rated red/yellow, because "correct" is defined partly by acceptance-criteria test names they never received.
- Coordinators have discovered and now rely on an undocumented workaround (`arm show`) to patch dispatch prompts by hand — a recurring manual step that should be automatic.
- The single-layer failure for EXECEV-T2 is not (yet) understood to be caused by anything task-specific, making it hard to predict which task in a story will be affected next.

## Candidate Follow-Ups

- Include acceptance-criteria test names (the same set `arm review prepare` bundles) in `render-context --format agent`'s core_spec or a dedicated layer — this is the highest-value fix since it's now been hit on three separate stories.
- Update the coordinator skill's dispatch step to fold in `arm show <TASK>` output (or wait for the fix above) rather than treating `render-context` alone as complete.
- Investigate why EXECEV-T2 specifically dropped `context_files`/`blocker_outcomes`/`sibling_outcomes` while its siblings didn't — this looks like a real bug in layer selection, not a documentation gap.
