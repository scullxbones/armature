# Next-Work Sequencing

**Date:** 2026-11-02. Gardened after the **S12 → W21** product cascade landed on `main`; this page tracks the remaining ranked backlog off the cut-era framing. No named successor release yet.

**Closed history:** Tier S, the Tier A feature spine, delivered pull-ins, the 2026-09-18 recommended sequence, and the F2 grilling decisions are in [`docs/archive/next-work-sequencing-closed-through-2026-09-27.md`](../archive/next-work-sequencing-closed-through-2026-09-27.md). v0.1.0 cut and post-cut leftover deliveries are in [`docs/archive/next-work-sequencing-closed-v010.md`](../archive/next-work-sequencing-closed-v010.md). S12 ops-backup / D11 / D14 and CLAIMORD-W21 product delivery are in [`docs/archive/next-work-sequencing-closed-s12-w21.md`](../archive/next-work-sequencing-closed-s12-w21.md). Ordinals below match that snapshot. Do not renumber them.

**Purpose:** One execution order across the planning rounds (`top-tier-gap-analysis.md`, `long-horizon-proposals.md`, `the-next-ten.html`, `narrow-gaps-addendum.md`) plus later filings that earned a row. Armature's DAG orders work inside a tree. It does not rank independent proposals that share no `blocked_by` edge. This file is that ranking. When an item is decomposed, its DAG is authoritative inside the item. This file only orders between items.

Statuses below audited 2026-11-02 against `origin/main` @ `d7b0809b` (#418 SHA-256 gitoid atop #420 residual `trls`) and the S12 cascade merge train. Re-audit before dispatch. Do not fold #419 (gitoid lint follow-ups) into this garden.

## What to do next

v0.1.0 is shipped. Post-cut leftovers (CLAIMORD W11–W14 + W20, S7 / S8 / S15 / S16) are **merged**. The Brian-authorized **S12 → W21** product path is **delivered** on `main` (S12-T1 #405, S12-T2 / D11 #408, S18-T4 / D14 #411, W21 identity #282, bug `trls` #420). `arm ready` is not a dispatch order. Do not invent a next release number until a cut task is filed.

1. **Next theme: item 28a, `HOSTREF-S1` (host-ref / GitHub Issues join).** This is the concrete ranked successor after leftovers and the S12 → W21 cascade. Ready entry points: `HOSTREF-S1-T1`, `HOSTREF-S1-T3`. Bidirectional GitHub Issues sync remains a non-goal under tripwire **T3**. Parent epic ID `HOSTREF` is absent; the story is a root today.
2. **Item 28b, `MCPSTDIO-S1`**, stays behind HOSTREF children (`HOSTREF-S1-T5`, `HOSTREF-S1-T6`, `HOSTREF-S1-T9`). [ADR 0024](../adr/0024-arm-mcp-stdio-session-is-not-t1.md) still amends **T1** for harness-spawned `arm mcp` stdio sessions. Do not start MCPSTDIO feature work ahead of those edges.
3. **Residual S12 → W21 ops are not a new theme.** Optional W21-EDGES and ops `done` → `merged` for `CLAIMORD-W21` are closeout / exclusivity. Do not fold them into HOSTREF PRs. W21 product code is already on `main` (#282); `bug-1783480206` is on `main` (#420).
4. **Item 24 T4** (`LNGHZN-S11-T4`, doctor **D13**) remains open on the living page; re-audit blockers before claiming. It is not the successor queue for HOSTREF.
5. **Do not treat Tier C** `TOPTIER-S14` / `TOPTIER-S14-T1` (ready) **as the next theme** just because it appears in `arm ready`. Other undecomposed Tier B/C rows stay ranked below.

## Still open

### Tier B

| # | Item | Source | Armature story |
|---|---|---|---|
| 24 | One merged-promotion path | LH D4 | `LNGHZN-S11` (ADR 0022). T1–T3 **delivered** (PR #307). **T4** (doctor **D13**) remains; prior S12 / bug product holders are on `main` — re-audit ops blockers before claim |
| 25 | Redesign transition hooks | LH D2 | not yet decomposed |
| 26 | Event stream (`arm events --follow`) | LH F3 | not yet decomposed |
| 28 | The Harness Compatibility Contract | Next-Ten №08 | not yet decomposed |
| 28a | Host-ref / GitHub Issues join | `docs/design/host-ref.md` | `HOSTREF-S1` (story root; no `HOSTREF` epic). **Next ranked theme.** Name ordered GitHub issue URLs, show current without fetch, print title/body once. Bidirectional Issues sync remains a non-goal under tripwire **T3**. Ready entry points: `HOSTREF-S1-T1`, `HOSTREF-S1-T3` |
| 28b | Session stdio MCP (`arm mcp`) | session-stdio design (source `7c9ac726-…`) | `MCPSTDIO-S1` (story root; no `MCPSTDIO` epic). `blocked_by` `HOSTREF-S1-T5`, `HOSTREF-S1-T6`, and `HOSTREF-S1-T9`. One invoked stdio session with one tool `arm`; [ADR 0024](../adr/0024-arm-mcp-stdio-session-is-not-t1.md) amends **T1** so a harness-spawned `arm mcp` stdio session that lives only as long as the parent's stdio pipes and exits on EOF is not a long-running process (no listening socket or port, no detaching or daemonizing, no surviving the parent, no background work after EOF) |
| 29 | Model-tier dispatch policy | LH C8 | not yet decomposed |
| 32 | The Second Substrate (foreign-repo dogfood) | Next-Ten №06 | not yet decomposed (dogfood themes from the v0.1.0 cut are optional backlog) |
| 33 | Session handoff bundle | LH C10 | not yet decomposed |

Item 22 (`TOPTIER-S12` / G2) and the S18-T4 D14 trailer live in the S12 → W21 archive. Items 23 (authorship / `TOPTIER-S16`), 30 (README / `TOPTIER-S7`), 31 (shims), 34 (`TOPTIER-S6` **merged**), and 35 (`TOPTIER-S8` **merged**) live in the v0.1.0 archive. The old late-tier G1 row (item 36, `TOPTIER-S11`) and item 27 (`LNGHZN-S10`) remain in the 2026-09-27 archive.

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
