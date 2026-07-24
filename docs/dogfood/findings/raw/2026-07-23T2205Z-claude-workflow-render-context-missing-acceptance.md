---
date: 2026-07-23
agent: claude
area: workflow
task: LNGHZN-S3 coordination
tags: [render-context, acceptance-criteria, worker-dispatch, review]
---

# `render-context --format agent` omitted the issue's acceptance array, causing two remediation rounds

## User Goal

Dispatching a haiku worker for LNGHZN-S3-T1 per the armature-coordinator skill,
using `arm render-context LNGHZN-S3-T1 --format agent` as "the worker's complete
task spec" (per the skill's Dispatch Protocol step 2: "pass it verbatim").

## Observed

`arm render-context LNGHZN-S3-T1 --format agent` returned only a `core_spec`
layer containing the DoD paragraph — no `acceptance` layer or field appeared
anywhere in the JSON. The task's issue record, however, did have a literal
`Acceptance` array with six exact required test function names (verified later
via `arm show LNGHZN-S3-T1`, which prints an `Acceptance:` line the
render-context output never surfaced). Because the coordinator briefed the
worker using only the render-context output (as instructed), the worker never
saw the required test names and wrote differently-named tests covering similar
but not identical ground. This was only caught two review rounds later, when
the armature-reviewer subagent read the acceptance array directly from the
review bundle (which does include it) and flagged the name mismatch as a RED
finding.

## Impact

Two full remediation rounds: round 1 (a real DoD violation — I/O helpers in the
wrong file) was compounded by round 2 solely being about the missing test names
this finding describes, requiring another full review cycle. That second
review cycle also happened to be the one that led to discovering a genuinely
critical path-resolution bug — so the friction here indirectly had a silver
lining, but the process cost (three total review passes on one task) traces
back to this gap.

## Evidence

- `arm render-context LNGHZN-S3-T1 --format agent` output: single `core_spec`
  layer, no acceptance content anywhere in the JSON.
- `arm show LNGHZN-S3-T1` (run later, independently): `Acceptance:
  ["TestHookEmitsRateLimitedHeartbeat_REQ_LNGHZN_S3_T1 passes", ...]` (six
  literal test names).
- armature-reviewer's second-pass assessment: "acceptance[1]... not_satisfied...
  Specified test name not authored" across 4 of the 6 criteria.

## Suggested Follow-Up

- Either fix `render-context --format agent` to include the acceptance array in
  its `core_spec` (or a dedicated layer), or update the armature-coordinator
  skill's Dispatch Protocol to explicitly instruct coordinators to cross-check
  `arm show <issue>` for acceptance criteria before briefing a worker, since
  "pass render-context verbatim" is not sufficient on its own when
  render-context doesn't carry the full contract.
