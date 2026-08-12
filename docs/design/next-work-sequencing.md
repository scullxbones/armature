# Next-Work Sequencing — A Cross-Document Execution Order

**Date:** 2026-07-07 (updated 2026-07-08 — F2 grilling session added `TOPTIER-S17` and `LNGHZN-S2`; updated 2026-07-19 — Tier S delivered; Tier A's remaining items decomposed as `LNGHZN-S3`–`S8` and `NXTTN-S3`/`S4`; updated 2026-08-11 — Tier A status audit: items 10–14 and 17 delivered, `LNGHZN-S9` filed as item 14a)
**Purpose:** A single execution order across all proposals from the three planning rounds — `docs/design/top-tier-gap-analysis.md` (GAP), `docs/design/long-horizon-proposals.md` (LH), `docs/design/the-next-ten.html` (Round Three) — plus the `docs/design/narrow-gaps-addendum.md` items (G1–G6, tracked as `TOPTIER-S11`–`S16`), which are now woven into the tiers below at the rough tier each was originally recommended for, rather than listed separately.

**Why this lives in markdown, not in Armature:** Armature's DAG models dependency and scope within a single epic/story tree — `blocked_by` edges, scope overlap, wave dispatch. It has no concept of *cross-document, cross-epic priority ordering* across independent proposals that don't share a scope or a `blocked_by` edge but still have a preferred execution order (e.g., "write the constitution before the census, even though nothing blocks the census on it"). Only the gap-analysis items are currently modeled as `TOPTIER` stories; the long-horizon and Round Three items are not yet decomposed into issues at all, and several of them (documents, policies, an internal memo) are not naturally issue-shaped work in the first place. Forcing this ordering into `blocked_by` edges would either be false precision (most of these items are *not* hard-blocked on each other) or would require inventing an epic spanning three separate planning documents that don't share a DAG today. This document is the citable ordering until — if ever — that changes.

---

## The ranked list (Tier S → C)

Ranking combines each document's own scoring (Round Three's Σ/30, LH's six-axis table for C1–C10) with cross-document dependency, dogfood-corpus evidence weight, and expiry urgency (several items are only cheap before `v0.1.0` ships).

### Tier S — foundational, do first (everything else cites these, or their window closes soon)

| # | Item | Source | Armature story |
|---|---|---|---|
| 1 | Doc corpus hygiene / archive (incl. ADR template) | GAP D5 | `TOPTIER-S10` |
| 2 | The Armature Constitution | Next-Ten №01 | `NXTTN-S1B` (core ratified on `main` via ADR 0009/`CONSTITUTION.md`; only the ADR-template field remains) |
| 3 | Skill lint / golden-transcript tests | GAP T1 | `TOPTIER-S1` |
| 4 | The Subtractive Release (surface census) | Next-Ten №02 | `NXTTN-S2` |
| 5 | Envelope/schema documentation | GAP D2 | `TOPTIER-S2` |
| 6 | Collapse `.arm/.armature/` dotdir | LH D5 | `LNGHZN-S1` (precursor bug `LNGHZN-B1`; release-gates `TOPTIER-S6-T3`) |
| 7 | The CLI Grammar Contract | Next-Ten №05 | `NXTTN-S5` |
| 8 | Scope-overlap validation gaps (plan-time scope-overlap checker) | Dogfood theme `scope-overlap-validation-gaps` | `TOPTIER-S17` (decomposed: T1 leading/unblocked, T2→T3→T4 sequenced on shared `validate.go` scope, T5 blocked by T1–T4) |
| 9 | Scope-disjoint wave planning (`arm ready --waves`) | LH F2 | `LNGHZN-S2` (decomposed: T1 blocked by `TOPTIER-S17-T1`, T2 unblocked, T3 blocked by T1; existing `TOPTIER-S1-T2` golden-transcript task now also blocked by `LNGHZN-S2-T1`/`T3`) |

Note: D5 is pulled forward from Tier C because its ADR-hygiene sub-item (D5.2) creates the ADR template the Constitution's "principles touched" field rides on — see Next-Ten №01. D5.1 and D5.3 travel with it since D5 is treated as one coherent unit (doc-corpus hygiene is small enough not to warrant splitting).

Note on items 8–9 (added 2026-07-08, F2 grilling session): F2 (`arm ready --waves`, item 9) partitions the ready queue into scope-disjoint dispatch waves using the same glob-aware overlap primitive (`claimPkg.ScopesOverlap`, `internal/claim/overlap.go`) that `arm claim` already uses at claim time. That primitive has a known blind spot — documented in the `scope-overlap-validation-gaps` dogfood theme — where it misreads parent/child scope containment (a story's scope is the union of its children's by design) as a conflict. `story` is a ready-eligible type (`internal/issuetype/issuetype.go`), so a parent story and its own ready child task can co-occur in the ready queue, meaning F2's wave-partitioning would inherit this bug and manufacture false conflicts between every story and its children. **Item 8 (`TOPTIER-S17`) is therefore sequenced immediately before item 9 and is a hard prerequisite**, not an optional adjacent cleanup. The theme documents four distinct defects; only one of them (parent/child containment) blocks F2 — the grilling session concluded the other three do not apply to wave-partitioning (see full rationale below the tables). `TOPTIER-S17`'s leading task should fix the parent/child containment blind spot in `ScopesOverlap` itself (using `dag.Graph`'s existing ancestor/descendant queries), benefiting both the existing `arm claim` check and F2 — `LNGHZN-S2` blocks on that task specifically, not on the full `TOPTIER-S17` story.

### Tier A — high impact, second wave

| # | Item | Source | Armature story | Status (2026-08-11) |
|---|---|---|---|---|
| 10 | End-to-end workflow test harness | GAP T2 | `TOPTIER-S3` | delivered |
| 11 | Crash/recovery resilience | GAP T3 | `TOPTIER-S4` | delivered |
| 12 | Autonomic heartbeats via harness hook | LH C1 | `LNGHZN-S3` | delivered |
| 13 | Transition-time delivery gate | LH C4 | `LNGHZN-S4` | delivered |
| 14 | Managed worktree lifecycle | LH F1 | `LNGHZN-S5` | delivered (PR #89) |
| 14a | `arm claim --from` for sub-task worktrees | LH F1.1 (F1 follow-on) | `LNGHZN-S9` | open |
| 15 | Context Economics | Next-Ten №03 | `NXTTN-S3` | open |
| 16 | Agent-grade error contract | LH C3 | `LNGHZN-S6` | open |
| 17 | Scope enforcement hardening | GAP T4 | `TOPTIER-S5` | delivered |
| 18 | The Paved Road | Next-Ten №04 | `NXTTN-S4` | open (T2 cancelled) |
| 19 | Make configuration honest | LH D1 | `LNGHZN-S7` | open |
| 20 | Reviewer self-validation (`arm review validate`) | LH C6 | `LNGHZN-S8` | open |
| 21 | Reviewer disagreement / consensus policy | Addendum G3 | `TOPTIER-S13` | open |

Note: item 21 (G3) sits at the A/B boundary in the addendum's own scoring (high evidence, moderate cost) and is placed at the tail of Tier A rather than the head of Tier B; treat its position as a tie with the adjacent Tier B items, not a strict ranking.

Note on item 14a (added 2026-08-11): `LNGHZN-S9` was filed during `LNGHZN-S5`'s PR #89 review (thread `claim.go:666`) and is direct spillover from F1 — it closes the case F1's managed-provisioning path does not cover, cutting a sub-task branch live from an already-open story worktree rather than adopting an externally pre-created branch after the fact. It is numbered `14a` rather than inserted as a new ordinal so the existing 1–47 numbering (cited from the source documents) does not shift. It carries no `blocked_by` edge and is not a prerequisite for anything else in Tier A; its position reflects topical adjacency to item 14, not a claim that it must precede items 15–21.

Note on Tier A status (audited 2026-08-11): items 10–14 and 17 are merged; `LNGHZN-S5` (item 14) was closed out on this date after all ten of its child tasks and PR #89 had merged. The six remaining open items — 14a, 15, 16, 18, 19, 20, 21 — carry **no `blocked_by` edges between them and are all simultaneously in the ready queue**, so their order is governed by this document alone, not by the DAG. Recommended order among them, by impact: 16 (`LNGHZN-S6`, LH's highest-scoring undelivered item at Σ27, and it compounds across every future command), then 20 (`LNGHZN-S8`, best leverage-to-cost ratio left — cost score 5, and it exposes validation `arm review record` already performs), then 15 (`NXTTN-S3`, ratchet-shaped: budgets get harder to adopt as agent-facing surface grows), then 19 (`LNGHZN-S7`, smallest blast radius and fully scoped — but note its `doctor` config-health check in T2 should speak item 16's error contract, so 16 first avoids a rewrite), then 14a, then 18 (`NXTTN-S4`, adopter-facing payoff with zero adopters), then 21 (`TOPTIER-S13`, gated on multi-reviewer dispatch running at volume).

### Tier B — solid, sequence after foundation

| # | Item | Source | Armature story |
|---|---|---|---|
| 22 | Ops-branch backup and disaster recovery | Addendum G2 | `TOPTIER-S12` |
| 23 | Authorship / copyright clarity for agent-authored commits | Addendum G6 | `TOPTIER-S16` |
| 24 | One merged-promotion path | LH D4 | not yet decomposed |
| 25 | Redesign transition hooks | LH D2 | not yet decomposed |
| 26 | Event stream (`arm events --follow`) | LH F3 | not yet decomposed |
| 27 | Tiered Quality Gates | Next-Ten №07 | not yet decomposed |
| 28 | The Harness Compatibility Contract | Next-Ten №08 | not yet decomposed |
| 29 | Model-tier dispatch policy | LH C8 | not yet decomposed |
| 30 | README quickstart rewrite | GAP D1 | `TOPTIER-S7` |
| 31 | Shim-retirement policy | LH D3 | not yet decomposed |
| 32 | The Second Substrate (foreign-repo dogfood) | Next-Ten №06 | not yet decomposed |
| 33 | Session handoff bundle | LH C10 | not yet decomposed |
| 34 | Distribution and compatibility maturity | GAP T5 | `TOPTIER-S6` |
| 35 | Adopter positioning | GAP D3 | `TOPTIER-S8` |
| 36 | Cost / token spend observability | Addendum G1 | `TOPTIER-S11` |

Note: G2 and G6 are placed early in Tier B per the addendum's own "cheap, should not wait" / "cheap, low urgency" framing; G1 is placed late per its own "high leverage once fleet volume grows" framing — its value is real but gated on a precondition (a real multi-worker fleet running at volume) that most of the rest of Tier B is not waiting on.

Note on Tier B / Tier A interleaving in the ready queue (added 2026-08-11): several Tier B tasks — notably `TOPTIER-S6-T2` (ops-schema versioning) and `TOPTIER-S7-T1` (README quickstart) — currently sit in `arm ready` alongside the open Tier A items with nothing in the DAG marking them as later. This is the expected consequence of the premise stated at the top of this document: `blocked_by` cannot express cross-document tier ordering, so **the ready queue is deliberately not tier-ordered and must not be read as a dispatch order**. A coordinator picking work off `arm ready` without consulting this table will pull Tier B ahead of Tier A. This is a known limitation, not a DAG defect; do not attempt to fix it by inventing `blocked_by` edges between tiers, which would be the false precision this document exists to avoid.

### Tier C — valuable, lower immediate leverage

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
| 47 | Human-newcomer onboarding diagnostics | Addendum G5 | `TOPTIER-S15` |

Note: G4 and G5 are placed at the tail of Tier C consistent with the addendum's own "low urgency until external adopters" framing for both — the same zero-adopters timing argument the prior three rounds already applied to several other deferred items in this tier.

---

## F2 grilling session — resolved decisions (2026-07-08)

A grilling pass on LH F2 (`arm ready --waves`) resolved the following, ahead of decomposition. These should carry into F2's eventual ADR (per the constitution's ADR-template field) and into `LNGHZN-S2`'s definition of done:

1. **`blocked_by` adds nothing to the partition rule.** Every entry in the ready queue already has all blockers merged (`ComputeReady`'s `allBlockersMerged` gate), so two ready entries can never have a `blocked_by` ordering relationship between them. F2's actual partition criterion is scope-disjointness alone.
2. **Reuse `claimPkg.ScopesOverlap` (`internal/claim/overlap.go`), not `validate.go`'s `scopeIntersection`.** The two disagree today (glob-aware vs. exact-string-only); using the weaker one would let a wave pass partitioning that then fails at actual claim time.
3. **F2's originally-proposed claim-time "companion check" is already shipped.** `cmd/armature/claim.go` already blocks/warns on scope overlap against actively-claimed issues (`ScopesOverlap`, same-worker auto-dismiss, cross-worker `--force`). F2 is net-new only for the `--waves` partition/output itself.
4. **`--waves` is advisory and pre-dispatch only — not the authoritative source for the coordinator's wave manifest.** The existing wave-verification-gate PRD's `WAVE_TASK_IDS`/`WAVE_BASE_SHA` are inherently post-dispatch runtime facts (real SHAs only exist once workers commit); `--waves` runs before any claim exists and can't produce them. The coordinator skill's existing prose-recorded manifest is unchanged by F2.
5. **Explicit non-goal (ADR-bound): scope-disjoint ≠ contract-safe.** The `2026-07-06` cross-task-format-drift finding showed scope-disjoint tasks in the same wave produced an interface mismatch (log format, entry IDs) invisible to per-task review. `--waves` guarantees freedom from file-level conflict only, never from shared-contract drift — this must be stated as an explicit non-goal in F2's ADR, not left implicit.
6. **Parent/child scope containment is a real, reachable prerequisite bug, not hypothetical.** `story` is ready-eligible (`internal/issuetype/issuetype.go`), so a story and its own ready child can co-occur in the ready set; naive `ScopesOverlap` would flag them as conflicting. Fixed as `TOPTIER-S17`'s leading task (see item 8 above), which `LNGHZN-S2` blocks on.
7. **The broader `scope-overlap-validation-gaps` theme is filed as its own story (`TOPTIER-S17`), not folded into F2.** Of its four documented defects — (a) no transitive closure over `blocked_by`, (b) parent/child containment, (c) no cross-story overlap awareness, (d) phantom-scope for blocker-created files — only (b) blocks F2. (a) is irrelevant to wave-partitioning (ready entries can't be mutually blocked_by-ordered). (c) is arguably *fixed for free* by F2, since partitioning runs across the whole ready set regardless of story boundary, unlike today's checkers which the theme says only compare within a story. (d) is orthogonal (filesystem-state validation, not pairwise overlap). Defects (a), (c), and (d) remain `TOPTIER-S17`'s scope but are **not** blockers for `LNGHZN-S2` — they should not be dropped or forgotten just because they don't gate F2; the theme documents this as "the single most frequently reported planning-time friction," recurring across at least 4 distinct stories (DF-S5, MIGH, HOOKBIND, EXECEV, ARCHIMP-S18).
8. **Partition algorithm: greedy first-fit, with priority as a hard tier boundary and conflict-degree ordering within a tier.** Critical-tier items are never delayed into a later wave to improve packing for lower-priority items (and vice versa). Within a priority tier, the existing depth/blocks-count/ID tie-break (built for single-item dispatch order) is dropped in favor of packing by scope-conflict degree (most-constrained items placed first), to actually optimize the stated goal — maximum parallelism, minimum potential merge conflicts — rather than inheriting an ordering built for a different purpose. No wave-size cap.
9. **Output shape: a new top-level grouped structure (array of waves, each an array of `ReadyEntry`), scoped to the `--waves` flag only.** Plain `arm ready` (no `--waves`) keeps today's flat-list shape unchanged. Since alpha software (pre-`v0.1.0`, no external adopters) removes backward-compat as a constraint, the shape was chosen for what's actually easiest for an agent coordinator to consume (dispatch-by-wave without a client-side groupby), not for minimizing diff against the existing flat-list contract.
10. **`--waves` is a pure computed view — no new op type, nothing persisted.** Consistent with I1 (git-native) and I2 (append-only): wave assignment is advisory and can change between calls as claims/priorities shift; it is never a system-remembered commitment. This is also why `--waves` cannot be the source of truth for the coordinator's `WAVE_TASK_IDS`/`WAVE_BASE_SHA` manifest (see item 4) — that manifest is inherently stateful/recorded, which `--waves` by design is not.

---

## Notes on reading this table

- **Ordinal, not cardinal**, following Round Three's own colophon convention: a Tier S item is not "twice as valuable" as a Tier B item, it is *earlier in line*.
- **Self-scored, single-author bias inherited from all three source documents** — this ranking synthesizes their own scores plus judgment; it does not re-run their adversarial rounds.
- **Ties within a tier are not meaningfully ordered** — items 10–21 in Tier A, for instance, are close enough in leverage/evidence/cost that the numbered order should not be read as a strict build sequence within the tier. This applies doubly to the six former-addendum items (G1–G6, now items 22–23, 36, 46–47, and 21): their position within a tier reflects the addendum's own qualitative call ("cheap, should not wait," "A/B boundary," etc.), not a re-scored ranking against their tier-mates.
- When any of these items is actually decomposed into Armature issues (as all six narrow gaps and thirteen other items now are), that DAG becomes authoritative for *that item's* internal sequencing — this document only orders *between* items and documents, which Armature has no mechanism for today.
- **The "Armature story" column** is this document's link into the DAG: it names the epic-level story an item has been decomposed into, where one exists, so the ranked order above can drive cross-story dispatch priority even though `blocked_by` edges can't express it. As of this writing every GAP item (T1–T5, D1–D5) has a 1:1 `TOPTIER-S1`–`S10` story (in that same order — `S1`=T1, `S2`=D2, `S3`=T2, `S4`=T3, `S5`=T4, `S6`=T5, `S7`=D1, `S8`=D3, `S9`=D4, `S10`=D5); the six narrow-gaps addendum items (G1–G6) are `TOPTIER-S11`–`S16` respectively; the `scope-overlap-validation-gaps` dogfood theme is `TOPTIER-S17` (filed and decomposed 2026-07-08), independently of the three planning rounds (same category as G1–G6 — a real gap discovered outside the original documents) during F2's grilling session, as a hard prerequisite for `LNGHZN-S2`; three Next-Ten items (№01, №02, №05) are decomposed under the `NXTTN` epic (story# = item#, except №01: `NXTTN-S1` was a cancelled duplicate, and the one remaining genuine piece of that item's work is tracked as `NXTTN-S1B`); LH D5 and LH F2 are decomposed under the `LNGHZN` epic (`LNGHZN-S1` for D5, blocked by precursor bug `LNGHZN-B1`; `LNGHZN-S2` for F2, decomposed and filed 2026-07-08, blocked by `TOPTIER-S17-T1` specifically) — the epic is the standing home for future LH items, same pattern as `NXTTN`/`TOPTIER`. On 2026-07-19, with Tier S delivered, the eight remaining Tier A items were decomposed in ordinal order: LH C1/C4/F1/C3/D1/C6 as `LNGHZN-S3`–`S8` respectively, and Next-Ten №03/№04 as `NXTTN-S3`/`NXTTN-S4` (story# = item#, matching the existing NXTTN convention). Cross-story same-file scope overlaps against still-open `TOPTIER` tasks (harness-hook docs/evaluator vs `TOPTIER-S5`, Makefile vs `TOPTIER-S3-T1`/`TOPTIER-S6-T1`, workflow.md vs `TOPTIER-S4-T1`) were serialized with `blocked_by` edges following this document's ordinal order. On 2026-08-11 a status audit added the Status column to Tier A and recorded `LNGHZN-S9` as item 14a — a story filed from `LNGHZN-S5`'s PR-review thread rather than from any of the four planning documents, and therefore the first entry in this table whose source is the DAG itself (LH F1.1 is a back-reference added to `long-horizon-proposals.md` to give it a citable source ID, not an item from the original round). Update this column, not just the source doc, whenever an item crosses from "proposed" to "filed" **or from "filed" to "delivered"** — it is the only place this ordering and the DAG's IDs are cross-referenced, and an un-updated Status column makes the whole tier read as pending.

- **Work filed outside the four planning documents needs a home here too.** `LNGHZN-S9` sat in the DAG untiered and uncited between its filing and the 2026-08-11 audit, which made it invisible to this ordering — it appeared in `arm ready` with no tier and no priority argument attached to it. Any story filed from a PR review, a dogfood theme, or a grilling session (as `TOPTIER-S17` was) should get a row here at filing time, using a sub-ordinal (`14a`) when it is a follow-on to an existing item, so the numbering cited from the source documents stays stable.
