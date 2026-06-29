# Theme: Test Coverage Gaps — False Green Test Suites

## Summary

Tests pass while critical end-to-end contracts are broken. Two recurring causes: (1) test fixtures are constructed as Go structs and never exercise the JSON serialization path a real user would follow; (2) eval corpus data is duplicated across multiple files and inline fixtures, so tests are self-referentially consistent even when diverged from the canonical source.

## Evidence

- [JSON string/int mismatch between skill docs and Go types hidden by struct-based tests](../../raw/2026-06-28T2200Z-claude-workflow-json-string-int-mismatch-hidden-by-tests.md) — `CriterionStatus`/`Rating` marshaled as integers but skill instructed string values. Every test constructed Go structs — the mismatch was never exercised. The end-to-end reviewer→`arm review record` loop was non-functional with a real reviewer agent while all tests were green. Caught only by opus code review reading the actual usage contract.
- [Eval corpus divergence — four copies, no single source of truth](../../raw/2026-06-29T0001Z-5207ee28-validation-eval-corpus-divergence.md) — A single logical change (case-008 `expected_rating: yellow → red`) required edits in four locations: `cases.json`, `reviewer_eval_results.json`, and two inline `setUp()` fixtures in `test_reviewer_eval_report.py`. Python tests passed throughout because the inline fixtures were internally consistent with each other, not with the canonical source. The divergence was invisible to CI until manually inspected.

## Pattern

Green tests create false confidence in two ways:

1. **Struct-only test construction**: When every test builds domain objects via Go struct literals and never parses a JSON file authored the way a real agent or user would, the serialization contract between skill documentation and Go types is never exercised. Bugs in `MarshalJSON`/`UnmarshalJSON`, or mismatches between documented string values and integer enum representations, survive until end-to-end testing or code review.

2. **Duplicated fixture data**: When test setUp methods maintain inline copies of canonical data (eval cases, expected results), internal consistency passes even when the inline copy diverges from the canonical source. The test validates the copy against itself, not against the specification.

## Impact

- End-to-end workflows broken while CI is green — discovered late (during code review or manual testing) with higher remediation cost.
- Diverged eval corpus fixtures make it impossible to know from test results alone whether the evaluation logic is correct.
- Agent workers implementing skill docs and Go types independently have no forcing function to check that their outputs are compatible unless a cross-layer test exists.

## Candidate Follow-Ups

- For any task that both adds a new Go type and documents that type in a skill (JSON format, string values, field names): require at least one test that round-trips via a JSON fixture file, not only via Go struct construction.
- Replace inline `setUp()` fixtures in `test_reviewer_eval_report.py` with loads from the canonical `cases.json` and `reviewer_eval_results.json` files so tests cannot diverge from the source of truth.
- The armature-worker skill's Definition of Done could require a cross-layer JSON fixture test when both a type and its skill documentation are in scope.
