# Next-Work Sequencing

**Date:** 2026-10-04. Gardened so this page tracks remaining work toward v0.1.0.

**Closed history:** Tier S, the Tier A feature spine, delivered pull-ins, the 2026-09-18 recommended sequence, and the F2 grilling decisions are in [`docs/archive/next-work-sequencing-closed-through-2026-09-27.md`](../archive/next-work-sequencing-closed-through-2026-09-27.md). Ordinals below match that snapshot. Do not renumber them.

**Purpose:** One execution order across the planning rounds (`top-tier-gap-analysis.md`, `long-horizon-proposals.md`, `the-next-ten.html`, `narrow-gaps-addendum.md`) plus later filings that earned a row. Armature's DAG orders work inside a tree. It does not rank independent proposals that share no `blocked_by` edge. This file is that ranking. When an item is decomposed, its DAG is authoritative inside the item. This file only orders between items.

Statuses below are the 2026-09-27 record. Re-audit with `arm show` before dispatch.

## What to do next

The Tier A feature spine is closed. `arm ready` is not a dispatch order. A coordinator that picks the first ready issue will treat held docs and Tier C extensibility as equal to the release front door.

1. **Item 24, `LNGHZN-S11` (ADR 0022).** Pulled forward of the S6 to S7 vertical. Closeout cannot see a squash or a stack land. Dispatch T1 when `TOPTIER-B1` (`engine.go`) is merged. T2 promotes one issue. T3 is `arm sync` and the hook. T4 is doctor **D13** and waits on T2 plus the open `doctor.go` holders (`TOPTIER-S12-T2`, `TOPTIER-S18-T4`, `bug-1783480206`).
2. **Items 34 then 30, when S11 is not the story in hand.** `TOPTIER-S6-T1` and `TOPTIER-S6-T2` are the front door. `TOPTIER-S7-T1` is blocked by `TOPTIER-S6-T1`.
3. **Hold item 22, `TOPTIER-S12`.** Leave it unless Brian needs G2. `TOPTIER-S18-T4` waits on S12-T2. That wait is not a reason to lift the hold. Doctor **D11** stays reserved for S12-T2. **D12** already landed (PR #198) and is lag, not disaster recovery.
4. **Do not treat these as the successor to the closed spine.** `TOPTIER-S12-T1` (held), `TOPTIER-S14` / `TOPTIER-S14-T1` (Tier C, ready), and `bug-1783480206` (waits on S18-T4).

## Still open

### Tier B

| # | Item | Source | Armature story |
|---|---|---|---|
| 22 | Ops-branch backup and disaster recovery | Addendum G2 | `TOPTIER-S12` (**hold**). T1 (docs) is ready. T2 is doctor **D11** and waits on T1. `TOPTIER-S18-T4` was reparented here and waits on T2 |
| 23 | Authorship / copyright clarity for agent-authored commits | Addendum G6 | `TOPTIER-S16` |
| 24 | One merged-promotion path | LH D4 | `LNGHZN-S11`, filed 2026-09-27 (ADR 0022). See "What to do next" |
| 25 | Redesign transition hooks | LH D2 | not yet decomposed |
| 26 | Event stream (`arm events --follow`) | LH F3 | not yet decomposed |
| 28 | The Harness Compatibility Contract | Next-Ten №08 | not yet decomposed |
| 29 | Model-tier dispatch policy | LH C8 | not yet decomposed |
| 30 | README quickstart rewrite | GAP D1 | `TOPTIER-S7`. T1 is blocked by `TOPTIER-S6-T1` |
| 31 | Shim-retirement policy | LH D3 | **delivered** (retirement paragraph in `ops-schema-compatibility.md`; known shims deleted; no plugin system). Move to archive on next garden |
| 32 | The Second Substrate (foreign-repo dogfood) | Next-Ten №06 | not yet decomposed |
| 33 | Session handoff bundle | LH C10 | not yet decomposed |
| 34 | Distribution and compatibility maturity | GAP T5 | `TOPTIER-S6`. T1 and T2 are the front-door vertical |
| 35 | Adopter positioning | GAP D3 | `TOPTIER-S8` |

G2 and G6 sit early in this tier because the addendum called them cheap. The old late-tier G1 row (item 36, `TOPTIER-S11`) is **delivered** and lives in the archive, as does item 27 (`LNGHZN-S10`), which was pulled forward and merged.

### Tier C

| # | Item | Source | Armature story |
|---|---|---|---|
| 37 | Findings as a product loop (`arm finding`) | LH C9 | not yet decomposed |
| 38 | Scope/context suggestion from co-change mining | LH C7 | not yet decomposed |
| 39 | The Strategy Memo | Next-Ten №09 | not yet decomposed |
| 40 | Community/contribution scaffolding | GAP D4 | `TOPTIER-S9` |
| 41 | Living Diagrams | Next-Ten №10 | not yet decomposed |
| 42 | Time-travel state (`--as-of`) | LH F4 | not yet decomposed |
| 43 | Flow analytics (`arm stats`) | LH F5 | not yet decomposed |
| 44 | Ops compaction and snapshot checkpoints | LH C2 | not yet decomposed |
| 45 | Redaction firewall for durable ops | LH C5 | not yet decomposed |
| 46 | Extensibility seam for custom issue types | Addendum G4 | `TOPTIER-S14` |
| 47 | Human-newcomer onboarding diagnostics | Addendum G5 | `TOPTIER-S15`. T1 is merged (PR #188). The story stays open for T2 (README troubleshooting appendix), blocked by `TOPTIER-S7-T2` |

G4 and G5 stay at the tail. The addendum framed both as low urgency until there are external adopters.

## How to keep this page

Ordinals are a line, not a score. Ties inside a tier are not a build sequence. `blocked_by` cannot express this cross-document order, so do not invent edges just to encode a tier.

When a story is filed from a PR review, a dogfood theme, or a grilling session, add a row at filing time. Use a sub-ordinal (`14a`) when it is a follow-on, so source-document numbers stay stable. A delivery pulled forward from a lower tier keeps its original number. Record the reason. Do not silently re-tier it.

Update the story column when an item is filed and when it is delivered. Delivered rows move to the archive. This page should stay the remaining line.
