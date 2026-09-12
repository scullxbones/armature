---
date: 2026-09-12
agent: bfc7fab2-5292-4d3d-9773-9f49b203f5c6~w-AOC-S4-T2
area: workflow
task: AOC-S4-T2
tags: [transition, idempotency, worker-thrash, dogfood]
---

# Harness loop re-reports identical empty-outcome done

## User Goal

Mark an issue `done` once, with a stable empty outcome, and leave the ops
log with that single transition.

## Observed

`ORCH-RUNTIME-V1-T3` emitted six byte-identical empty-outcome `done`
transitions inside 15 minutes. Same issue, same `to`, same empty outcome,
six appended ops.

The AOC design measurement (recorded in ADR 0018) counted 14 true
duplicates across 1,367 transition ops. Replay of 9,172 ops in
`.armature/ops/` found 43 same-status transitions: 29 amendments (same
status, richer payload) and 14 true duplicates (byte-identical payload).
True duplicates are 0.153% of the full log.

The contrast case is `LNGHZN-S9-T2`: it reached `done` seven times, each
time with a corrected, richer outcome. Those writes are amendments, not
thrash. Status-only idempotency would have frozen that issue at the first
report and dropped the later audit correction.

The defect is the harness loop re-reporting `done` after the first
success. The ops log did what I2 requires: it recorded every write. It
does not need a rewrite, a counter on an existing line, or a squash of
history.

## Impact

Once AOC-S4-T1 / ADR 0018 keys `arm transition` idempotency on payload,
those 14 true duplicates become no-ops: exit 0, append nothing, say so
explicitly. The log stays clean. The harness loop does not.

A suppressed duplicate leaves no durable trace in ops (Q9b / ADR 0018).
A retry counter on the existing op would both rewrite history (I2) and
race under I3, because two workers cannot safely update one log line.
The only place the thrash stays visible as a tracked concern is this
dogfood finding.

Without it, a later reader of a quiet ops log would conclude the loop
was fixed. Payload-keyed no-op is the ops-side mitigation. It is not a
harness fix.

## Evidence

- Writer identity from `arm worker-init --check`: `bfc7fab2-5292-4d3d-9773-9f49b203f5c6`, slot `w-AOC-S4-T2`.
- `ORCH-RUNTIME-V1-T3`: six byte-identical empty-outcome `done`
  transitions in 15 minutes (AOC-S4-T1 notes; ADR 0018 Context).
- Corpus: 14 true duplicates / 1,367 transition ops (AOC-S4-T2
  definition of done; same measurement as ADR 0018).
- Full-log split: 9,172 ops replayed; 43 same-status transitions; 29
  amendments; 14 true duplicates; 0.153% of the log.
- Contrast: `LNGHZN-S9-T2` seven `done`s with a richer outcome each time
  (amendments that must remain appendable).
- ADR: `docs/adr/0018-payload-keyed-op-idempotency.md`.
- Contract item 9: `docs/design/agent-output-contract.md` (idempotency
  keyed on payload; worker thrash is a harness defect).

## Suggested Follow-Up

Keep this finding open after payload-keyed no-op lands. Do not treat a
quiet ops log as proof the loop is gone.

Fix the harness so a successful `arm transition --to done` with an
identical empty outcome is not re-issued. The CLI no-op at exit 0 is
already the right answer for a retry; the loop should not retry.

Do not add a duplicate counter to ops. That would rewrite history and
race under I3.
- Project convention: `docs/agents/dogfood-findings.md`.
