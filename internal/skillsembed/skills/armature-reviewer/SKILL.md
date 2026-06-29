---
name: armature-reviewer
description: >
  Use when receiving a ReviewBundle from arm review prepare and producing a
  ConformanceAssessment JSON. Evaluates each criterion from the contract against
  the delivery diff, records evidence as citations, and returns the assessment
  JSON to the coordinator (which records it via arm review record).
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Reviewer

The Reviewer evaluates a prepared ReviewBundle against the contract requirements
and delivery diff. It produces a ConformanceAssessment JSON with criterion-level
results, citations, and ratings, then returns the JSON to the coordinator.
The coordinator is responsible for recording the assessment via `arm review record`.

## Prerequisites

If `arm` is not found, stop and resolve this before proceeding.

The Reviewer does not require `arm worker-init`. The ReviewBundle is pre-prepared
by the Coordinator or harness via `arm review prepare`.

---

## The Review Workflow

```
ReviewBundle file path (from coordinator)
    ↓
Evaluate each criterion against delivery
    ↓
Record citations (file paths, line numbers)
    ↓
Assign status (satisfied, partially_satisfied, not_satisfied, indeterminate)
    ↓
Produce ConformanceAssessment JSON
    ↓
Return assessment JSON to coordinator
    ↓
Coordinator: arm review record --issue ISSUE-ID --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"
    ↓
AssessmentAttestation (durable record)
```

---

## Input: ReviewBundle

The ReviewBundle is a JSON structure produced by `arm review prepare`. It contains:

- **Issue** — the reviewed issue ID, type, title, and recorded outcome
- **Contract** — definition_of_done and ordered acceptance criteria
- **Delivery** — base/head SHAs, changed files, and unified diff
- **Fingerprints** — canonical SHA-256 hashes for reproducibility and idempotence

You receive the full ReviewBundle as input (typically via stdin or a JSON file).

### ReviewBundle Example

```json
{
  "schema_version": 1,
  "bundle_id": "bundle-abc123",
  "issue": {
    "id": "TASK-42",
    "type": "task",
    "title": "Implement TokenParser.Parse()",
    "outcome": "Implemented Parse() with 8 token types; all tests green; 82% coverage"
  },
  "contract": {
    "definition_of_done": "TokenParser.Parse() handles all token types without panicking",
    "acceptance": [
      "All 8 token types parse correctly",
      "Tests cover each token type",
      "No uncaught panics in Parse()"
    ]
  },
  "delivery": {
    "base_sha": "abc123",
    "head_sha": "def456",
    "changed_files": ["pkg/parser/parser.go", "pkg/parser/parser_test.go"],
    "diff": "... unified diff (may be empty if large) ..."
  },
  "fingerprints": {
    "contract": "sha256-contract-hash",
    "delivery": "sha256-delivery-hash"
  }
}
```

---

## Step-by-Step Review Process

### 1. Parse and Validate the ReviewBundle

```json
{
  "schema_version": 1,
  "bundle_id": "...",
  ...
}
```

Verify:
- `schema_version` is 1
- `bundle_id` is present and non-empty
- `issue.id`, `issue.type`, `contract.definition_of_done` are all present
- `fingerprints.contract` and `fingerprints.delivery` are set

If validation fails, report the error with the bundle ID and stop.

### 2. Evaluate Definition of Done

This is the primary criterion. Reference `references/rubric.md` for guidance on
assigning status.

```json
{
  "id": "definition_of_done",
  "status": "satisfied|partially_satisfied|not_satisfied|indeterminate",
  "rationale": "Explain why this status was assigned",
  "citations": [
    {"path": "pkg/parser/parser.go", "line": 42},
    {"path": "pkg/parser/parser_test.go", "line": 120},
    {"path": "pkg/parser/types.go"}
  ],
  "missing_evidence": "Only if status is not_satisfied, partially_satisfied, or indeterminate AND no citations present"
}
```

> **Note:** `line` is optional. Omitting it (or setting it to `0`) creates a **path-level citation** that validates against file presence in the diff rather than a specific line number. Use path-level citations when the evidence spans an entire file or no specific line is more relevant than another.

```json
```

### 3. Evaluate Each Acceptance Criterion

For each acceptance criterion in order:

```json
{
  "id": "acceptance[0]",
  "status": "satisfied|partially_satisfied|not_satisfied|indeterminate",
  "rationale": "Explain the assessment",
  "citations": [
    {"path": "...", "line": 123}
  ],
  "missing_evidence": "Only if needed"
}
```

Criteria are indexed starting at 0: `acceptance[0]`, `acceptance[1]`, etc.

### 4. Assign Ratings

After evaluating all criteria, derive the **Rating**:

- **Green** — all criteria are `satisfied`
- **Yellow** — some criteria are `partially_satisfied` or `indeterminate`, none `not_satisfied`
- **Red** — at least one criterion is `not_satisfied`

The rating is computed automatically by `arm review record` from the results.

### 5. Produce ConformanceAssessment JSON

