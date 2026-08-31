# ConformanceAssessment Field Validation Rules

This document specifies mandatory validation rules for every field in the ConformanceAssessment JSON.
These rules enforce citation precision and assessment integrity.

---

## Global Assessment Fields

### `schema_version`

**Type:** Integer  
**Required:** Yes  
**Valid values:** `1`  
**Validation rule:** Must be exactly `1`. No other versions are currently supported.

**Failure action:** Reject the assessment if schema_version is not 1.

---

### `bundle_id`

**Type:** String  
**Required:** Yes  
**Format:** Non-empty string matching the bundle ID from the input ReviewBundle  
**Validation rule:** Must match the `bundle_id` from the input ReviewBundle exactly. This binds
the assessment to the specific bundle it evaluates.

**Examples:**
- ✓ Valid: `"bundle-abc123"` (if input ReviewBundle has bundle_id: "bundle-abc123")
- ✗ Invalid: `"bundle-abc"` (does not match input: "bundle-abc123")
- ✗ Invalid: `"bundle-abc123-v2"` (does not match input: "bundle-abc123")
- ✗ Invalid: `""` (empty string)

**Failure action:** Reject the assessment if bundle_id does not match the input exactly.

---

### `contract_fingerprint`

**Type:** String  
**Required:** Yes  
**Format:** Hex-encoded SHA-256 hash (64 characters, lowercase)  
**Validation rule:** Must match `fingerprints.contract` from the input ReviewBundle exactly.
Ensures the assessment is evaluated against the same contract requirements.

**Examples:**
- ✓ Valid: `"abc123def456..."` (64-char hex string matching input fingerprint)
- ✗ Invalid: `"ABC123..."` (uppercase; hashes must be lowercase)
- ✗ Invalid: `"abc123..."` (only 48 chars; must be 64)
- ✗ Invalid: `"abc123xyz..."` (non-hex characters)

**Failure action:** Reject the assessment if contract_fingerprint does not match the input exactly.

---

### `delivery_fingerprint`

**Type:** String  
**Required:** Yes  
**Format:** Hex-encoded SHA-256 hash (64 characters, lowercase)  
**Validation rule:** Must match `fingerprints.delivery` from the input ReviewBundle exactly.
Ensures the assessment is evaluated against the exact diff that was delivered.

**Examples:**
- ✓ Valid: `"def456abc123..."` (64-char hex string matching input fingerprint)
- ✗ Invalid: `"DEF456..."` (uppercase)
- ✗ Invalid: `"def456..."` (only 48 chars)

**Failure action:** Reject the assessment if delivery_fingerprint does not match the input exactly.

---

### `results`

**Type:** Array of CriterionResult objects  
**Required:** Yes  
**Validation rule:** Must include exactly one result per criterion in the contract:
- Exactly 1 result with id `"definition_of_done"`
- Exactly N results with ids `"acceptance[0]"`, `"acceptance[1]"`, ..., `"acceptance[N-1]"`
  where N is the number of acceptance criteria in the input contract

**Examples:**
- ✓ Valid: 3 results (definition_of_done + 2 acceptance criteria)
  ```json
  "results": [
    {"id": "definition_of_done", ...},
    {"id": "acceptance[0]", ...},
    {"id": "acceptance[1]", ...}
  ]
  ```
- ✗ Invalid: Missing acceptance[1]
  ```json
  "results": [
    {"id": "definition_of_done", ...},
    {"id": "acceptance[0]", ...}
  ]
  ```
- ✗ Invalid: Duplicate definition_of_done
  ```json
  "results": [
    {"id": "definition_of_done", ...},
    {"id": "definition_of_done", ...},
    {"id": "acceptance[0]", ...}
  ]
  ```

**Failure action:** Reject the assessment if results array is empty, has duplicate criterion IDs,
or is missing any expected criteria.

---

## CriterionResult Fields

Each result in the `results` array must validate according to these rules.

### `id`

**Type:** String  
**Required:** Yes  
**Format:** `"definition_of_done"` or `"acceptance[N]"` where N is an integer ≥ 0  
**Validation rule:** Must be a valid criterion ID matching the contract. Ordering must match the
contract order: definition_of_done first, then acceptance[0], acceptance[1], etc.

