# Next-Work Sequencing — closed with v0.1.0 and post-cut leftovers

**Superseded by:** [`docs/design/next-work-sequencing.md`](../design/next-work-sequencing.md). That file is the living order. This snapshot records Tier B/C rows delivered on the v0.1.0 path and the post-cut leftover train so the living page stays remaining work only.

**Status:** Historical. Do not dispatch from this file.

**Earlier closed history:** Tier S, Tier A, and the pre-cut snapshot through 2026-09-27 remain in [`next-work-sequencing-closed-through-2026-09-27.md`](next-work-sequencing-closed-through-2026-09-27.md).

---

## Delivered on the v0.1.0 cut

Ordinals match the living page / 2026-09-27 archive. Do not renumber.

| # | Item | Source | Armature story | Delivery |
|---|---|---|---|---|
| 24 (T1–T3) | One merged-promotion path | LH D4 | `LNGHZN-S11` | T1–T3 **merged** (PR #307, ADR 0022). Story stays `in-progress` for **T4** (doctor **D13**) — that trailer remains on the living page |
| 30 (T1) | README quickstart rewrite | GAP D1 | `TOPTIER-S7-T1` | **merged** (PR #308). T2 delivered in the post-cut leftovers section below |
| 31 | Shim-retirement policy | LH D3 | (policy + deletions) | **delivered**: retirement paragraph in `ops-schema-compatibility.md`; known shims deleted (PR #309); no plugin system |
| 34 | Distribution and compatibility maturity | GAP T5 | `TOPTIER-S6` | T1–T3 **merged** (T1 PR #304, T2 PR #305, T3 / cut PR #314). Story later **merged** in post-cut closeout. Tag [`v0.1.0`](https://github.com/scullxbones/armature/releases/tag/v0.1.0) |
| 35 | Adopter positioning | GAP D3 | `TOPTIER-S8` | Story **done**; T3 **merged** (PR #316). T1 and T2 remain `done`. why-armature, examples trail, adopter demo |
| 40 | Community/contribution scaffolding | GAP D4 | `TOPTIER-S9` | Story **done**; T1 **merged** (was still listed open on the pre-cut living page) |

## Delivered after v0.1.0 (post-cut leftovers, 2026-10-24)

| # | Item | Source | Armature story | Delivery |
|---|---|---|---|---|
| 23 | Authorship / copyright clarity for agent-authored commits | Addendum G6 | `TOPTIER-S16` | Story **merged**. T1 AUTHORSHIP.md (PR #326); T2 Strategy Memo authorship tripwire (PR #379) |
| 30 (T2 + story) | README quickstart rewrite | GAP D1 | `TOPTIER-S7` | T2 README TUI visual **merged** (PR #324). Story **merged** |
| 39 | The Strategy Memo | Next-Ten №09 | (filed via `TOPTIER-S16-T2`) | **delivered** as `docs/design/strategy-memo.md` (PR #379) |
| 47 | Human-newcomer onboarding diagnostics | Addendum G5 | `TOPTIER-S15` | Story **merged**. T1 already merged (PR #188); T2 troubleshooting appendix **merged** (PR #378) |

CLAIMORD product-DAG leftovers (not sequencing ordinals): W12–W14 **merged** (PRs #278–#280). `CLAIMORD-W20` code is on `main` (PR #281); ops closeout was **claimed** / mid-flight at garden time — living page notes **ops pending**.

## Notes

- First external cut is **v0.1.0** (`TOPTIER-S6-T3`). Do not redefine “1.0.” No named successor release yet.
- Post-cut closeout (2026-10-08): `arm merged` promoted `CLAIMORD-W11`, `TOPTIER-S6-T3`, and `TOPTIER-S8-T3` from `done` → `merged`. `TOPTIER-S6` later reached **merged**.
- Held item 22 (`TOPTIER-S12`) was **not** lifted. Doctor cascade `S18-T4` → `bug-1783480206` → `LNGHZN-S11-T4` stays behind that hold.
