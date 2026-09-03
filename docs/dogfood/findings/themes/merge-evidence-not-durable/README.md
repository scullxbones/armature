# Theme: `merged` Is Asserted Without Durable, Machine-Checkable Evidence

## Summary

[i6-promotion-agent-owned](../i6-promotion-agent-owned/README.md) covers the
*mechanism* that fails to promote `done` → `merged`. This theme covers what is
left over once someone promotes by hand: **what evidence exists, months later,
that a specific issue's work is on `origin/main`** — and whether the status the
system can record is even true.

Armature records no `branch`, no `pr`, and no delivery SHA on the ops that
matter. So the only durable link between an issue ID and a diff is the
conventional-commit subject `type(<ID>): …`. That link is thin, and three things
break it:

- **Umbrella commits.** A worker committing once per story instead of once per
  task destroys per-task attribution permanently — I2 means it cannot be
  backfilled, only annotated. 22 of 79 stale issues in one audit had no
  ID-tagged commit; every one had genuinely landed. The evidence gap was 100%
  false negatives caused by commit practice.
- **Gate tasks and no-ops.** Some completed work produces no diff by design (a
  task whose DoD is "`make check` green"), and some produces none by discovery
  (a task that finds its change already made). Commit-based evidence returns
  "no evidence" for these permanently and correctly.
- **Work that landed and was then removed.** `merged` is true and reads as
  "this is in the product"; `cancelled` is false. Neither is honest, and under
  I2 the wrong choice is permanent.

The per-task commit convention is usually justified as reviewability. Its
load-bearing purpose is narrower: it is the only machine-checkable proof that a
specific task reached main. That makes `docs/conventions.md`'s per-task rule an
evidence requirement, not a style preference.

## Evidence

- [Story-level umbrella commits erase the only per-task evidence that `merged` is true](../../raw/2026-09-01T1156Z-claude-workflow-story-level-commits-erase-task-evidence.md) — `ORCRUN-T01..T04` delivered by one `feat(ORCRUN-S1)` commit; `run.go (new)` in T01's scope, no per-task attribution recoverable. `ARM-S2-T2+` shows an agent inventing a `+` suffix to say "and its siblings" for want of a sanctioned way to batch.
- [Three kinds of completed work have no honest terminal status](../../raw/2026-09-01T1156Z-claude-workflow-no-status-for-shipped-then-removed-or-gate-tasks.md) — shipped-then-deleted (`ORCRUN-T01..T04`, removed by `ORCRMV-T04`), artifact-free gate tasks (`E7-S1-T15`), and worked-but-empty no-ops (`RP-T4`). Four distinct claims collapsed into two statuses.
- [Stable whole-DAG merged-status audit finds no remaining provably false record](../../raw/2026-08-23T1930Z-codex-validation-stable-merged-status-audit.md) — 562 confirmed / 0 false / 7 `NOT_DETERMINABLE` over 569 records. The seven are all transient verification claims (`make check` passed, doctor clean) with no retained attestation. Also the negative result worth keeping: current-file mismatch is a *poor* proxy for false merge — 26 alarming mismatches were historical deliveries later removed or renamed.

The live counter-example that motivated the audit — a story recorded `merged`
for code never on `main` — is curated under
[unknown-recorded-as-answered](../unknown-recorded-as-answered/README.md).

## Candidate Follow-Ups

- **Record the delivery SHA on the `transition --to done` op.** The worker knows
  it at that moment. One recorded SHA turns every question in the audit above
  from an investigation into a lookup, and it does not depend on commit-message
  discipline holding.
- **Warn at `--to done` when no commit reachable from `HEAD` names the issue.**
  That is the last moment the evidence still exists and the fix is one
  `git commit --amend`.
- **Let a terminal status carry a structured reason** rather than adding
  statuses: `merged (superseded-by: ORCRMV-T04)`, `cancelled (reason: no-op)`.
  If one status is added, `superseded` is the one that earns its place — it is
  the only case a reader otherwise gets wrong in both directions.
- **Persist deterministic gate attestations** keyed by invoking checkout, commit,
  command and result. Until then a verification-only `merged` record can honestly
  be no better than `NOT_DETERMINABLE` at audit time.
- **Stop emitting artifact-free gate tasks as `task` type.** A gate is a property
  of a story's completion, not a unit of work with its own scope — `E7-S1-T15`
  listed four scope files it never touched.
