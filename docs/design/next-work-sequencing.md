# Next-Work Sequencing — A Cross-Document Execution Order

**Date:** 2026-07-07
**Purpose:** A single execution order across all proposals from the three planning rounds — `docs/design/top-tier-gap-analysis.md` (GAP), `docs/design/long-horizon-proposals.md` (LH), `docs/design/the-next-ten.html` (Round Three) — plus the `docs/design/narrow-gaps-addendum.md` items now tracked as `TOPTIER-S11`–`S16`.

**Why this lives in markdown, not in Armature:** Armature's DAG models dependency and scope within a single epic/story tree — `blocked_by` edges, scope overlap, wave dispatch. It has no concept of *cross-document, cross-epic priority ordering* across independent proposals that don't share a scope or a `blocked_by` edge but still have a preferred execution order (e.g., "write the constitution before the census, even though nothing blocks the census on it"). Only the gap-analysis items are currently modeled as `TOPTIER` stories; the long-horizon and Round Three items are not yet decomposed into issues at all, and several of them (documents, policies, an internal memo) are not naturally issue-shaped work in the first place. Forcing this ordering into `blocked_by` edges would either be false precision (most of these items are *not* hard-blocked on each other) or would require inventing an epic spanning three separate planning documents that don't share a DAG today. This document is the citable ordering until — if ever — that changes.

---

## The ranked list (Tier S → C)

Ranking combines each document's own scoring (Round Three's Σ/30, LH's six-axis table for C1–C10) with cross-document dependency, dogfood-corpus evidence weight, and expiry urgency (several items are only cheap before `v0.1.0` ships).

### Tier S — foundational, do first (everything else cites these, or their window closes soon)

| # | Item | Source |
|---|---|---|
| 1 | Doc corpus hygiene / archive (incl. ADR template) | GAP D5 |
| 2 | The Armature Constitution | Next-Ten №01 |
| 3 | Skill lint / golden-transcript tests | GAP T1 |
| 4 | The Subtractive Release (surface census) | Next-Ten №02 |
| 5 | Envelope/schema documentation | GAP D2 |
| 6 | Collapse `.arm/.armature/` dotdir | LH D5 |
| 7 | The CLI Grammar Contract | Next-Ten №05 |
| 8 | Scope-disjoint wave planning (`arm ready --waves`) | LH F2 |

Note: D5 is pulled forward from Tier C because its ADR-hygiene sub-item (D5.2) creates the ADR template the Constitution's "principles touched" field rides on — see Next-Ten №01. D5.1 and D5.3 travel with it since D5 is treated as one coherent unit (doc-corpus hygiene is small enough not to warrant splitting).

### Tier A — high impact, second wave

| # | Item | Source |
|---|---|---|
| 8 | End-to-end workflow test harness | GAP T2 |
| 9 | Crash/recovery resilience | GAP T3 |
| 10 | Autonomic heartbeats via harness hook | LH C1 |
| 11 | Transition-time delivery gate | LH C4 |
| 12 | Managed worktree lifecycle | LH F1 |
| 13 | Context Economics | Next-Ten №03 |
| 14 | Agent-grade error contract | LH C3 |
| 15 | Scope enforcement hardening | GAP T4 |
| 16 | The Paved Road | Next-Ten №04 |
| 17 | Make configuration honest | LH D1 |
| 18 | Reviewer self-validation (`arm review validate`) | LH C6 |

### Tier B — solid, sequence after foundation

| # | Item | Source |
|---|---|---|
| 19 | One merged-promotion path | LH D4 |
| 20 | Redesign transition hooks | LH D2 |
| 21 | Event stream (`arm events --follow`) | LH F3 |
| 22 | Tiered Quality Gates | Next-Ten №07 |
| 23 | The Harness Compatibility Contract | Next-Ten №08 |
| 24 | Model-tier dispatch policy | LH C8 |
| 25 | README quickstart rewrite | GAP D1 |
| 26 | Shim-retirement policy | LH D3 |
| 27 | The Second Substrate (foreign-repo dogfood) | Next-Ten №06 |
| 28 | Session handoff bundle | LH C10 |
| 29 | Distribution and compatibility maturity | GAP T5 |
| 30 | Adopter positioning | GAP D3 |

### Tier C — valuable, lower immediate leverage

| # | Item | Source |
|---|---|---|
| 31 | Findings as a product loop (`arm finding`) | LH C9 |
| 32 | Scope/context suggestion from co-change mining | LH C7 |
| 33 | The Strategy Memo | Next-Ten №09 |
| 34 | Community/contribution scaffolding | GAP D4 |
| 35 | Living Diagrams | Next-Ten №10 |
| 36 | Time-travel state (`--as-of`) | LH F4 |
| 37 | Flow analytics (`arm stats`) | LH F5 |
| 38 | Ops compaction and snapshot checkpoints | LH C2 |
| 39 | Redaction firewall for durable ops | LH C5 |

### Addendum — six narrow gaps, now tracked as issues

These are the only items in this document that *are* modeled in Armature (`TOPTIER-S11`–`S16`, cited against `docs/design/narrow-gaps-addendum.md`). Listed here for completeness of the full ordering; their DAG state is authoritative in `arm show`, not this table.

| Item | Armature story | Rough tier |
|---|---|---|
| Reviewer disagreement/consensus policy | `TOPTIER-S13` | A/B boundary — high evidence, moderate cost |
| Cost/token spend observability | `TOPTIER-S11` | B — high leverage once fleet volume grows |
| Ops-branch backup/DR | `TOPTIER-S12` | B — cheap, should not wait |
| Authorship/copyright clarity | `TOPTIER-S16` | B — cheap, low urgency until external contributors |
| Human-newcomer onboarding diagnostics | `TOPTIER-S15` | C — low urgency until external adopters |
| Extensibility seam for custom issue types | `TOPTIER-S14` | C — low urgency until external adopters |

---

## Notes on reading this table

- **Ordinal, not cardinal**, following Round Three's own colophon convention: a Tier S item is not "twice as valuable" as a Tier B item, it is *earlier in line*.
- **Self-scored, single-author bias inherited from all three source documents** — this ranking synthesizes their own scores plus judgment; it does not re-run their adversarial rounds.
- **Ties within a tier are not meaningfully ordered** — items 8–18 in Tier A, for instance, are close enough in leverage/evidence/cost that the numbered order should not be read as a strict build sequence within the tier.
- When any of these items is actually decomposed into Armature issues (as the six narrow gaps now are), that DAG becomes authoritative for *that item's* internal sequencing — this document only orders *between* items and documents, which Armature has no mechanism for today.
