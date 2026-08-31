# Criterion Status Rubric

This rubric provides detailed guidance on how to evaluate each criterion in a
ReviewBundle and assign status values (`satisfied`, `partially_satisfied`,
`not_satisfied`, `indeterminate`).

---

## Status Values and When to Use Them

### Satisfied

**Definition:**
Criterion is **fully met** with evidence from the delivery diff. The reviewer can
point to specific code, tests, or documentation that demonstrates fulfillment.

**When to assign:**
- Code changes address the criterion completely
- Tests are present and pass (for test-related criteria)
- Documentation or outcome explicitly addresses the criterion
- Edge cases and error handling are covered
- No gaps or unfinished work

**Citation requirement:** Required. At least one citation. `missing_evidence`
cannot keep a result `satisfied`.

**Rationale examples:**
- "TokenParser.Parse() method implemented with cases for all 8 token types; each covered by at least one test case."
- "Configuration option added to --config flag; documented in help text; defaults to 'strict'."
- "Error handling added for malformed input; three new tests verify rejection; no uncaught panics."

### Partially Satisfied

**Definition:**
Criterion is **partially met**. Some aspects are addressed, but gaps remain.
The delivery makes progress but is incomplete against the criterion.

**When to assign:**
- Only 50% of test coverage required is present
- Feature implemented but edge cases missing
- Code added but not all acceptance criteria in the test suite pass
- Documentation incomplete or only covers the happy path
- Refactoring partially done (some modules updated, others deferred)

**Citation requirement:** Must provide evidence of *what was done*. Must also
include `missing_evidence` field explaining *what is missing*.

**Rationale examples:**
- "Parser handles 6 of 8 token types; tests added for all 6; remaining 2 types deferred to next sprint per issue spec."
- "Configuration option added and documented; validation implemented; error messages incomplete."
- "Critical path tests pass; edge case tests pending per decision doc DESIGN-456."

**MissingEvidence examples:**
- "Token types KEYWORD and OPERATOR not yet implemented; no test cases for them."
- "Edge case: empty input array. Expected behavior undefined in spec; no test present."
- "Performance regression detection not implemented (was optional in acceptance criteria)."

### Not Satisfied

**Definition:**
Criterion is **not met** or **actively violated**. The delivery does not address
the requirement, or the delivery contradicts it.

**When to assign:**
- Expected code or tests are completely absent
- Code is deleted instead of added
- Tests fail or do not run
- Outcome explicitly states the criterion was deferred or rejected
- Requirement is violated (e.g., security hole opened when criterion was "no security regressions")

**Citation requirement:** If evidence exists in the diff (e.g., failing test),
cite it. If the criterion is missing entirely, use `missing_evidence` to explain.

**Rationale examples:**
- "TokenParser.Parse() not implemented; only stub remains. Outcome: 'deferred to next phase'."
- "Acceptance criterion: 'All tests pass'. Actual: 3 test failures in parser_test.go."
- "Criterion: 'No breaking API changes'. Actual: TokenParser.Parse() signature changed; existing callers break."

**MissingEvidence examples:**
- "TokenParser.Parse() method not found in delivery. No implementation present."
- "Expected test file parser_test.go does not exist in delivery."
- "Outcome states 'tests to be added in follow-up work'; none present in current delivery."

### Indeterminate

**Definition:**
Evidence is **insufficient or ambiguous**. The reviewer cannot confidently assess
the criterion due to truncated diff, vague outcome, or missing context.

**When to assign:**
- Diff is truncated or very large (file size limit hit)
- Outcome is too vague to map to criterion ("Done", "Fixed", "Completed")
- Criterion phrasing is ambiguous and the delivery doesn't clarify intent
- Changed files do not include the expected area (e.g., tests added but source code diff is truncated)
- External dependency or deferred work makes assessment impossible without more information

**Citation requirement:** May have citations if partial evidence is visible. Must
include `missing_evidence` explaining why status cannot be determined.

**Rationale examples:**
- "Diff truncated after 1000 lines; cannot verify test coverage. Outcome states 'tests added' but full test file not visible."
- "Outcome: 'Edge case handling added'. No specific edge cases mentioned or demonstrated in diff. Unclear which cases were added."
- "Criterion requires 85% coverage; build/coverage.txt not present in delivery. Cannot verify."

**MissingEvidence examples:**
- "Parser changes are large (>5000 lines); diff was truncated at line 1000. Full implementation not visible in review bundle."
- "Expected changed_files includes 'internal/cache/cache.go' but diff is empty for that file."
- "Outcome mentions 'concurrent access handled' but mutex/lock code not visible in truncated diff."

