---
date: 2026-07-06
agent: claude
area: tooling
task: EXECEV coordination
tags: [render-context, context-layers]
---

# render-context returned only the core_spec layer for one task mid-story

## User Goal

Coordinator fetching `arm render-context EXECEV-T2 --format agent` to build the worker's dispatch prompt.

## Observed

For EXECEV-T2 the output contained only the `core_spec` layer — no `context_files` (the ADR-0008 excerpt that every sibling task carried), no `blocker_outcomes`, no `sibling_outcomes` — even though T1 was already merged with a recorded outcome and the same ADR source was linked story-wide. T1, T3, T4, T5 all rendered full multi-layer context. The coordinator had to hand-paste the ADR context and prior-task outcomes into the worker prompt.

## Impact

A worker dispatched with the bare core_spec would have implemented the bundle activity section without the ADR's trust-model constraints in view. Cost was coordinator effort to reconstruct context manually; risk (unnoticed) would have been a materially under-informed worker.

## Evidence

- `arm render-context EXECEV-T2 --format agent` output: layers 2–8 present but `content: ""` except core_spec (captured in session transcript, 2026-07-06).
- Compare `arm render-context EXECEV-T3 --format agent` moments later: full ADR text in `context_files`, blocker + sibling outcomes populated.

## Suggested Follow-Up

Check whether context assembly for T2 raced with T1's merge/outcome recording, or whether source-links on T2 were missing; consider a render-context warning when a linked source or blocker outcome exists but a layer renders empty.