**Examples:**
- ✓ Valid: `"definition_of_done"`
- ✓ Valid: `"acceptance[0]"`, `"acceptance[1]"`, `"acceptance[2]"`
- ✗ Invalid: `"acceptance[0.5]"` (not an integer)
- ✗ Invalid: `"acceptance[-1]"` (negative index)
- ✗ Invalid: `"Acceptance[0]"` (capitalized)
- ✗ Invalid: `"acceptance_0"` (underscore instead of brackets)

**Failure action:** Reject the assessment if any result ID is malformed or does not match the contract.

---

### `status`

**Type:** String  
**Required:** Yes  
**Valid values:** `"satisfied"`, `"partially_satisfied"`, `"not_satisfied"`, `"indeterminate"`  
**Validation rule:** Must be one of the four allowed values (lowercase, with underscores for multi-word statuses).

**Examples:**
- ✓ Valid: `"satisfied"`, `"partially_satisfied"`, `"not_satisfied"`, `"indeterminate"`
- ✗ Invalid: `"satisfied_fully"` (not in the allowed set)
- ✗ Invalid: `"Satisfied"` (capitalized)
- ✗ Invalid: `"partial"` (incomplete)
- ✗ Invalid: `"in-progress"` (hyphen instead of underscore; also not a valid status)

**Failure action:** Reject the assessment if status is not one of the four allowed values.

---

### `rationale`

**Type:** String  
**Required:** Yes  
**Format:** Non-empty string, preferably 1-3 sentences or 1 paragraph  
**Validation rule:** Must be present and non-empty. Should be concrete and evidence-based,
referencing specific code, tests, or documentation. See `references/rubric.md` for rationale quality guidelines.

**Minimum length:** 10 characters (to avoid placeholder text like "OK" or "Yes")

**Examples:**
- ✓ Valid: `"TokenParser.Parse() implemented with all 8 token types; each covered by at least one test case."`
- ✓ Valid: `"Implementation complete for 6 of 8 token types; remaining 2 deferred per DESIGN-456. All 6 have passing tests."`
- ✗ Invalid: `"OK"` (too vague, too short)
- ✗ Invalid: `"Done"` (no specific evidence)
- ✗ Invalid: `""` (empty string)

**Failure action:** Reject the assessment if rationale is missing or less than 10 characters.

---

### `citations`

**Type:** Array of Citation objects  
**Required:** Conditionally  
**Format:** Each citation has `path` (string) and optional `line` (integer) and `column` (integer)  
**Mandatory line-citation validation rules:**

1. **Every citation must point to a file in the delivery's changed_files**
   - The `path` field must match a file in `delivery.changed_files` from the input ReviewBundle
   - Path comparison is case-sensitive and must be exact

2. **If a line number is specified, it must exist in that file's diff hunk**
   - Line numbers are 1-indexed
   - The line must have been added or modified in the diff (marked with + in the unified diff)
   - Context lines (unchanged lines shown for readability in the diff) cannot be cited by line number; use path-level citations instead (see item 3 below)
   - If the file was added (new file), all lines in the file were added, so any line ≤ file length is valid
   - If the file was deleted, line-number citations are not valid (reject assessment) — see item 3 for path-level citations to deleted files

