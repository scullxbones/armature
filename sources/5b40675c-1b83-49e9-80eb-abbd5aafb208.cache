# Next-Work Sequencing

**Date:** 2026-10-24. Gardened after **v0.1.0** leftovers landed; this page tracks the remaining ranked backlog. No named successor release yet.

**Closed history:** Tier S, the Tier A feature spine, delivered pull-ins, the 2026-09-18 recommended sequence, and the F2 grilling decisions are in [`docs/archive/next-work-sequencing-closed-through-2026-09-27.md`](../archive/next-work-sequencing-closed-through-2026-09-27.md). v0.1.0 cut and post-cut leftover deliveries are in [`docs/archive/next-work-sequencing-closed-v010.md`](../archive/next-work-sequencing-closed-v010.md). Ordinals below match that snapshot. Do not renumber them.

**Purpose:** One execution order across the planning rounds (`top-tier-gap-analysis.md`, `long-horizon-proposals.md`, `the-next-ten.html`, `narrow-gaps-addendum.md`) plus later filings that earned a row. Armature's DAG orders work inside a tree. It does not rank independent proposals that share no `blocked_by` edge. This file is that ranking. When an item is decomposed, its DAG is authoritative inside the item. This file only orders between items.

Statuses below audited 2026-10-24 with `arm show`. Re-audit before dispatch.

## What to do next

v0.1.0 is shipped (`TOPTIER-S6` / `TOPTIER-S6-T3` **merged**; tag `v0.1.0`). Post-cut leftovers on the docs/CLAIMORD trains are **merged** on ops and `main` (S7-T2 #324, S8 #316, S15-T2 #378, S16-T1/T2 #326/#379, CLAIMORD W11–W14 + W20 #277–#281). `arm ready` is not a dispatch order. A coordinator that picks the first ready issue will treat held docs and Tier C extensibility as equal to real next work.

1. **`CLAIMORD-W21`** is next on the CLAIMORD chain, but **not ready**: still waits on `bug-1783480206` (behind the S12 / `TOPTIER-S18-T4` doctor cascade). W13, W14, and W20 are **merged**. Do not start W21 under the S12 hold.
2. **Hold item 22, `TOPTIER-S12`.** Leave it unless Brian needs G2. `TOPTIER-S18-T4` and item 24's T4 (doctor **D13**) wait on S12-T2 / related holders. That wait is not a reason to lift the hold. Doctor **D11** stays reserved for S12-T2. **D12** already landed (PR #198).
3. **Do not treat these as the successor queue.** `TOPTIER-S12-T1` (held), `TOPTIER-S14` / `TOPTIER-S14-T1` (Tier C, ready), and `bug-1783480206` (waits on S18-T4).
4. **Pick a backlog theme** from the remaining Tier B/C rows below. Do not invent a next release number until a cut task is filed.

## Still open

### Tier B

| # | Item | Source | Armature story |
|---|---|---|---|
| 22 | Ops-branch backup and disaster recovery | Addendum G2 | `TOPTIER-S12` (**hold**). T1 (docs) is ready. T2 is doctor **D11** and waits on T1. `TOPTIER-S18-T4` was reparented here and waits on T2 |
| 24 | One merged-promotion path | LH D4 | `LNGHZN-S11` (ADR 0022). T1–T3 **delivered** (PR #307). **T4** (doctor **D13**) remains; waits on T2 plus the open `doctor.go` holders (`TOPTIER-S12-T2`, `TOPTIER-S18-T4`, `bug-1783480206`) |
| 25 | Redesign transition hooks | LH D2 | not yet decomposed |
| 26 | Event stream (`arm events --follow`) | LH F3 | not yet decomposed |
| 28 | The Harness Compatibility Contract | Next-Ten №08 | not yet decomposed |
| 29 | Model-tier dispatch policy | LH C8 | not yet decomposed |
| 32 | The Second Substrate (foreign-repo dogfood) | Next-Ten №06 | not yet decomposed (dogfood themes from the v0.1.0 cut are optional backlog) |
| 33 | Session handoff bundle | LH C10 | not yet decomposed |

G2 sits early because the addendum called it cheap. Items 23 (authorship / `TOPTIER-S16`), 30 (README / `TOPTIER-S7`), 31 (shims), 34 (`TOPTIER-S6` **merged**), and 35 (`TOPTIER-S8` **merged**) live in the v0.1.0 archive. The old late-tier G1 row (item 36, `TOPTIER-S11`) and item 27 (`LNGHZN-S10`) remain in the 2026-09-27 archive.

### Tier C

| # | Item | Source | Armature story |
|---|---|---|---|
| 37 | Findings as a product loop (`arm finding`) | LH C9 | not yet decomposed |
| 38 | Scope/context suggestion from co-change mining | LH C7 | not yet decomposed |
| 41 | Living Diagrams | Next-Ten №10 | not yet decomposed |
| 42 | Time-travel state (`--as-of`) | LH F4 | not yet decomposed |
| 43 | Flow analytics (`arm stats`) | LH F5 | not yet decomposed |
| 44 | Ops compaction and snapshot checkpoints | LH C2 | not yet decomposed |
| 45 | Redaction firewall for durable ops | LH C5 | not yet decomposed |
| 46 | Extensibility seam for custom issue types | Addendum G4 | `TOPTIER-S14` |

Items 39 (Strategy Memo), 40 (`TOPTIER-S9`), and 47 (`TOPTIER-S15`) are **delivered** and live in the v0.1.0 archive. G4 stays at the tail. The addendum framed it as low urgency until there are external adopters.

## How to keep this page

Ordinals are a line, not a score. Ties inside a tier are not a build sequence. `blocked_by` cannot express this cross-document order, so do not invent edges just to encode a tier.

When a story is filed from a PR review, a dogfood theme, or a grilling session, add a row at filing time. Use a sub-ordinal (`14a`) when it is a follow-on, so source-document numbers stay stable. A delivery pulled forward from a lower tier keeps its original number. Record the reason. Do not silently re-tier it.

Update the story column when an item is filed and when it is delivered. Delivered rows move to the archive. This page should stay the remaining line.
