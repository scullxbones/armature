# Common Failure Modes

| Failure | Cause | Fix |
|---|---|---|
| Parallel agents share one log | `ARM_LOG_SLOT` not in each agent prompt | First worker instruction after skill load: `export ARM_LOG_SLOT=<slot>` |
| Remediator heartbeats ignored | Unslotted remediating `arm claim` | Prefix `ARM_LOG_SLOT=$REMEDIATOR_SLOT` on the same `arm claim`; assert `ClaimedBy` |
| Confirmation still yellow/red, wave merged | Steps 5–6 ran once then fell through to a.3 | Repeat 5–6 until green or cycle 3; never enter a.3 on non-green |
| Parallel reviews clobber one assessment file | Path unique only by issue + bundle | Distinct `<reviewer-token>`; consolidate findings before `arm review record` |
| Build breaks after parallel merges | Skipped wave verification | Run the gate in `references/wave-verification.md` before the next wave |
| Semantic revert on parallel task branches | Same file touched by two tasks | Overlap audit in `references/overlap-audit.md` before `merged` |
| Story `done` fails on uncited nodes | Transitioned before citation coverage | `arm validate`; `arm sources link` or `arm sources accept-citation --ci` |
| Ops dirty on push | Single-branch leftover | After story transition, `git status`; commit `.armature/` only in single-branch mode |
