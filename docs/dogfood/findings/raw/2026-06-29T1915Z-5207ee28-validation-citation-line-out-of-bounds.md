---
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
area: validation
date: 2026-06-29T19:15Z
---

# `arm review record` citation validator caught reviewer hallucinating an out-of-bounds line number

## What happened

During SMTC-S1-T2 review, the reviewer agent cited `internal/review/prepare.go:104`
in the `definition_of_done` result. The file is 87 lines in the diff (`@@ -0,0 +1,87 @@`).
`arm review record` rejected the assessment with:

```
assessment citation validation errors:
  - criterion result definition_of_done: citation references internal/review/prepare.go:104 which is not in diff
```

The reviewer was attempting to cite the `Prepare` function. Line 104 does not
exist — the file ends at 87. This is a classic LLM hallucination: the model
knew the function was "near the end" of the file and cited a plausible-sounding
line number that exceeded the actual file length.

## Why it matters

The citation validator in `arm review record` caught this before it was recorded,
which is the system working correctly. However, the coordinator had to manually
patch the assessment (drop the bad citation, add a path-level citation) before
recording could succeed. This is unexpected friction in what should be a
coordinator-delegates-to-reviewer-then-records flow.

## Evidence

- Bundle diff for `internal/review/prepare.go`: `@@ -0,0 +1,87 @@` (87 lines)
- Reviewer citation: `{"path": "internal/review/prepare.go", "line": 104}`
- `arm review record` exit 1: "citation references internal/review/prepare.go:104 which is not in diff"
- Fix: replaced with path-level citation `{"path": "internal/review/prepare.go"}` → record succeeded

## Positive signal

The validator is doing real work. A stale or invented line number would silently
pollute the audit record without this check.

## Potential mitigations

- The reviewer skill could note that citations for new files should prefer
  path-level citations (no `line`) when the function is "somewhere in the file"
  rather than at a known specific line — avoids off-by-one or hallucinated lines.
- Alternatively, the reviewer skill could instruct the model to verify line
  numbers against the diff hunk headers before citing them.
