# Next-Work Sequencing — closed 2026-11-15 ranking (pre ADR 0026)

**Superseded by:** [`docs/design/next-work-sequencing.md`](../design/next-work-sequencing.md). That file is the living order. This snapshot is the 2026-11-15 garden: item 25 as the next theme, then the open Tier B and Tier C rows.

**Status:** Historical. Do not dispatch from this file.

**Closed by:** [ADR 0026](../adr/0026-armature-1-0-scope-keep-kill-defer.md) (15 November 2026 re-groom). That ADR keeps, kills, or defers the rows below. Ordinals are unchanged.

| # | ADR 0026 disposition |
|---|---|
| 25 | Keep. Phase 3, with a rollout dogfood. |
| 26 | Kill. Item 25's `on:` filter covers the event vocabulary. |
| 28 | Keep, expanded to five harnesses. Phase 2. Canary CI stays deferred. |
| 29 | Kill. Harnesses pick models themselves. |
| 32 | Keep, as the landing page built with armature. Phase 4. |
| 33 | Kill. The PR is the cloud-agent handoff. |
| 37 | Absorbed into the Cursor setup (`arm mcp` collects dogfood findings). |
| 38 | Defer. Revive if scope gaps recur after N3. |
| 41 | Keep. Phase 4. |
| 42 | Kill. Intent is HOSTREF-S2. State history is `arm log --since --issue` and `arm context-history`. |
| 43 | Defer. Revive after a month of N1's real done/merged times. |
| 44 | Defer. Workstream 4 replaces it. Revive only if that workstream misses 10–100 ms. |
| 45 | Keep, as a CI secret scan of `_armature`. May land anytime. |
| 46 / `TOPTIER-S14` | Defer (hold, not cancel). Revive on the first partner request. |

**Earlier closed history:** Tier S / A through 2026-09-27 in [`next-work-sequencing-closed-through-2026-09-27.md`](next-work-sequencing-closed-through-2026-09-27.md). v0.1.0 cut + post-cut leftovers in [`next-work-sequencing-closed-v010.md`](next-work-sequencing-closed-v010.md). S12 → W21 product delivery in [`next-work-sequencing-closed-s12-w21.md`](next-work-sequencing-closed-s12-w21.md). Item 24 T4, HOSTREF-S1, and MCPSTDIO-S1 in [`next-work-sequencing-closed-s11-hostref-mcpstdio.md`](next-work-sequencing-closed-s11-hostref-mcpstdio.md).

---

## Snapshot (living page as of 2026-11-15)

**Date:** 2026-11-15. Gardened against `origin/main` @ `14ee59886` and `origin/_armature` @ `c4ac94992`. No named successor release yet.

Statuses audited 2026-11-15 with `arm list`, `arm show`, and `arm ready` on `origin/_armature` @ `c4ac94992`, and with `git log` on `origin/main` @ `14ee59886`.

### What this garden said to do next

