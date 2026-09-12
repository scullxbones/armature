# ADR 0019: Park TOON as an Agent Output Encoding

## Status

Accepted

## Principles touched

I4, I5

## Context

AXI's first principle prefers TOON (Token-Oriented Object Notation) over
JSON for agent-facing structured output. ADR 0017 took AXI as prior art,
not as a standard, and left alternate encodings out of that decision so a
park would not hide in a consequences note. This ADR is that later
decision. A park buried in another file quietly becomes a purge, which is
what [ADR 0010](0010-park-not-purge-subtractive-release.md) exists to
prevent.

The measured basis is the live inventory at `v0.0.2-237-g80dee97c` (738
issues), using the bytes/4 token convention that `NXTTN-S3-T1`
standardises and that `make context-report` still emits:

| payload | bytes | estimated_tokens (bytes/4) |
| --- | ---: | ---: |
| `arm list` (non-TTY default) | 342,560 | 85,640 |
| same, minimal 4-field schema, pretty JSON | 111,223 | 27,805 |
| same, compact JSON | 89,082 | 22,270 |
| same, TOON | 61,880 | 15,470 |

Dropping `outcome` from list rows is 68 percent of the available saving.
TOON is the last 30 percent of what remains after that schema cut
(61,880 vs 89,082 compact JSON bytes). That remainder is real. It is not
large enough, on its own, to pay the costs of a second wire format:

1. **No Go encoder.** Armature is a Go CLI with no runtime JS. AXI's
   encoder is JavaScript. Parking TOON avoids putting a third-party
   encoder, or a first-party TOON writer we do not yet have, on the
   paved road.
2. **Quoting is load-bearing.** 98 of 738 titles contain commas. A
   tabular encoding that treats comma as a delimiter has to quote those
   titles correctly on every list, ready, and show payload, or agents
   parse garbage.
3. **A second pin.** `--format json` and `--format agent` already share
   one envelope. Adding TOON would be a new encoding of that envelope,
   pinned by `embed-examples` and every skill example, paid on every
   later envelope change.

`make context-report` is the standing meter for the compact JSON that
agents actually see. On `feat/AOC-S3-T2` (`a0711444`) it reports fixture
json/agent stdout as:

| PATH | CLASS | BYTES | EST_TOKENS |
| --- | --- | ---: | ---: |
| list | invocation | 377 | 94 |
| ready | invocation | 592 | 148 |
| show | invocation | 415 | 103 |
| render-context | invocation | 1164 | 291 |

Those rows sit under the named `target_bytes` promises in
`internal/contextreport/budgets.json` (list 2048, ready 1024, show 2048,
render-context 16000). Compact JSON already holds the customer promise
on the fixture. TOON would shrink those rows further. It would not be
the cut that makes the promise true.

This is a park, not a rejection and not a purge. TOON never shipped, so
there is no runtime code to delete. ADR 0010 still applies: no redirect
shim, no `--format=toon` error, no feature-flagged dormancy. The record
is this ADR plus a re-entry criterion. Resuscitation, if the criterion
is met, is a fresh implementation task, not a revert.

## Decision

TOON is **parked** as an agent output encoding.

1. The Agent Output Contract stays JSON. `--format json` and
   `--format agent` keep emitting the same envelope.
2. The CLI MUST NOT grow a TOON writer, a `--format=toon` value, a
   dormant encoder, or a redirect that names this park.
3. Re-open the encoding only when the **re-entry criterion** below
   holds. Taste ("TOON looks better") is not a criterion.

**Re-entry criterion.** A standing test, not a promise of future work.
Evaluate it in the units `make context-report` emits: UTF-8 `bytes` and
`estimated_tokens`, where `estimated_tokens = bytes/4` (integer
division). Read those from the human table columns `BYTES` and
`EST_TOKENS`, or from the JSON fields `bytes` and `estimated_tokens`.
Do not convert through another tokenizer.

All three conditions MUST hold for the same priced invocation path
(`list`, `ready`, `show`, or `render-context`):

1. **JSON is over the named promise.** `make context-report` reports
   that path's `bytes` greater than that path's `target_bytes` in
   `internal/contextreport/budgets.json`.
2. **TOON is the remaining cut that meets the promise.** Encode that
   same payload as TOON. Count UTF-8 bytes the same way context-report
   does (`len(payload)`). TOON `bytes` MUST be less than or equal to
   `target_bytes`. Compact JSON `bytes` MUST remain greater than
   `target_bytes`.
3. **The measured remainder still shows up.** TOON `bytes` MUST be at
   most 70 percent of compact JSON `bytes` for that payload. That is
   the share measured at park time (TOON 61,880 of compact JSON 89,082
   on `arm list`).

If (1) fails, JSON still holds the named promise and TOON stays parked.
If (1) holds but (2) or (3) fails, trim the JSON payload. Do not add a
second encoding to chase a smaller number that does not close the
budget. Meeting the criterion authorises a resuscitation task. It does
not itself merge an encoder.

## Consequences

- ADR 0017's deferred encoding question has a citable answer. The
  normative spec in
  [`docs/design/agent-output-contract.md`](../design/agent-output-contract.md)
  stays JSON; this file holds the park and the test that would unpark it.
- Skills, fixtures, and `embed-examples` pin one encoding. Envelope
  changes do not have to be proven twice.
- Anyone proposing TOON later compares `make context-report` output to
  `target_bytes` and to a TOON `bytes` count of the same payload. They
  do not re-argue AXI §1.
- Resuscitation pays the costs recorded here (a Go encoder we own,
  quoting rules for comma-bearing titles, a second pin in
  embed-examples and skills) as part of that fresh task. Those costs
  are why this is a park. They are not a substitute for the byte test.
