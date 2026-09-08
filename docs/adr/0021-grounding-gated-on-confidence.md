# ADR 0021: Grounding is gated on Confidence, not status

An ungrounded Draft is legal. An ungrounded Verified is a Graph Finding.
The chokepoint is Plan Release. Coverage reports Draft and Verified
separately so the percentage is not a mix that lies.

## Status

Accepted

## Principles touched

I5, I7

## Context

`Compute` treated every issue with `SourceLinkCount == 0` as Uncited,
Draft and Verified alike. Mixing those bands made coverage % lie: a
legal ungrounded Draft dragged down the ready-flow figure, and
`arm validate` would treat in-flight drafts as Graph Findings.

CONTEXT.md already separates **Confidence** (Draft / Verified) from
status and readiness. ADR 0016 already made Plan Release the
whole-graph validate door, with **Release Override** as the recorded
human break-glass. This ADR records the gating *axis* for grounding:
Confidence, not status. No new machinery.

## Decision

1. **Axis.** Grounding is gated on Confidence. Status (`claimed` /
   `in-progress` / `done`) is not a grounding gate.
2. **Draft.** An ungrounded Draft is legal. It is not a Graph Finding
   and must not enter Verified coverage totals.
3. **Verified.** An ungrounded Verified is a Graph Finding. Empty
   Confidence is Verified (legacy default).
4. **Chokepoint.** Plan Release (promotion of a draft subtree to
   verified) is already the whole-graph validate gate. That is where
   grounding becomes required. Introduction does not demand a source
   at `arm create`.
5. **Coverage.** Draft and Verified are reported separately. Headline
   `CoveragePct` is the Verified band so the percentage stays
   meaningful.
6. **Override.** Release Override remains the human break-glass (I7).
   It is never a green release.

Rejected alternatives (do not re-litigate):

- `--source` required at `arm create` — Introduction is not the
  grounding door; birth is Draft.
- Gate on status (`claimed` / `in-progress` / `done`) — status is
  workflow position, not provenance.
- Introduction-scoped CI as a replacement for whole-graph validate —
  already rejected (ADR 0016 / D7): dirt in nobody's subtree stays
  dirty.
- Time-based grace for ungrounded Verified — dies on I5; LLM or clock
  judgment must not be an automated merge decision.

## Consequences

- Traceability `IssueRef` carries Confidence. `Compute` emits Graph
  Findings only for ungrounded Verified nodes.
- `validate` E7 still lives in `internal/validate` until a later task
  consumes these semantics; this ADR is the citable rule those callers
  must follow.
- Coverage JSON grows Draft/Verified bands and a `findings` list.
  Consumers that treated mixed `coverage_pct` as whole-graph health
  should read the Verified band.
