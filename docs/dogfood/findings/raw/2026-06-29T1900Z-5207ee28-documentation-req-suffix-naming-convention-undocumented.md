---
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
area: documentation
date: 2026-06-29T19:00Z
---

# `_REQ_<TASK-ID>` test naming convention in acceptance criteria is undocumented

## What the agent was trying to do

Running `arm review prepare` + reviewer agent on SMTC-S1-T1 as a first live
dogfood of the semantic conformance review protocol.

## What happened

The reviewer agent rated SMTC-S1-T1 **red** on two acceptance criteria:

- `acceptance[0]`: "TestDeriveRating_REQ_SMTC-S1-T1 passes"
- `acceptance[1]`: "TestProtocolJSONUsesStableNames_REQ_SMTC-S1-T1 passes"

The actual delivery contains correct, thorough tests:

- `TestDeriveRating_AllSatisfied_Green`, `TestDeriveRating_NotSatisfied_Red`,
  `TestDeriveRating_Mixed_YellowAndRed_Red`, etc. (7 cases covering the full
  algebra)
- `TestCriterionStatus_JSONRoundTrip`, `TestRating_JSONRoundTrip` (covering
  stable JSON serialization)

The reviewer correctly observed: `go test -run TestDeriveRating_REQ_SMTC-S1-T1`
reports "no tests to run." So the literal acceptance criterion is not met, even
though the semantic intent is satisfied.

## Why it matters

The `_REQ_<TASK-ID>` suffix is a traceability convention: it ties test functions
to the task that required them. But this convention is documented nowhere:

- Not in the worker skill
- Not in the planner skill  
- Not in the armature-reviewer skill
- Not in CLAUDE.md
- Not in CONTEXT.md

A worker writing perfectly good tests will naturally use descriptive suffixes
(`_AllSatisfied_Green`) rather than traceability suffixes (`_REQ_SMTC-S1-T1`),
and will fail review on a literal name check. The semantic substance of the work
is correct; the traceability label is missing.

## Secondary observation: reviewer behavior is correct

The reviewer did exactly the right thing: it applied the contract literally.
"TestDeriveRating_REQ_SMTC-S1-T1 passes" is an unambiguous acceptance criterion;
the test does not exist by that name; the criterion is not_satisfied. The red
rating is technically accurate. The problem is upstream in how the planner wrote
the criteria.

## Tertiary observation: arm show surfaces review inline

`arm show SMTC-S1-T1` now shows:
```
Review: red (bundle 6f1a6e4bf55a; 2 satisfied, 2 not_satisfied)
```
This is working correctly and is a positive signal for the feature.

## Evidence

- Reviewer assessment: acceptance[0] and acceptance[1] → not_satisfied
- `grep -rn "_REQ_SMTC-S1-T1"` → 0 matches in internal/review/
- `go test ./internal/review -v` → all 20 tests pass (no _REQ_ names)
- Acceptance criteria in SMTC-S1-T1: explicitly require `_REQ_SMTC-S1-T1` suffix

## What would help

1. Document the `_REQ_<TASK-ID>` naming convention in the worker skill and
   the planner skill's "Writing Acceptance Criteria" guidance.
2. Or: stop requiring exact test function names in acceptance criteria. Instead,
   state the behavioral requirement ("DeriveRating algebra is tested for all
   rating outcomes") and let the reviewer assess substance, not names.
3. If the convention is kept, add it to CLAUDE.md and the worker skill so
   workers know to use it when writing tests.
