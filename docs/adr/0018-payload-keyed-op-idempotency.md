# ADR 0018: Payload-Keyed Op Idempotency

## Status

Accepted

## Principles touched

I2, I3, I4

## Context

Replaying 9,172 ops in this repository's `.armature/ops/` found 43 same-status
transitions. They split into 29 amendments (same status, richer payload) and
14 true duplicates (byte-identical payload). `LNGHZN-S9-T2` reached `done`
seven times, each time with a corrected outcome. `ORCH-RUNTIME-V1-T3` fired
six empty-outcome `done`s in 15 minutes. True duplicates are 0.153% of the
log.

Keying idempotency on status alone would have frozen `LNGHZN-S9-T2` at its
first report and dropped every later audit correction. Treating every
same-status write as history would keep recording the `ORCH-RUNTIME-V1-T3`
thrash. Agents (I4) retry; the CLI has to tell them which of those two
things happened, at exit 0, without making a retry look like a failure.

Constitution I2 says history is never rewritten. Declining to append an op
that cannot change materialized state is not a rewrite: nothing that was
recorded is changed, and an op that would have been a no-op on replay was
never history. A counter on an existing op would both rewrite that line and
race under I3 (two workers cannot safely update one log). Suppressed
duplicates therefore leave no durable trace; worker thrash belongs in the
dogfood corpus (AOC-S4-T2), not in the ops log.

Optional token counts (`input_tokens` / `output_tokens`, TOPTIER-S11-T1)
live on the same transition payload. They join payload equality only when
they were already recorded. Legacy ops omit them; `omitempty` keeps absent
and zero the same bytes, so equality does not invent a new rule that would
treat every legacy retry as an amendment.

## Decision

Idempotency for `arm transition` is keyed on the transition payload, not on
status:

1. If the issue is already at `--to` and the proposed payload is
   byte-identical to the currently recorded transition payload, the command
   is a no-op: exit 0, append nothing, say so explicitly. This is not an
   error.
2. If the issue is already at `--to` and the payload differs (outcome,
   branch, PR, tokens, delivery-gate override, and so on), append a new
   transition op as an amendment at exit 0.
3. Recorded payload is the last transition op whose `to` still matches the
   issue's status. When there is no such op, the payload is synthesized from
   materialized status, outcome, branch, and PR, without inventing token
   counts.

This is the same decision recorded as item 9 in
[`docs/design/agent-output-contract.md`](../design/agent-output-contract.md).
This ADR is the citable record; it does not restate the rest of the Agent
Output Contract (ADR 0017).

## Consequences

Workers can retry `arm transition` without polluting the log or losing a
later, richer outcome. Readers of the log can still see every amendment.
I2 is intact: no-op means "do not start a history line," not "edit one."
I3 is intact: no worker rewrites another worker's file. Follow-up: AOC-S4-T2
records the harness thrash pattern in dogfood so the 14 suppressed
duplicates remain visible as a tracked concern.
