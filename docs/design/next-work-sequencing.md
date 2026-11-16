# Next-Work Sequencing

**Date:** 2026-11-16. 1.0 scope is [ADR 0026](../adr/0026-armature-1-0-scope-keep-kill-defer.md), from the 15 November 2026 re-groom. No named successor release yet. The 1.0 cut is the end of Phase 4, not a version number invented here.

**Closed history:** Tier S, the Tier A feature spine, delivered pull-ins, the 2026-09-18 recommended sequence, and the F2 grilling decisions are in [`docs/archive/next-work-sequencing-closed-through-2026-09-27.md`](../archive/next-work-sequencing-closed-through-2026-09-27.md). v0.1.0 cut and post-cut leftover deliveries are in [`docs/archive/next-work-sequencing-closed-v010.md`](../archive/next-work-sequencing-closed-v010.md). S12 ops-backup / D11 / D14 and CLAIMORD-W21 product delivery are in [`docs/archive/next-work-sequencing-closed-s12-w21.md`](../archive/next-work-sequencing-closed-s12-w21.md). Item 24's T4 trailer (doctor **D13**), item 28a (`HOSTREF-S1`), and item 28b (`MCPSTDIO-S1`) are in [`docs/archive/next-work-sequencing-closed-s11-hostref-mcpstdio.md`](../archive/next-work-sequencing-closed-s11-hostref-mcpstdio.md). The 2026-11-15 ranking (item 25 as the next theme, open Tier B and Tier C) is in [`docs/archive/next-work-sequencing-closed-2026-11-15-pre-adr-0026.md`](../archive/next-work-sequencing-closed-2026-11-15-pre-adr-0026.md). Ordinals below match that snapshot. Do not renumber them.

**Purpose:** One execution order across the planning rounds (`top-tier-gap-analysis.md`, `long-horizon-proposals.md`, `the-next-ten.html`, `narrow-gaps-addendum.md`) plus later filings that earned a row. Armature's DAG orders work inside a tree. It does not rank independent proposals that share no `blocked_by` edge. This file is that ranking. When an item is decomposed, its DAG is authoritative inside the item. This file only orders between items.

ADR 0026 is the 1.0 ranking. External adoption is the end goal. This page is the build order up to the 1.0 cut. Value rank is the keep table in the ADR. Phases below are the build order: skills diet is value-rank 5 and still Phase 0, because it depends on N1.

The 2026-11-15 ops audit still stands for closeout, and it is not a theme. `arm show` on `origin/_armature` @ `c4ac94992` still had `HOSTREF-S1` `in-progress`, `MCPSTDIO-S1` `open`, and `LNGHZN-S11-T4` `open`. The per-id table is in the S11 archive. Do not claim those ids to rebuild PRs already on `main`. `arm ready` is not a dispatch order. On that tip it listed `TOPTIER-S12-T1`, `TOPTIER-S14`, and `TOPTIER-S14-T1`. S12 product is already on `main`. `TOPTIER-S14` is deferred below.

## What to do next