---

## Rating Logic (Derived from Results)

After all criteria are evaluated, the Review system automatically derives a
three-level rating:

### Green

**Criteria:** All results have status `satisfied`.

**Meaning:** Delivery fully satisfies the contract. Ready for merge and deployment.

**Example:**
```
definition_of_done:  satisfied
acceptance[0]:       satisfied
acceptance[1]:       satisfied
acceptance[2]:       satisfied
→ Rating: GREEN
```

### Yellow

**Criteria:** No results have status `not_satisfied`. At least one result is
`partially_satisfied` or `indeterminate`.

**Meaning:** Delivery is mostly good but has known gaps or ambiguities. May
require rework or clarification before merge.

**Example:**
```
definition_of_done:  satisfied
acceptance[0]:       satisfied
acceptance[1]:       partially_satisfied
acceptance[2]:       indeterminate
→ Rating: YELLOW (because no "not_satisfied", but has gaps)
```

### Red

**Criteria:** At least one result has status `not_satisfied`.

**Meaning:** Delivery violates or fundamentally fails to meet the contract.
Requires rework before merge.

**Example:**
```
definition_of_done:  satisfied
acceptance[0]:       satisfied
acceptance[1]:       not_satisfied
acceptance[2]:       satisfied
→ Rating: RED (because acceptance[1] is not met)
```

---

## Rating-to-Action Guidance

| Rating | Action | Notes |
|--------|--------|-------|
| **Green** | Merge approved | All criteria met; delivery is complete and correct |
| **Yellow** | Request clarification or rework | Gaps present; reviewer and implementer should discuss; may be acceptable with explicit approval |
| **Red** | Request rework | Unmet criteria require implementation; no merge until green or yellow |

---

## Citation Precision

Citations tie assessment results to specific locations in the delivery. Use them
to make reviews **auditable** and **precise**.

### Citation Structure

```json
{
  "path": "pkg/parser/parser.go",
  "line": 42,
  "column": 5
}
```

- **path:** File path within the delivery (relative to repo root)
- **line:** Line number (optional; 1-indexed)
- **column:** Column number (optional; 1-indexed)

### Best Practices

- **Be specific:** Point to the exact line of code or test that demonstrates the criterion
- **Multiple citations are OK:** If a criterion is demonstrated in multiple places, cite all of them
- **Test citations count:** If criterion is "tests pass", cite the test file and specific test function
- **Code + test:** Prefer citations that include both implementation and tests

### Citation Examples

**Good (specific):**
```json
{
  "citations": [
    {"path": "pkg/parser/parser.go", "line": 42},
    {"path": "pkg/parser/parser_test.go", "line": 120}
  ]
}
```

**Poor (vague):**
```json
{
  "citations": [
    {"path": "pkg/parser/parser.go"}  // no line number; could be anywhere in file
  ]
}
```

---

## Rationale Quality Guidelines

Each criterion result requires a `rationale` — a concise explanation of why the
status was assigned. Rationales are auditable and should be **specific**, **evidence-based**, and **actionable**.

### Good Rationales

- Reference specific code or test names: "Method `Parse()` at line 42 handles all 8 token types; test_parse_keyword_token, test_parse_operator_token, etc. all pass."
- Explain gaps for partially_satisfied: "Acceptance criterion requires 85% coverage; actual is 78%. Mutation testing shows 3 uncovered code paths."
- Cite unmet criteria for not_satisfied: "Criterion 'all tests pass'; actual test results: 2 failures in parser_test.go line 150, 203."

### Poor Rationales

- Vague: "Done", "Completed", "Looks good"
- Generic: "Tests look comprehensive" (which tests? how many? what do they verify?)
- Assumption-based: "Probably handles edge cases" (cite the code or tests that prove it)
- Out of scope: "The team is great at testing" (evidence from *this* delivery only)

### Rationale Length

- **Preferred:** 1-3 sentences
- **Maximum:** 1 paragraph
- If you need more than a paragraph, you likely need to rethink the status or break into multiple criteria

---

## Common Evaluation Scenarios

### Scenario 1: Test-Heavy Criterion

**Criterion:** "All token types are tested"

**Evaluation:**
1. Check for test file (e.g., `parser_test.go`) in changed files
2. Search diff for test functions covering each token type
3. If all types are covered: `satisfied`
4. If some types are covered: `partially_satisfied`
5. If no tests present: `not_satisfied`
6. If test file exists but is truncated: `indeterminate`

