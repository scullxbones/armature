---
date: 2026-09-01
agent: claude
area: workflow
task: Promote stale done-under-merged tasks across the whole DAG
tags: [status, lifecycle, i6, merged, cancelled, gate-task, taxonomy]
---

# Three kinds of completed work have no honest terminal status

## User Goal

Assign a correct terminal status to every issue stuck at `done`, where `merged`
asserts "confirmed on `origin/main`" (I6) and `cancelled` asserts "abandoned".

## Observed

Three items resisted both statuses, each for a different reason. All three are
legitimate outcomes of real work, and none of them is expressible.

**1 — Shipped, then deliberately removed.** `ORCRUN-T01..T04` and `E7-S1-T15`
delivered `internal/orchestrate/` and `internal/workerruntime/`. Every scope file
is present in `origin/main` history. The packages were later deleted outright by
`45d05dfe feat(ORCRMV-T04): delete internal/orchestrate and internal/workerruntime
packages` — a tracked task, doing exactly what it was asked to do. `merged` is
true but reads as "this is in the product". `cancelled` is false: the work was
built, reviewed and landed. The parent `ORCRUN` epic is `cancelled` while its
descendants are legitimately `merged`, which is a shape no check currently
anticipates.

**2 — Gate tasks with no artifact.** `E7-S1-T15`'s definition of done is:

> make check fully green (lint + test + coverage ≥80% + mutation); arm validate
> reports no errors; arm doctor --strict reports no errors; all orchestration op
> types present in ValidOpTypes

Its deliverable is a *green gate*, not a diff. It can never produce a commit, so
every commit-based criterion returns "no evidence" for it — permanently, and
correctly. `ARM-S4-T4` is the same shape ("Verification only: run make check…")
and happened to get a commit anyway, so an identical pair of tasks received
opposite treatment purely by commit-message luck.

**3 — Tasks that turn out to be no-ops.** `RP-T4`'s scope was `.gitignore`. Its
own outcome records: *"dist/ already present in .gitignore (as /dist/ at line 3);
goreleaser snapshot build deferred to CI — goreleaser not installed locally."*
It changed nothing and deferred both verification clauses of its own DoD, yet was
transitioned to `done`. Neither `merged` (asserts a DoD that was not met) nor
`cancelled` (implies it was dropped, when it was worked and found empty) is
accurate.

## Impact

Each gap pushes a false statement into an append-only log. Under I2 the wrong
choice cannot be retracted, only annotated by a later op — so the cost of an
inexpressible state is permanent, and it compounds every time an auditor has to
re-derive from notes what a status should have said.

The `done` ≠ `merged` distinction (I6) exists precisely because self-reported
completion and confirmed-on-main are different claims. These three cases show the
same argument extends further: *confirmed-on-main*, *confirmed-then-withdrawn*,
*confirmed-by-gate*, and *found-empty* are four different claims currently
collapsed into two statuses.

Practically, this also breaks tooling that reasons about status. `RunRollup`
promotes a parent when all children are `merged`; a wholly no-op child forced to
`cancelled` blocks that rollup (see Recurs), so a bookkeeping compromise in one
place silently strands a parent somewhere else.

## Evidence

- `git log origin/main --oneline -- internal/orchestrate` → 25 commits, ending at
  `45d05dfe feat(ORCRMV-T04): delete internal/orchestrate and internal/workerruntime packages`
- `ORCRUN` epic status `cancelled`; `ORCRUN-S1` and `ORCRUN-T01..T04` merged beneath it
- `arm show --issue E7-S1-T15` → DoD names only gates; siblings `E7-S1-T1..T14`
  each have an ID-tagged commit, T15 has none
- `arm show --issue ARM-S4-T4` → scope reads "Verification only: run make check |
  arm --help | arm ready --format agent | arm version"; promoted on commit `0c6c12ba`
- `arm show --issue RP-T4` → outcome states `/dist/` pre-existed and the goreleaser
  verification was deferred; `git show origin/main:.gitignore` confirms `/dist/` at line 3
- Recurs: [`arm sync` skips every done issue because `branch` is never recorded](2026-08-31T1142Z-claude-workflow-sync-skips-every-issue-branch-never-recorded.md)

## Suggested Follow-Up

Do not rush to add statuses; each one costs every consumer a branch. The cheaper
first move is to let a terminal status carry a *reason*, the way `cancelled`
already implies one, and to have `arm transition` record it structurally rather
than in prose notes. `merged (superseded-by: ORCRMV-T04)` and
`cancelled (reason: no-op)` would have expressed all four cases above without a
new state in the machine.

If a status is added, `superseded` is the one that earns its place: it is the only
case here where the work genuinely reached main and genuinely is not there now,
and it is the only one a reader will otherwise get wrong in both directions.

Separately, the planner should stop emitting artifact-free gate tasks as `task`
type. A gate is a property of a story's completion, not a unit of work with its
own scope — `E7-S1-T15` listed four scope files it never touched.