v0.1.0 is shipped. Post-cut leftovers and the S12 → W21 product path are delivered. Item 24 including doctor **D13** is on `main` (PR #444). Item 28a `HOSTREF-S1` is on `main` (PRs #445–#453 and #455). Item 28b `MCPSTDIO-S1` is on `main` (PR #460, `bec4f3d`). `arm ready` is not a dispatch order.

1. **Next theme: item 25, Redesign transition hooks (LH D2).** Lowest open ordinal remaining in Tier B. Items 24, 28a, and 28b moved to the archive in this garden. Ordinals were not renumbered and no row was re-tiered. The story column was still "not yet decomposed".
2. **Ops closeout for that archived product is not a theme.** `arm show` on `c4ac94992` still has `HOSTREF-S1` `in-progress`, `MCPSTDIO-S1` `open` (`blocked_by` `HOSTREF-S1-T5`, `HOSTREF-S1-T6`, `HOSTREF-S1-T9`), and `LNGHZN-S11-T4` `open`. `arm ready --explain` names those unmet edges. The per-id table is in [`next-work-sequencing-closed-s11-hostref-mcpstdio.md`](next-work-sequencing-closed-s11-hostref-mcpstdio.md). Do not claim those ids to rebuild PRs already on `main`.
3. **`arm ready` on this tip is `TOPTIER-S12-T1`, `TOPTIER-S14`, and `TOPTIER-S14-T1`.** S12 product is already on `main` (T1 #405, D11 #408). `arm show CLAIMORD-W21` is `merged`. `arm show W21-EDGES` returns `SHOW-1` (no such issue). `bug-1783480206` is on `main` (#420) and `arm show` is still `open` (`arm ready --explain`: blocker `TOPTIER-S18-T4` not merged).
4. **Item 28** (Harness Compatibility Contract) stayed at ordinal 28, behind 25 and 26. Sub-ordinals 28a and 28b are the archived follow-ons.
5. **Tier C `TOPTIER-S14` / `TOPTIER-S14-T1` were `arm ready` and were not this theme.** Other open Tier B rows stayed ranked above them.

### Tier B (open on this snapshot)

Story cells that say **not yet decomposed** were checked on `origin/_armature` @ `c4ac94992`: `arm list` returned 818 issues, and the only non-terminal stories were `HOSTREF-S1`, `LNGHZN-S11`, `MCPSTDIO-S1`, `TOPTIER-S12`, and `TOPTIER-S14`. None of those rows has a delivery subject on `origin/main` @ `14ee59886`.

| # | Item | Source | Armature story |
|---|---|---|---|
| 25 | Redesign transition hooks | LH D2 | not yet decomposed |
| 26 | Event stream (`arm events --follow`) | LH F3 | not yet decomposed |
| 28 | The Harness Compatibility Contract | Next-Ten №08 | not yet decomposed |
| 29 | Model-tier dispatch policy | LH C8 | not yet decomposed |
| 32 | The Second Substrate (foreign-repo dogfood) | Next-Ten №06 | not yet decomposed (dogfood themes from the v0.1.0 cut are optional backlog) |
| 33 | Session handoff bundle | LH C10 | not yet decomposed |

Item 22 (`TOPTIER-S12` / G2) and the S18-T4 D14 trailer live in the S12 → W21 archive. Item 24 (T1–T3 in the v0.1.0 archive, T4 / D13 in the S11 archive) and items 28a / 28b live in the closed-history links above. Items 23 (authorship / `TOPTIER-S16`), 30 (README / `TOPTIER-S7`), 31 (shims), 34 (`TOPTIER-S6` merged), and 35 (`TOPTIER-S8` merged) live in the v0.1.0 archive. The old late-tier G1 row (item 36, `TOPTIER-S11`) and item 27 (`LNGHZN-S10`) remain in the 2026-09-27 archive.

### Tier C (open on this snapshot)

| # | Item | Source | Armature story |
|---|---|---|---|
| 37 | Findings as a product loop (`arm finding`) | LH C9 | not yet decomposed |
| 38 | Scope/context suggestion from co-change mining | LH C7 | not yet decomposed |
| 41 | Living Diagrams | Next-Ten №10 | not yet decomposed |
| 42 | Time-travel state (`--as-of`) | LH F4 | not yet decomposed |
| 43 | Flow analytics (`arm stats`) | LH F5 | not yet decomposed. `arm stats --cost` is the delivered item 36 / G1.2 view (PR #161). `cmd/armature/stats.go` states that LH F5 latency and rework analytics are not implemented |
| 44 | Ops compaction and snapshot checkpoints | LH C2 | not yet decomposed |
| 45 | Redaction firewall for durable ops | LH C5 | not yet decomposed |
| 46 | Extensibility seam for custom issue types | Addendum G4 | `TOPTIER-S14` story `open`. `TOPTIER-S14-T1` `open` and in `arm ready` (blockers `LNGHZN-S10-T4`, `LNGHZN-S10-T12`, `LNGHZN-S10-T5`, `TOPTIER-S18-T2` are `merged`). `TOPTIER-S14-T2` `open`, `blocked_by` `TOPTIER-S14-T1` (`arm ready --explain`) |

The Tier B audit note covers the **not yet decomposed** cells in this table too. Items 39 (Strategy Memo), 40 (`TOPTIER-S9`), and 47 (`TOPTIER-S15`) are delivered and live in the v0.1.0 archive. G4 stayed at the tail. The addendum framed it as low urgency until there are external adopters.