Assemble all criterion results into a ConformanceAssessment:

```json
{
  "schema_version": 1,
  "bundle_id": "bundle-abc123",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "...",
      "citations": [...]
    },
    {
      "id": "acceptance[0]",
      "status": "satisfied",
      "rationale": "...",
      "citations": [...]
    }
  ],
  "contract_fingerprint": "sha256-contract-hash",
  "delivery_fingerprint": "sha256-delivery-hash"
}
```

**Constraints:**
- `bundle_id` must match the input ReviewBundle exactly
- `contract_fingerprint` and `delivery_fingerprint` must match the input fingerprints exactly
- `results` must include one result per criterion (definition_of_done + all acceptance criteria)
- Each result must pass `CriterionResult.Valid()` — see `references/rubric.md` for details

### 6. Return the ConformanceAssessment

Output the ConformanceAssessment JSON to stdout (or return it to the coordinator). Do **not** call `arm review record` — recording is the coordinator's responsibility. The coordinator passes the assessment to `arm review record --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"` after receiving it, so the fingerprint validation is bound to the exact bundle it dispatched.

```bash
# Output the assessment JSON so the coordinator can capture it:
cat assessment.json
```

---

## Criterion Evaluation Rubric

See `references/rubric.md` for detailed guidance on:
- How to interpret criterion status values
- When to use each status
- How to phrase rationales
- How to structure citations
- When missing evidence is required

---

## Common Review Patterns

### Pattern 1: Happy Path (All Green)

- Delivery includes complete implementation
- Tests cover all acceptance criteria
- No defects or edge cases
- Outcome is concrete and addresses each criterion

→ Assign `satisfied` to all criteria → Rating: **Green**

### Pattern 2: Partial Delivery (Yellow)

- Most acceptance criteria met
- Some criteria partially addressed (e.g., "tests added but coverage incomplete")
- No active violations or defects
- Outcome documents what was done and what remains

→ Assign `satisfied` to fully-met criteria, `partially_satisfied` to incomplete ones → Rating: **Yellow**

### Pattern 3: Broken (Red)

- At least one acceptance criterion is not met
- Delivery actively violates the contract (e.g., code deleted instead of added)
- Tests fail or are missing
- Outcome does not address the criterion

→ Assign `not_satisfied` to broken criteria → Rating: **Red**

### Pattern 4: Ambiguous Delivery (Yellow/Red)

- Diff is truncated or very large
- Cannot determine if criterion is met from available evidence
- Outcome is vague ("Done", "Completed")

→ Assign `indeterminate` with `missing_evidence` explaining why → Rating: **Yellow** or **Red** depending on severity

---

## Returning Results to the Coordinator

After producing the ConformanceAssessment JSON, return it to the coordinator. Do **not** call `arm review record` — that is the coordinator's responsibility. The coordinator records the assessment with `--bundle "$BUNDLE_FILE"` so fingerprint validation is bound to the exact bundle it prepared.

**Example Workflow:**

```bash
# 1. Receive ReviewBundle file path (from coordinator)
# The coordinator passes: $BUNDLE_FILE

# 2. Review and evaluate
# ... create assessment.json ...

# 3. Output the assessment JSON for the coordinator to capture
cat assessment.json

# The coordinator then runs:
# arm review record --issue TASK-42 --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"
```

The recorded assessment is durable and can be queried later:

```bash
arm review show TASK-42
```

---

## Validation and Idempotence

**Validation:**
```bash
# Validate a ConformanceAssessment JSON before recording
# (arm review record will reject invalid assessments)
```

**Idempotence:**
- If you record the same bundle twice with the same results, the fingerprints will match
- `arm review record` detects this and returns the same rating without duplicating the record
- This allows safe retry logic if the review process is interrupted

---

## Error Handling

### Invalid ReviewBundle

- Bundle fails `schema_version` check
- Missing required fields (issue.id, contract.definition_of_done, fingerprints)
- Cannot validate → report error with bundle ID

**Action:** Stop and ask for a fresh ReviewBundle from the coordinator.

### Invalid ConformanceAssessment

- Results array is empty
- Missing a criterion (e.g., no acceptance[1] when contract has 2 acceptance criteria)
- `bundle_id` does not match input
- Fingerprints do not match

**Action:** Fix the assessment JSON and retry.

### arm review record Failure

- Assessment file not found
- Assessment JSON is malformed
- Issue ID does not exist

**Action:** Check the error message and fix the issue, then retry.

---

## Command Reference

```bash
# Prepare a bundle (done by coordinator, not reviewer)
arm review prepare --issue TASK-42 --title "Implement feature X" \
  --scope "pkg/feature.go" "pkg/feature_test.go" \
  --criteria "Feature must compile" "All tests pass" \
  --base abc123 --head def456

# Record an assessment (done by coordinator, not reviewer)
arm review record --issue TASK-42 --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"

# Display recorded assessment
arm review show TASK-42

# List all assessments for a story
arm review list --story STORY-99
```

