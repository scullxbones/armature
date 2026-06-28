# Evaluator Corpus

This directory contains the labeled evaluation corpus for the reviewer metric system.

## Corpus Format

### cases.json

Each case in `cases.json` represents a labeled review scenario with expected outcomes. The format is:

```json
{
  "id": "case-001",
  "description": "Case description",
  "bundle": {
    "issue_id": "TEST-1",
    "contract": {
      "issue_id": "TEST-1",
      "title": "Issue title",
      "scope": ["file1.go"],
      "acceptance_criteria": [
        {"id": "AC1", "text": "Criterion text"}
      ]
    },
    "delivery": {
      "issue_id": "TEST-1",
      "git_base": "commit_hash",
      "git_head": "commit_hash",
      "diff": "diff content",
      "changed_files": ["file1.go"]
    }
  },
  "expected_rating": "green",
  "expected_statuses": {"AC1": "satisfied"}
}
```

**Fields:**
- `id`: Unique case identifier
- `description`: Human-readable case description
- `bundle.contract`: The acceptance criteria contract (what was promised)
- `bundle.delivery`: The actual implementation (what was delivered)
- `expected_rating`: Expected overall rating (`green`, `yellow`, or `red`)
- `expected_statuses`: Map of criterion ID to status (`satisfied`, `partially_satisfied`, `not_satisfied`, or `indeterminate`)

**Status meanings:**
- `satisfied`: Criterion fully met by the delivery
- `partially_satisfied`: Criterion partially met (not fully addressed)
- `not_satisfied`: Criterion not met
- `indeterminate`: Cannot determine status from available evidence (e.g., no relevant diff)

## Running the Evaluator

### Smoke Test

To verify the evaluator works correctly with perfect results:

```bash
python3 scripts/reviewer_eval_report.py \
  --cases internal/review/testdata/evals/cases.json \
  --results scripts/testdata/reviewer_eval_results.json
```

This should exit with code 0.

### Unit Tests

To run all evaluator tests:

```bash
python3 -m unittest scripts/test_reviewer_eval_report.py -v
```

## Metrics Computed

The evaluator computes and reports these 5 metrics:

1. **rating_accuracy**: Fraction of cases where actual rating matches expected
2. **status_accuracy**: Fraction of criterion statuses correct across all cases
3. **false_red_rate**: Proportion of cases expected green/yellow but rated red
4. **false_green_rate**: Proportion of cases expected red but rated green/yellow
5. **partial_detection_rate**: Fraction of partially_satisfied criteria correctly identified

The evaluator exits with code 0 if `rating_accuracy >= 0.75`, else 1.
