---
date: 2026-08-23
agent: claude
area: validation
task: Deep branch audit during LNGHZN-S10/S9 closeout
tags: [i6, merged, integrity, validate, rollup, silent-failure]
---

# The DAG records `merged` for work that has never been on main

## User Goal

Establish, by content, which local branches still held work not present on `origin/main`,
so the rest could be safely deleted.

## Observed

`ARCHIMP-S15` ("Phase 2: Claim resolver") is recorded in Armature with status `merged` and a
specific, confident outcome:

> "claim.PlanClaim(ResolveInput) pure function with OverlapDecision consts
> (None/DismissSameWorker/Block/Force) added to internal/claim/resolver.go; claim handler
> migrated off inline overlap loop to call PlanClaim; 4 table-driven tests cover all decision
> branches; make check green, mutation efficacy 100%."

None of it is on main:

- `internal/claim/` on `origin/main` holds only `claim.go`, `overlap.go` and their tests.
  There is no `resolver.go`, and no commit has ever touched that path on main.
- `git log --oneline origin/main --grep='ARCHIMP-S15'` → **0 commits**.
- `git log -S'PlanClaim' origin/main` → matches only a dogfood finding and a plan document.
  `PlanClaim` has never existed in code on main.

The only surviving copies are three local branches (`feat/ARCHIMP-S15` @ 56102747,
`task/ARCHIMP-S15-T1` @ 7abfc5e2, `task/ARCHIMP-S15-T2` @ 1504dc9d) — which the branch
cleanup running at the time would have deleted had the audit been done by ancestry rather
than by content.

`arm validate` reports clean (730/730 cited). Nothing in the system flags the discrepancy.

## Impact

This is an **I6** integrity failure in the data itself: `done` ≠ `merged` is the invariant,
and here `merged` is asserted for work never confirmed on main. It is worse than the two
promotion failures found earlier the same session, which were failures to *detect* landed
work. This is the inverse — the system affirmatively recorded work as landed that was not —
and it is equally silent.

Consequences observed:
- The outcome text is detailed and confident enough that a reader would not think to check it.
  Specificity read as evidence.
- A DAG-driven cleanup would have destroyed the only copy of the work, using the DAG's own
  (false) claim as justification.
- No deterministic check surfaces it. `arm validate` is green, so a coordinator, an auditor,
  and `arm doctor` all pass over it in silence.

Third silent failure of the same session, alongside `arm sync` failing to see squash-merged
work and conformance evidence living only in GitHub. All three share a shape: the system
reports success or silence where the truth is unknown to it.

## Evidence

- `arm show ARCHIMP-S15` — status `merged`, outcome quoted above
- `git ls-tree origin/main internal/claim/ --name-only`
- `git log --oneline origin/main --grep='ARCHIMP-S15'` → empty
- `git log --oneline -S'PlanClaim' origin/main` → docs only
- `git log --oneline origin/main -- internal/claim/resolver.go` → empty
- `arm validate` → `OK: no issues found (COVERAGE: 730/730 cited)`
- `internal/materialize/engine.go:589` — `RunRollup` promotes a story once all children are
  `merged`, which is one candidate path for how this was recorded

## Suggested Follow-Up

Two separable questions. (1) How did the status become `merged` — an explicit op, an
auto-rollup over children themselves wrongly marked, or a manual transition? (2) Should a
deterministic check exist that a `merged` issue's claimed delivery is actually present on the
target branch? Such a check is content-based, needs no LLM judgment, and would be I5-clean.
Absent it, `merged` is an unverified self-report wearing the name of a confirmed state.