v0.1.0 is shipped. Item 24 including doctor **D13** is **on `main`** (PR #444). Item 28a and item 28b are **on `main`** (see the S11 archive). Do not invent a release number until a cut task is filed after Phase 4.

**Phase 0 — truth.** N1 promotion from the host, then the skills diet. The diet depends on N1. N1's design (R1–R4, the pre-merge `promotion-ready` check, the post-merge Action, jidoka) is decided in ADR 0026 and is not built yet. Skills diet: remove coordinator, activity-indexer, and auditor from default bootstrap into an opt-in pack; keep reviewer; rewrite worker for cloud agents.

**Phase 1 — speed.** Workstream 4, materialization speed. This replaces item 44. Target 10–100 ms. Today: warm ~956 ms, cold ~5.2 s, `render-context` ~5.8 s. The cost is git subprocesses and blob re-reads.

**Phase 2 — reach.** Item 28, the harness contract, for five harnesses (Claude, Codex, Devin, Cursor, Grok Build). Then the Cursor setup: required `arm mcp` and dogfood collection, which absorbs item 37. Then N3: scope at claim is the contract. Item 28's canary CI is not in this phase.

**Phase 3 — leverage.** Item 25 transition hooks, built with a rollout dogfood. Then HOSTREF-S2: GitHub issues as context and citation. Code stays the source of truth.

**Phase 4 — trust.** Item 41 Living Diagrams. Then item 32, the landing page built with armature (the second substrate).

**Then the 1.0 cut.**

**Anytime.** Item 45, a CI secret scan of `_armature`. It does not wait on a phase.

## Still open

### 1.0 keep line

Story cells that said **not yet decomposed** on 2026-11-15 were checked on `origin/_armature` @ `c4ac94992`: `arm list` returned 818 issues, and the only non-terminal stories were `HOSTREF-S1`, `LNGHZN-S11`, `MCPSTDIO-S1`, `TOPTIER-S12`, and `TOPTIER-S14`. Re-audit before filing a story from a row.

| Phase | Rank | Item | Source | Armature story |
|---|---|---|---|---|
| 0 | 1 | N1 promotion from the host | ADR 0026 | not yet decomposed. Design is the ADR. Item 24 T4 is done (#444) |
| 0 | 5 | Skills diet | ADR 0026 skills audit | not yet decomposed. Depends on N1 |
| 1 | 4 | Materialization speed | replaces item 44 / LH C2 | not yet decomposed. Target 10–100 ms |
| 2 | 2 | Harness contract, five harnesses | item 28 / Next-Ten №08 | not yet decomposed. 28a and 28b are delivered |
| 2 | 3 | Cursor setup, required `arm mcp`, dogfood collection | ADR 0026; absorbs item 37 / LH C9 | not yet decomposed. `arm mcp` itself is on `main` (ADR 0024, PR #460) |
| 2 | 8 | N3 scope at claim is the contract | ADR 0026 | not yet decomposed |
| 3 | 7 | Transition hooks, with a rollout dogfood | item 25 / LH D2 | not yet decomposed |
| 3 | 6 | HOSTREF-S2, issues as context and citation | ADR 0026 | not yet decomposed. HOSTREF-S1 is delivered |
| 4 | 9 | Living Diagrams | item 41 / Next-Ten №10 | not yet decomposed |
| 4 | 10 | Landing page built with armature | item 32 / Next-Ten №06 | not yet decomposed |
| any | 11 | CI secret scan of `_armature` | item 45 / LH C5 | not yet decomposed |

Item 22 and the S18-T4 D14 trailer live in the S12 → W21 archive. Item 24 and items 28a / 28b live in the closed-history links above. Items 23, 30, 31, 34, and 35 live in the v0.1.0 archive. Item 36 and item 27 remain in the 2026-09-27 archive.

### Deferred

Hold these. Do not build them until the revive trigger fires. `TOPTIER-S14` may still show up in `arm ready`; that does not put it on the 1.0 line.

| # | Item | Revive trigger | Armature story |
|---|---|---|---|
| 46 | Extensibility seam for custom issue types | First partner request. Hold, do not cancel | `TOPTIER-S14` story `open`. `TOPTIER-S14-T1` `open` (blockers `LNGHZN-S10-T4`, `LNGHZN-S10-T12`, `LNGHZN-S10-T5`, `TOPTIER-S18-T2` are `merged` on the 2026-11-15 audit). `TOPTIER-S14-T2` `open`, `blocked_by` `TOPTIER-S14-T1` |
| 38 | Scope/context suggestion from co-change mining | Scope gaps recur after N3 | not yet decomposed |
| 28 (canary only) | Item 28 canary CI | A harness release breaks a fixture | not yet decomposed. The harness contract itself is on the keep line |
| 43 | Flow analytics / latency | A month of N1's real done/merged times | not yet decomposed. `arm stats --cost` is the delivered item 36 / G1.2 view (PR #161). `cmd/armature/stats.go` states that LH F5 latency and rework analytics are not implemented |
| 44 | Ops compaction and snapshot checkpoints | Workstream 4 misses 10–100 ms | not yet decomposed |
| — | Task-scope idea 3, post-Bash diff | A harness offers a post-shell hook; it then becomes a detect column | not filed |
| — | Task-scope idea 4, protect hook config | A partner runs untrusted agents. Document per-harness managed-config support with the harness contract now | not filed |

### Killed

Items 26 (event stream), 29 (model-tier dispatch), 33 (session handoff bundle), and 42 (time-travel state) are out. Reasons are in ADR 0026. The 2026-11-15 rows are in the pre-ADR archive. Do not file stories for them.

Items 39 (Strategy Memo), 40 (`TOPTIER-S9`), and 47 (`TOPTIER-S15`) are **delivered** and live in the v0.1.0 archive.

## How to keep this page

Ordinals are a line, not a score. `blocked_by` cannot express this cross-document order, so do not invent edges just to encode a phase.

ADR 0026 owns keep, kill, and defer for the 1.0 cut. A new keep, kill, or revive changes that ADR (a new ADR that supersedes or amends it), then this page. Do not silently re-rank.

When a story is filed from a PR review, a dogfood theme, or a grilling session, add a row at filing time. Use a sub-ordinal (`14a`) when it is a follow-on, so source-document numbers stay stable. A delivery pulled forward keeps its original number. Record the reason.

Update the story column when an item is filed and when it is delivered. Delivered rows move to the archive. Killed rows move to the archive with the ADR reason. Deferred rows stay here with the revive trigger. This page should stay the remaining line.
