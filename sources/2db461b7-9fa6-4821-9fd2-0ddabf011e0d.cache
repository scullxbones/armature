# The Subtractive Release — Census Plan

**Date:** 2026-07-07
**Source:** `docs/design/the-next-ten.html`, item №02, "The Subtractive Release" (Σ 27)
**Status:** Resolved via `/grill-with-docs`; ready for `/to-issues`.
**Sequencing:** Tier S, position 4 in `docs/design/next-work-sequencing.md` — after doc-corpus hygiene (GAP·D5), the Armature Constitution (Next-Ten №01), and skill lint / golden-transcript tests (GAP·T1). Deadline: before `v0.1.0` (GAP·T5.3).

## Thesis

The cheapest deletion window this project will ever have is open now, and it closes at `v0.1.0`. Every user-facing surface — issue type, status, confidence state, field, command, flag — should show dogfood-corpus evidence of use, or carry a written justification, or be cut before the first real release.

## Scope: what counts as a surface

This census governs the full **surface** space (`CONTEXT.md`): issue types, statuses, confidence states, fields, commands, flags. Explicitly **excluded**:

- Config knobs — already audited by LH·D1 (the completed pilot this census generalizes).
- Skills — governed by Context Economics (Next-Ten №03)'s token-budget work, not this census.
- Ops-log event/op types — internal materialization plumbing, not user-facing.

If a category surfaces mid-audit that fits none of the six buckets, note it but do not rule on it — out of scope for this pass.

## Evidence and ruling

A surface's **ruling** (kept-evidence / kept-justified / parked) is made by a single accountable person and recorded permanently as a row in the **census** — a markdown table living under `docs/`, following the `docs/dogfood/findings/` precedent as a durable, human-and-agent-readable artifact. No per-surface ADR; the census row is the record.

Two independently sufficient evidence types:

1. **Corpus evidence** — the surface appears in the dogfood ops-log corpus (currently 6,918 ops across 71 worker logs, queryable via grep/jq over the ops-branch worker logs; the query recipe is written as an appendix to the census doc so a future sweep is reproducible without reverse-engineering methodology).
2. **Written justification** — a short prose reason, independent of corpus evidence. A justification can drive a *keep* (not yet dogfooded but intentional) or a *cut* (dogfooded but redundant/confusing) even where corpus data says otherwise.

Corpus absence alone is sufficient grounds for a cut; it is not the only path either direction.

## Park, not purge

A cut is **parked**: its code is deleted outright, but its census row, re-entry criterion, and the removing commit persist as an intentional, documented, in-principle-reversible record. See ADR 0010 for why literal deletion (not a soft-deprecation shim or a feature-flagged dormancy) is the mechanism.

**Purge** — no census row, no re-entry criterion, no documented path back — is a near-unused category reserved for code with no product surface at all (dead code, not a cut feature). This census does not expect to produce any purges.

Each parked row carries a **re-entry criterion**: a written, standing condition (not a promise of future work) that would justify **resuscitation** — re-implementing the surface once the criterion is met.

## The `behavior` finding

Evidence-gathering for this plan found `behavior` as an issue `Type` value with 8 occurrences in the corpus and no definition anywhere in code or `CONTEXT.md`. Investigation during grilling found `Type` (`internal/materialize/state.go:15`) is an **unvalidated free-text string** — no enum constraint exists. `behavior` is not a deliberate type; it is very likely a stray value written by a codex agent that the system silently accepted.

Ruling: **not a new domain concept.** Add enum validation on `Type` (`epic`/`story`/`feature`/`task`/`bug`), migrate the 8 existing issues by hand (small enough not to need tooling), and do **not** add `behavior` to `CONTEXT.md`. This lands as its own small PR — a bug fix, not a subtraction — so it isn't buried in the census PRs.

## Enforcement: keeping the census honest after v0.1.0

The census's own standing rule — "every new verb starts with a census row" — is enforced in CI, not left as unbound prose (the failure mode Next-Ten №04, The Paved Road, calls out directly). A CI check enumerates the structurally-enumerable current surface (commands, issue types, statuses, flags) and diffs it against the census's surface list; CI fails if code has drifted ahead of the census. This follows the same pattern as GAP·T1.1's skill lint.

## Delivery: PR batching

The cuts land as a **sequenced series**, batched by category rather than by count:

1. **Census doc** — the table itself (surface × evidence × ruling), standalone, reviewable before any code changes.
2. **Schema-adjacent cuts** — issue-type / status / confidence-state parks, batched together since they likely touch shared validation code.
3. **CLI-adjacent cuts** — command / flag parks.
4. **`behavior` type-validation fix** — its own PR (bug fix, not a subtraction).

## Glossary additions

Resolved and written to `CONTEXT.md` during grilling: `Surface`, `Census`, `Ruling`, `Park`, `Purge`, `Re-entry Criterion`, `Resuscitation`.
