# The Strategy Memo

**Date:** 2026-10-24  
**Source:** `docs/design/the-next-ten.html`, item №09, "The Strategy Memo" (Σ 23); authorship row from `narrow-gaps-addendum.md` G6.2 / `TOPTIER-S16-T2`  
**Status:** Active internal decision memo. Review date below.  
**Sequencing:** Next-Ten №09 / sequencing item 39. G6.1 attribution text lives in [`AUTHORSHIP.md`](../../AUTHORSHIP.md).

## Thesis

Three stacked roadmaps existed and none named who Armature is for first. This memo fixes that: one wedge, one wager, one sequencing rule, a pre-mortem with tripwires, metrics that can fail, and a content play. Future planning rounds cite the wedge or argue with it.

## The wedge

**First persona: the solo operator running an agent fleet on one real repository.**

In architecture vocabulary (`docs/design/architecture.md` §21): **solo freelance** and **solo enterprise** — one accountable human, many agent workers, protected or unprotected `main`, no multi-human claim races. Team monorepo and team multi-repo stay design targets; they are not the wedge.

Planning aliases "Wrangler" and "Conductor" are retired for this decision. Prefer the four architecture personas when writing new docs. Feeding that vocabulary back into `CONTEXT.md` is follow-on hygiene, not a blocker for this memo.

## The wager

Armature matters if and only if:

1. A solo operator can run multiple agents against one git repo without inventing a second coordination layer.
2. Deterministic gates decide merge-worthiness; LLM review advises and does not own facts.
3. Depth-first dogfood on this repository produces evidence stronger than a positioning deck.

If any of those fails in practice, the product is a skill pack with a CLI costume — stop expanding surface and cut.

## Sequencing

1. **Depth-first dogfood** on `scullxbones/armature` (and the operator's own repos) until the wedge persona can complete a story wave without heroics.
2. **A few design partners** who already match the wedge — not broad launch, not enterprise RFPs.
3. **Public** only after partners can install from a release, follow CONTRIBUTING/AUTHORSHIP, and file issues without the maintainer on the call.

`docs/design/next-work-sequencing.md` ranks the remaining backlog inside that order. Held Tier C (ops backup G2 / `TOPTIER-S12`, extensibility G4) stays held unless the wedge forces a lift.

## Pre-mortem

| Risk | Why it kills us | Tripwire | Hedge / owner |
|---|---|---|---|
| Harness API churn | Skills and hooks rot every vendor release; workers stop mid-wave | A harness upgrade breaks `arm` hooks or skill front matter on a supported tool within one release cycle | Next-Ten №08 Harness Compatibility Contract; keep adapter surface thin |
| Model-behavior drift | Same skill, new model, silent DoD miss | Two consecutive dogfood waves need prompt/skill rewrites solely because the model changed | Prefer deterministic gates; golden transcripts when GAP·T1.2 returns; do not build an eval platform for one adopter |
| Bus factor | One maintainer; ops + product knowledge in one head | Maintainer unavailable ≥14 days with no written recovery path for claim/ops restore | Document recovery; keep doctor checks green; G2 backup remains the hold item until needed |
| Context-price collapse | Cheap context removes the budget pressure that justifies Armature's design | Dominant models ship context so large/cheap that "minimize tokens" stops being a buying reason for the wedge persona | Re-validate the wager; lean on determinism and git-native coordination, not token scarcity alone |
| Category consolidation | Agent frameworks absorb "issue DAG + git worktrees" as a checkbox | A mainstream harness ships built-in multi-agent git orchestration that design partners prefer to Armature | Stay git-native and gate-hard; refuse to become a chat bus or merge authority |
| Authorship / copyright of agent-authored commits | Legal attribution ambiguity blocks OSS adoption and partner legal review — distinct from ops-log writer trust | Ship, market, or onboard a design partner without a published attribution + license statement for agent-authored commits, or merge agent-produced code without following it | [`AUTHORSHIP.md`](../../AUTHORSHIP.md) (G6.1): `Co-authored-by` trailers + Apache-2.0 inbound=outbound. This row is G6.2 |

The killed "trust & authorship model for multi-writer ops" item stays killed: zero adversarial writers. This authorship row is **legal** attribution of repository content, not ops-log trust.

## Metrics and review date

**Review date:** 2027-01-24 (90 days from this memo).

Metrics that can fail (any one red → revisit wedge or sequencing before adding Tier C surface):

| Metric | Green | Red |
|---|---|---|
| Wedge dogfood | Maintainer completes ≥1 multi-worker story wave on this repo without bypassing delivery gate | Delivery gate skipped as habit, or waves only succeed with maintainer live-debugging every claim |
| Design partners | ≥1 external wedge-shaped user past `arm bootstrap` with a filed issue or PR | Zero external installs after the review date with no written reason |
| Authorship hygiene | Agent-authored merges carry `Co-authored-by` per AUTHORSHIP.md; no partner blocked on license/attribution | Partner or release blocked on missing attribution/license clarity |
| Sequencing honesty | Living sequencing page matches `arm show` tip; held items stay held | `arm ready` treated as dispatch order for held S12 / Tier C leaves |

## Content play

The dogfood corpus (`docs/dogfood/`) is the most credible marketing this project owns. Spend it after the wedge is stable: publish findings as evidence of the wager, not as a feature list. Adopter-facing positioning remains GAP·D3 / shipped S8 surfaces; this memo is the internal decision those surfaces assume.

## Not that

- Not an adopter README (that is S7 / S15 / CONTRIBUTING).
- Not the Armature Constitution (№01) — invariants stay in `CONSTITUTION.md`.
- Not a re-opening of multi-writer ops trust (constitution tripwire / killed Next-Ten item).

## Plan produces (checklist)

- [x] Wedge decision
- [x] Wager statement
- [x] Sequencing
- [x] Pre-mortem table (including authorship/copyright + tripwire)
- [x] Metrics and a review date
- [ ] Unified persona vocabulary fed back into `CONTEXT.md` (follow-on; architecture §21 remains canonical)