3. **Path-level citations (line omitted or 0) are valid if**
   - The file is in changed_files
   - The entire file is the relevant evidence (e.g., "this test file demonstrates the criterion")
   - Note: unlike line-number citations, a path-level citation to a **deleted** file is still valid (the file's entry is present in changed_files) — use it only to cite the deletion itself, not remaining file content

4. **Citation array may be empty only if `missing_evidence` is present.**
   - This applies to every status, including `"satisfied"`.
   - Evidence-free satisfaction is invalid: a `satisfied` result with neither
     citations nor `missing_evidence` is rejected.

5. **Citation array must be non-empty OR `missing_evidence` must be present**
   - For every status
   - A dropped citation that leaves a criterion with no remaining evidence
     cannot stay `satisfied` without `missing_evidence` (and a behavioral
     gate claim in that position is `not_satisfied`)

**Examples:**

✓ Valid citations:
```json
"citations": [
  {"path": "pkg/parser/parser.go", "line": 42},
  {"path": "pkg/parser/parser_test.go", "line": 120}
]
```

✓ Valid path-level citation (entire file is evidence):
```json
"citations": [
  {"path": "pkg/parser/parser_test.go"}
]
```

✓ Valid with column (optional but encouraged for precision):
```json
"citations": [
  {"path": "pkg/parser/parser.go", "line": 42, "column": 10}
]
```

✗ Invalid (file not in changed_files):
```json
"citations": [
  {"path": "pkg/other/other.go", "line": 42}
]
```

✗ Invalid (line number does not exist in diff):
```json
"citations": [
  {"path": "pkg/parser/parser.go", "line": 9999}
]
```

✗ Invalid (negative line number):
```json
"citations": [
  {"path": "pkg/parser/parser.go", "line": -1}
]
```

✗ Invalid (no citations and no missing_evidence for non-satisfied status):
```json
{
  "id": "acceptance[0]",
  "status": "not_satisfied",
  "rationale": "Not implemented",
  "citations": []
}
```

**Failure action:** Reject the assessment if:
- Any citation path is not in changed_files
- Any citation line number is invalid (≤ 0, or does not exist in the diff)
- Citations array is empty AND `missing_evidence` is absent, for any status
  including `"satisfied"`

---

### `missing_evidence`

**Type:** String  
**Required:** Conditionally  
**Format:** Non-empty string explaining what evidence is absent  
**Validation rule:**

1. **Required if:**
   - Citations array is empty, for **every** status including `"satisfied"`
   - Explains what evidence would be needed to make a confident assessment

2. **Optional if:**
   - Citations are already present
   - Abundant citations already explain what was done

3. **Should explain:**
   - What evidence is missing from the diff (e.g., "Test file not in changed_files")
   - Why the missing evidence matters to the criterion
   - What the reviewer would need to see to upgrade the status

**Examples:**

✓ Valid (for partially_satisfied):
```json
{
  "status": "partially_satisfied",
  "citations": [{"path": "pkg/parser/parser.go", "line": 42}],
  "missing_evidence": "Token types KEYWORD and OPERATOR not yet implemented; test cases for them are absent."
}
```

✓ Valid (for not_satisfied, no citations):
```json
{
  "status": "not_satisfied",
  "citations": [],
  "missing_evidence": "TokenParser.Parse() method not found in delivery; only stub remains."
}
```

✓ Valid (for indeterminate):
```json
{
  "status": "indeterminate",
  "citations": [{"path": "pkg/parser/parser.go"}],
  "missing_evidence": "Diff truncated after 1000 lines; cannot verify implementation completeness."
}
```

✗ Invalid (missing_evidence absent but needed):
```json
{
  "status": "partially_satisfied",
  "citations": [],
  "rationale": "Some work done"
}
```

**Failure action:** Reject the assessment if citations is empty and missing_evidence is absent,
for any status including `"satisfied"`.

---

## Citation Precedence and Conflict Resolution

When a result has both `citations` and `missing_evidence`:

- **Citations** document what *was* done or *is* evidence
- **missing_evidence** explains what *was not* done or *cannot* be verified

This is valid for `partially_satisfied` and `indeterminate` statuses.

**Example:**
```json
{
  "id": "acceptance[0]",
  "status": "partially_satisfied",
  "rationale": "Configuration option added (flag parsing works); error messages incomplete.",
  "citations": [
    {"path": "pkg/config/flags.go", "line": 142}
  ],
  "missing_evidence": "Error message localization and validation error output not yet implemented."
}
```

---

## Write and validate (Step 5b)

Write the assessment file, then run:

```bash
arm review validate --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE" --format json
```

Retry and response shapes are in SKILL.md step 5b. Do not return to the
coordinator until that command exits 0, or use step 6's exhausted-retry
chat shape if the retry cap is reached.

---

## Common Errors and Fixes

| Error | Fix |
|-------|-----|
| `bundle_id mismatch` | Copy bundle_id from input ReviewBundle exactly |
| `Fingerprint mismatch` | Ensure fingerprints match input exactly (case-sensitive) |
| `Invalid status` | Use only: `satisfied`, `partially_satisfied`, `not_satisfied`, `indeterminate` |
| `Citation path not in changed_files` | Verify file path matches exactly; check for typos |
| `Citation line does not exist` | Ensure line number appears in the file's diff hunk; verify 1-indexed |
| `Missing citations for any status` | Add citations pointing to evidence or add `missing_evidence` explanation |
| `Empty rationale` | Provide concrete, evidence-based explanation (≥ 10 characters) |
| `Duplicate criterion IDs` | Ensure results array has no duplicate ids |
| `Missing acceptance criterion` | Add missing `acceptance[N]` result to match contract |