**Example Assessment:**
```json
{
  "id": "acceptance[0]",
  "status": "satisfied",
  "rationale": "Test file parser_test.go includes 8 test functions: test_keyword, test_operator, test_literal, test_identifier, test_number, test_string, test_comment, test_whitespace. All pass according to test output in outcome.",
  "citations": [
    {"path": "pkg/parser/parser_test.go", "line": 10},
    {"path": "pkg/parser/parser_test.go", "line": 25},
    {"path": "pkg/parser/parser_test.go", "line": 40}
  ]
}
```

### Scenario 2: Code Change Criterion

**Criterion:** "Config option added for parser behavior"

**Evaluation:**
1. Check changed files for config/settings files
2. Search diff for new flags, env vars, or config parameters
3. Verify it's actually used in code (not dead code)
4. Check for documentation or help text
5. If all present: `satisfied`
6. If code present but no docs: `partially_satisfied`
7. If only mentioned in outcome but not in diff: `not_satisfied`

**Example Assessment:**
```json
{
  "id": "acceptance[1]",
  "status": "satisfied",
  "rationale": "New flag --parser-mode added to pkg/config/flags.go (line 142) with three options: 'strict', 'lenient', 'permissive'. Flag is consumed in parser.go line 65 and passed to NewParser(). Documentation added to docs/config.md.",
  "citations": [
    {"path": "pkg/config/flags.go", "line": 142},
    {"path": "pkg/parser/parser.go", "line": 65},
    {"path": "docs/config.md", "line": 87}
  ]
}
```

### Scenario 3: Performance/Edge Case Criterion

**Criterion:** "No performance regression on large inputs"

**Evaluation:**
1. Check if benchmark tests are added
2. Look for perf comparison or baseline
3. If outcome says "benchmarks added, no regression detected": likely `satisfied`
4. If outcome says "benchmarks pending": `partially_satisfied`
5. If no perf tests and claim of "no regression": `indeterminate` (cannot verify)

**Example Assessment (Yellow status):**
```json
{
  "id": "acceptance[2]",
  "status": "partially_satisfied",
  "rationale": "Benchmark file added (parser_bench_test.go) with BenchmarkParseToken and BenchmarkParseLargeFile. Results show 5% improvement over baseline. However, integration tests (parsing full document) not yet present; deferred per DECISION-789.",
  "citations": [
    {"path": "pkg/parser/parser_bench_test.go", "line": 1}
  ],
  "missing_evidence": "End-to-end performance test (parsing 10MB+ file) not implemented; decision document defers this to next iteration."
}
```

---

## Validation Checklist

Before submitting your ConformanceAssessment, verify:

- [ ] One result for `definition_of_done` + all acceptance criteria (no more, no fewer)
- [ ] Each result has `id`, `status`, `rationale`, and either `citations` or `missing_evidence`
- [ ] `bundle_id` matches input ReviewBundle exactly
- [ ] `contract_fingerprint` and `delivery_fingerprint` match input exactly
- [ ] All citations point to files in `changed_files`
- [ ] Rationales are concrete and evidence-based (no vague language like "looks good")
- [ ] Status values are consistent (`satisfied` has citations; other empty-citation statuses have `missing_evidence`)
- [ ] JSON is valid and passes schema validation

---

## FAQ

**Q: What if the criterion is ambiguous or poorly written?**
A: Interpret the criterion in the most reasonable way based on the outcome and diff.
If you cannot decide, assign `indeterminate` with `missing_evidence` explaining the ambiguity.
Document your interpretation in the rationale.

**Q: What if the outcome contradicts the diff?**
A: Prefer the diff (code doesn't lie). If outcome says "all tests pass" but the diff shows failing tests,
mark the criterion as `not_satisfied` and cite the failing tests in the diff.

**Q: What if the diff is huge and truncated?**
A: Assign `indeterminate` for criteria you cannot fully assess. Cite the visible parts and use
`missing_evidence` to explain what was truncated.

**Q: Can I give partial credit for "almost working" code?**
A: Yes — that's `partially_satisfied`. Use it when implementation is ~70-90% complete but has
known gaps. Cite the working parts and document missing parts in `missing_evidence`.

**Q: How do I handle deferred work?**
A: Check the decision documents (linked in the outcome). If work is explicitly deferred per
a decision, mark as `partially_satisfied` (if some work done) or `not_satisfied` (if none done).
Cite the decision document in the rationale.

