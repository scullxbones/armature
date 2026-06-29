# Eval Corpus Divergence — Four Copies, No Single Source of Truth

**Date:** 2026-06-29  
**Writer:** 5207ee28-cdd8-48e6-98dc-7da179d4a40d  
**Area:** validation

## What I Was Trying to Do

Fix the case-008 `expected_rating` from `yellow` to `red` across the eval corpus as part of PR review fixes for SMTC-S1.

## What Happened

The fix required editing four separate locations for a single logical change:

1. `internal/review/testdata/evals/cases.json` — the canonical case definitions
2. `scripts/testdata/reviewer_eval_results.json` — reference reviewer eval results
3. `scripts/test_reviewer_eval_report.py` (line ~61) — inline `self.cases` fixture in `setUp()`
4. `scripts/test_reviewer_eval_report.py` (line ~104) — inline `self.perfect_results` fixture in `setUp()`
5. A case description string in `cases.json` (also said "→ yellow")
6. Comments in `test_reviewer_eval_report.py` about count of "green/yellow" cases

The Python test `TestEvaluatorMetrics` passed even before the fix — the two inline fixtures were internally consistent with each other (both said yellow for case-008), so `rating_accuracy` came out 1.0 regardless of whether the expectation matched the canonical corpus.

## Effect on Confidence and Time

The test suite gave a false green: the `test_reviewer_eval_report.py` tests passed even with an incorrect expectation in `cases.json`, because the inline fixtures are self-referential rather than loaded from the canonical source. This masked the bug for an unknown period.

Finding the four locations required grep across the codebase — it was not obvious that there were inline fixtures in the test file duplicating the canonical corpus data.

## Evidence

- Opus code review explicitly flagged this as finding E1 ("Align case-008 with the rating algebra").
- The PR branch had `cases.json` already fixed to `red` but `reviewer_eval_results.json` and both Python inline fixtures still said `yellow`.
- Python tests passed throughout with `yellow` in the inline fixtures.

## What Would Help

A single authoritative source for eval cases, loaded by both the Python eval runner (`reviewer_eval_report.py`) and the Python tests (`test_reviewer_eval_report.py`). The test file's `setUp()` should load from `scripts/testdata/reviewer_eval_results.json` or `internal/review/testdata/evals/cases.json` rather than maintaining its own inline copy. Any fixture duplication creates a class of silent divergence bugs.
