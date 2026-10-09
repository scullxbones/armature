# Next-Work Sequencing

**Date:** 2026-10-08. Gardened after **v0.1.0** shipped; this page tracks the remaining ranked backlog. No named successor release yet.

**Closed history:** Tier S, the Tier A feature spine, delivered pull-ins, the 2026-09-18 recommended sequence, and the F2 grilling decisions are in [`docs/archive/next-work-sequencing-closed-through-2026-09-27.md`](../archive/next-work-sequencing-closed-through-2026-09-27.md). v0.1.0 cut deliveries (items 31, 34, 35, 40; S7-T1; S11 T1–T3) are in [`docs/archive/next-work-sequencing-closed-v010.md`](../archive/next-work-sequencing-closed-v010.md). Ordinals below match that snapshot. Do not renumber them.

**Purpose:** One execution order across the planning rounds (`top-tier-gap-analysis.md`, `long-horizon-proposals.md`, `the-next-ten.html`, `narrow-gaps-addendum.md`) plus later filings that earned a row. Armature's DAG orders work inside a tree. It does not rank independent proposals that share no `blocked_by` edge. This file is that ranking. When an item is decomposed, its DAG is authoritative inside the item. This file only orders between items.

Statuses below audited 2026-10-08 with `arm show`. Re-audit before dispatch.

## What to do next

v0.1.0 is shipped (`TOPTIER-S6` / `TOPTIER-S6-T3` **merged**; tag `v0.1.0`). `arm ready` is not a dispatch order. A coordinator that picks the first ready issue will treat held docs and Tier C extensibility as equal to real next work.

1. **CLAIMORD ownership spine** (product DAG, not an ordinal here): `CLAIMORD-W12` → `CLAIMORD-W13` → `CLAIMORD-W14`. `CLAIMORD-W11` is `merged`.
2. **Item 30 trailer / docs vertical:** `TOPTIER-S7-T2` (README TUI visual) → item 47 T2 (`TOPTIER-S15-T2`) → `CLAIMORD-W20` (also waits on the cut, now satisfied).
3. **Item 23, `TOPTIER-S16-T1`** — cheap Tier B authorship (addendum G6). Independent of the docs vertical.
4. **Hold item 22, `TOPTIER-S12`.** Leave it unless Brian needs G2. `TOPTIER-S18-T4` and item 24's T4 (doctor **D13**) wait on S12-T2 / related holders. That wait is not a reason to lift the hold. Doctor **D11** stays reserved for S12-T2. **D12** already landed (PR #198).
5. **Do not treat these as the successor queue.** `TOPTIER-S12-T1` (held), `TOPTIER-S14` / `TOPTIER-S14-T1` (Tier C, ready), and `bug-1783480206` (waits on S18-T4).

## Still open

### Tier B

| # | Item | Source | Armature story |
|---|---|---|---|
| 22 | Ops-branch backup and disaster recovery | Addendum G2 | `TOPTIER-S12` (**hold**). T1 (docs) is ready. T2 is doctor **D11** and waits on T1. `TOPTIER-S18-T4` was reparented here and waits on T2 |
| 23 | Authorship / copyright clarity for agent-authored commits | Addendum G6 | `TOPTIER-S16` |
| 24 | One merged-promotion path | LH D4 | `LNGHZN-S11` (ADR 0022). T1–T3 **delivered** (PR #307). **T4** (doctor **D13**) remains; waits on T2 plus the open `doctor.go` holders (`TOPTIER-S12-T2`, `TOPTIER-S18-T4`, `bug-1783480206`) |
| 25 | Redesign transition hooks | LH D2 | not yet decomposed |
| 26 | Event stream (`arm events --follow`) | LH F3 | not yet decomposed |
| 28 | The Harness Compatibility Contract | Next-Ten №08 | not yet decomposed |
| 29 | Model-tier dispatch policy | LH C8 | not yet decomposed |
| 30 | README quickstart rewrite | GAP D1 | `TOPTIER-S7`. T1 **merged** (PR #308). **T2** open (README TUI visual); unblocked now that `TOPTIER-S8-T3` is `merged` |
| 32 | The Second Substrate (foreign-repo dogfood) | Next-Ten №06 | not yet decomposed (dogfood themes from the v0.1.0 cut are optional backlog) |
| 33 | Session handoff bundle | LH C10 | not yet decomposed |

G2 and G6 sit early in this tier because the addendum called them cheap. Items 31 (shims), 34 (`TOPTIER-S6`), and 35 (`TOPTIER-S8`) are **delivered** with the v0.1.0 cut and live in the v0.1.0 archive. The old late-tier G1 row (item 36, `TOPTIER-S11`) and item 27 (`LNGHZN-S10`) remain in the 2026-09-27 archive.

### Tier C

| # | Item | Source | Armature story |
|---|---|---|---|
| 37 | Findings as a product loop (`arm finding`) | LH C9 | not yet decomposed |
| 38 | Scope/context suggestion from co-change mining | LH C7 | not yet decomposed |
| 39 | The Strategy Memo | Next-Ten №09 | not yet decomposed |
| 41 | Living Diagrams | Next-Ten №10 | not yet decomposed |
| 42 | Time-travel state (`--as-of`) | LH F4 | not yet decomposed |
| 43 | Flow analytics (`arm stats`) | LH F5 | not yet decomposed |
| 44 | Ops compaction and snapshot checkpoints | LH C2 | not yet decomposed |
| 45 | Redaction firewall for durable ops | LH C5 | not yet decomposed |
| 46 | Extensibility seam for custom issue types | Addendum G4 | `TOPTIER-S14` |
| 47 | Human-newcomer onboarding diagnostics | Addendum G5 | `TOPTIER-S15`. T1 is merged (PR #188). The story stays open for T2 (README troubleshooting appendix), blocked by `TOPTIER-S7-T2` |

Item 40 (`TOPTIER-S9`, community/contribution scaffolding) is **delivered** (`merged`) and lives in the v0.1.0 archive. G4 and G5 stay at the tail. The addendum framed both as low urgency until there are external adopters.

## How to keep this page

Ordinals are a line, not a score. Ties inside a tier are not a build sequence. `blocked_by` cannot express this cross-document order, so do not invent edges just to encode a tier.

When a story is filed from a PR review, a dogfood theme, or a grilling session, add a row at filing time. Use a sub-ordinal (`14a`) when it is a follow-on, so source-document numbers stay stable. A delivery pulled forward from a lower tier keeps its original number. Record the reason. Do not silently re-tier it.

Update the story column when an item is filed and when it is delivered. Delivered rows move to the archive. This page should stay the remaining line.
