# Next-Work Sequencing — closed S11-T4, HOSTREF-S1, and MCPSTDIO-S1

**Superseded by:** [`docs/design/next-work-sequencing.md`](../design/next-work-sequencing.md). That file is the living order. This snapshot records product that landed on `main` after the 2026-11-01 garden so the living page stays remaining work only.

**Status:** Historical. Do not dispatch from this file.

**Earlier closed history:** Tier S / A through 2026-09-27 in [`next-work-sequencing-closed-through-2026-09-27.md`](next-work-sequencing-closed-through-2026-09-27.md). v0.1.0 cut + post-cut leftovers in [`next-work-sequencing-closed-v010.md`](next-work-sequencing-closed-v010.md). S12 → W21 product delivery in [`next-work-sequencing-closed-s12-w21.md`](next-work-sequencing-closed-s12-w21.md). That S12 file left item **24 T4** on the living page. v0.1.0 left the same trailer there. This file records the later landing.

**Audit:** `origin/main` @ `14ee59886` and `origin/_armature` @ `c4ac94992` (2026-11-15). `arm show` / `arm list` / `arm ready --explain` read that ops tip.

---

## Delivered on `main` (product path)

Ordinals match the living page. Do not renumber.

| # | Item | Source | Armature story | Delivery |
|---|---|---|---|---|
| 24 (T4) | One merged-promotion path | LH D4 | `LNGHZN-S11-T4` | Doctor **D13** live **on `main`** (PR #444, `3e9ca4905`). T1–T3 already **merged** (PR #307; `arm show` `merged`). D13 reservation is gone (`internal/doctor` live check IDs include D13) |
| 28a | Host-ref / GitHub Issues join | `docs/design/host-ref.md` | `HOSTREF-S1` | T1–T9 **on `main`** (PRs #445–#453) plus reader-order fix #455 (`a379a26ab`). Bidirectional Issues sync stays a non-goal under tripwire **T3**. No `HOSTREF` epic |
| 28b | Session stdio MCP (`arm mcp`) | session-stdio design | `MCPSTDIO-S1` | Story **on `main`** (PR #460, `bec4f3d`). Commit body lands T1–T5: constitution T1 exemption, `Invocation.Stdin`, newline session, Hidden `arm mcp`, and in-process `ref` / `ref-read` / `list --external-ref`. [ADR 0024](../adr/0024-arm-mcp-stdio-session-is-not-t1.md) still amends **T1** for that session shape |

HOSTREF task PRs on `main`, in task order: T1 #446 `3ac4fa321`, T2 #447 `539970614`, T3 #445 `ccf6ac6c0`, T4 #448 `ac3a03fbd`, T5 #453 `737cbc81d`, T6 #450 `4d79a7c83`, T7 #449 `8d3ec016e`, T8 #451 `3239253e4`, T9 #452 `2aee1cfc1`.

## Ops log has not caught up (I6)

`done` and `merged` in the DAG are distinct from a commit on `main`. These issues are still open on `origin/_armature` @ `c4ac94992`. Do not claim them to rebuild the PRs above.

| ID | `arm show` status | `arm ready --explain` |
|---|---|---|
| `HOSTREF-S1` | `in-progress` | (story; children below) |
| `HOSTREF-S1-T1` | `claimed` (`15e93628-ef00-4e33-b0e9-d6f1775ffd57`) | claimed, not in the ready queue |
| `HOSTREF-S1-T2` | `open` | blocker not merged: `HOSTREF-S1-T1` |
| `HOSTREF-S1-T3` | `claimed` (same worker as T1) | claimed, not in the ready queue |
| `HOSTREF-S1-T4` | `open` | blocker not merged: `HOSTREF-S1-T3` |
| `HOSTREF-S1-T5` | `open` | blockers not merged: `HOSTREF-S1-T2`, `HOSTREF-S1-T3` |
| `HOSTREF-S1-T6` | `claimed` (`1eb85bb5-31d4-47b6-ae69-198484fc80a5`) | claimed, not in the ready queue |
| `HOSTREF-S1-T7` | `claimed` (`332489d9-ce9e-44d7-a94e-f596fc584d0d`) | claimed, not in the ready queue |
| `HOSTREF-S1-T8` | `claimed` (`d0a84863-8933-41b1-8c4b-1694bc4bf819`) | claimed, not in the ready queue |
| `HOSTREF-S1-T9` | `claimed` (`1229b139-eb5d-4d90-9646-73b0d6a38c2a`) | claimed, not in the ready queue |
| `MCPSTDIO-S1` | `open` | `blocked_by` `HOSTREF-S1-T5`, `HOSTREF-S1-T6`, `HOSTREF-S1-T9` |
| `MCPSTDIO-S1-T1` | `open` | same HOSTREF blockers as the story |
| `MCPSTDIO-S1-T2` | `open` | same HOSTREF blockers as the story |
| `MCPSTDIO-S1-T3` | `open` | blocker not merged: `MCPSTDIO-S1-T2` |
| `MCPSTDIO-S1-T4` | `open` | blockers not merged: `MCPSTDIO-S1-T1`, `MCPSTDIO-S1-T3` |
| `MCPSTDIO-S1-T5` | `open` | blockers not merged: `MCPSTDIO-S1-T4`, `HOSTREF-S1-T5`, `HOSTREF-S1-T6`, `HOSTREF-S1-T9` |
| `LNGHZN-S11` | `in-progress` | story stays open while T4's op is open |
| `LNGHZN-S11-T4` | `open` | blockers not merged: `TOPTIER-S12-T2`, `TOPTIER-S18-T4`, `bug-1783480206` (`LNGHZN-S11-T2` is `merged` and is not in that explain line) |

`arm ready` (no `--explain`) on that tip returns three ids: `TOPTIER-S12-T1`, `TOPTIER-S14`, `TOPTIER-S14-T1`. It does not return HOSTREF, MCPSTDIO, or `LNGHZN-S11-T4`.

## Notes

- No named successor release. First external cut remains **v0.1.0**.
- Living page after this archive names **item 25** (LH D2, redesign transition hooks) as the next open Tier B ordinal. That is the lowest ordinal still on the living page, not a new ranking.
- `CLAIMORD-W21` is `merged` (`arm show`). `W21-EDGES` is not an issue (`arm show` → `SHOW-1`). `bug-1783480206` product is on `main` (#420, `b2901ac2f`); `arm show` is still `open`.
