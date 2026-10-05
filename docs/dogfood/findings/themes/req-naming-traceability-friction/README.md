---
curated_by: cursor
date: 2026-10-05
---

# Theme: `_REQ_` Traceability Naming Is Undocumented and Conflicts

## Summary

Acceptance criteria require exact test names like `TestFoo_REQ_<TASK-ID>`. That convention is not in the worker skill, planner skill, `AGENTS.md`, or `CONTEXT.md` at the time of the first live `arm review` dogfood. Superpowers plans prescribe a different suffix (`_P1` / `_P2`). Workers write thorough tests under invented names, `go test` is green, and review is red. Quality-control C9 is still a GAP; this theme is the dogfood of that interim naming path.

## Evidence

- [`_REQ_<TASK-ID>` test naming convention undocumented`](../../raw/2026-06-29T1900Z-5207ee28-documentation-req-suffix-naming-convention-undocumented.md) — SMTC-S1-T1 rated red for names like `TestDeriveRating_AllSatisfied_Green` instead of `TestDeriveRating_REQ_SMTC-S1-T1`. Also listed under [documentation-gaps](../documentation-gaps/README.md).
- [`_REQ_` test-naming convention conflicts with superpowers plan test names`](../../raw/2026-07-02T0000Z-claude-workflow-req-naming-conflicts-with-superpowers-plans.md) — Two authoritative naming rules, neither says which wins. Also listed under [documentation-gaps](../documentation-gaps/README.md).
- [`Haiku workers deviated from contract-required exact test names`](../../raw/2026-07-24T1104Z-claude-workflow-contract-test-naming-mismatch.md) — Substantial coverage under invented names; T2 claimed pass-through logging that only recorded on the block path. Also listed under [unreliable-worker-self-report](../unreliable-worker-self-report/README.md).
- [`A story made an acceptance rule normative before its evidence mechanism existed`](../../raw/2026-08-14T2338Z-5207ee28-coordination-normative-rule-lands-before-its-evidence-mechanism.md) — Traceability/evidence rules graded against a mechanism T3 had not shipped. Also listed under [documentation-gaps](../documentation-gaps/README.md).

## Candidate Follow-Ups

- Document `_REQ_<TASK-ID>` in `docs/conventions.md` and the worker/planner skills; state which convention wins when a source plan names tests.
- Done-time check for contracted names (C9 interim), not only reviewer prose.
- Do not make an evidence rule MUST in an earlier task than the task that implements the evidence command.
