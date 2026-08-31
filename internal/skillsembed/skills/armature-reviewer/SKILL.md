---
name: armature-reviewer
description: >
  Use when receiving a ReviewBundle from arm review prepare and producing a
  ConformanceAssessment JSON. Evaluates each criterion from the contract against
  the delivery diff, records evidence as citations, writes the assessment JSON
  under .armature/review/ once, runs arm review validate and applies suggestions
  for at most 3 attempts after the first failure, then returns rating, findings,
  and that path — or, if validation never succeeded or failed operationally,
  a no-path shape, never a recordable path.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Reviewer

The Reviewer evaluates a prepared ReviewBundle against the contract requirements
and delivery diff. It produces a ConformanceAssessment JSON with criterion-level
results, citations, and ratings, writes that JSON under `.armature/review/`
**once**, runs `arm review validate` (applying suggestions for at most 3
attempts after the first failure), and on success returns only the rating,
actionable findings, and the assessment path. If validation is still failing
after that cap, return the exhausted-retry shape in step 6. If `arm review
validate` instead fails operationally — a path, the bundle, or issue state
is broken, including a `valid: false` report whose only suggestion is to
re-run `arm review prepare` — do not retry; return the operational-error
shape so the coordinator can repair the setup. In neither
case return a recordable path. The coordinator is responsible for recording a validated
assessment via `arm review record`. Schema and citation-bound retries belong
in this skill, not in coordinator post-processing.

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
Write ConformanceAssessment JSON once to a unique $ASSESSMENT path
    ↓
arm review validate --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE"
    ↓ (invalid: apply suggestions to the same path, rewrite, re-validate;
       at most 3 attempts after the first failure)
    ↓ (valid)
Return rating + findings + assessment path to coordinator
    ↓
Coordinator: arm review record --issue ISSUE-ID --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"
    ↓
AssessmentAttestation (durable record)

    (still invalid after the cap → exhausted-retry chat; coordinator does not record)
    (setup/operational: prepare-the-bundle suggestion, {error:...}, or Step 1 miss
     → operational-error chat; coordinator repairs setup)
```

---

## Input: ReviewBundle

The ReviewBundle is a JSON structure produced by `arm review prepare`. It contains:

- **Issue** — the reviewed issue ID, type, title, and recorded outcome
- **Contract** — definition_of_done and ordered acceptance criteria
- **Delivery** — base/head SHAs, changed files, and unified diff
- **Fingerprints** — canonical SHA-256 hashes for reproducibility and idempotence

You receive the full ReviewBundle as input (typically via stdin or a JSON file).

**Confirmation mode.** If the coordinator also passes a findings-scope file
(the remediating set), this is a **hard-scoped confirmation**, not a new
comprehensive review. Re-evaluate those findings against the new bundle
and put only that set in the chat findings. The assessment JSON still
includes one result per contract criterion (schema requirement). Record
any out-of-scope defect you notice, but treat it as a blocker only at
critical severity. Do not invent a second findings list or restart the
serial discovery loop. If no findings-scope file is passed, this is the
one comprehensive initial review.

### ReviewBundle Example

```json artifact_type=review-bundle
{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
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
    "base_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "head_sha": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "changed_files": ["pkg/parser/parser.go", "pkg/parser/parser_test.go"],
    "diff": "... unified diff (may be empty if large) ..."
  },
  "fingerprints": {
    "contract": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "delivery": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
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

If validation fails (missing `schema_version`, `bundle_id`, or a
fingerprint), return step 6's **operational-error** shape — this is a
setup failure, not an assessment retry:

1. `Validation: error`
2. the missing-field reason, including the bundle ID if present
3. `Assessment: not returned`

Do not write `$ASSESSMENT`. Do not start step 5b. The coordinator repairs
the bundle and re-dispatches.

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

### 3. Evaluate Each Acceptance Criterion

For each acceptance criterion in order:

```json
{
  "id": "acceptance[0]",
  "status": "satisfied|partially_satisfied|not_satisfied|indeterminate",
  "rationale": "Explain the assessment",
  "citations": [
    {"path": "...", "line": 123},
    {"activity_entry_id": "0"}
  ],
  "missing_evidence": "Only if needed"
}
```

Criteria are indexed starting at 0: `acceptance[0]`, `acceptance[1]`, etc.

**Citation types:**
- **Diff citations** (`path`, `line`, `column`) — evidence from the code diff
- **Activity citations** (`activity_entry_id`) — evidence from the activity log (raw entry ID, never the index)

A single citation object must use exactly one of these forms: `path` (with optional
`line`/`column`) **or** `activity_entry_id`, never both. `arm review record` rejects
a citation that sets both.

`activity_entry_id` is a **plain, 0-based integer as a string** (`"0"`, `"1"`, `"2"`, …) —
the physical line number of the entry in the activity log, exactly as returned by the
Activity Indexer's `id` field. It is not zero-padded and not 1-based.

Citations recorded here are subject to the mandatory verification rules:
- Every diff citation (`{"path", "line"}`) must resolve against an actual diff hunk (Step 5a)
- Every activity citation (`{"activity_entry_id"}`) must reference a valid entry ID from the activity log
- An activity citation whose entry has `exit_status: "unknown"` (harness did not report an exit
  code) cannot support a `satisfied` verdict on the criterion it's attached to — treat it the
  same as missing evidence for that purpose
- Activity citations follow the upgrade-only rule (lift indeterminate verdicts on behavioral criteria only)

### 3a. Gate Evidence Acceptance Rule (normative)

When a criterion asserts a behavioral gate outcome (e.g., "the full gate
passes", "`make check` is green"), you may treat it as **satisfied** only
from **citable gate evidence**: an evidence op with `exit=0`, `profile=full`,
and `sha` equal to the bundle's `delivery.head_sha`.

- **Older SHA, `profile=fast`, or no evidence op at all ⇒ rerun required.**
  Do not accept a self-reported "tests pass" claim from the outcome text,
  activity log prose, or worker narration as gate evidence — only the
  recorded evidence op counts.
- Never assign `indeterminate` as a way to route around missing gate
  evidence. If the required evidence op is absent or stale, the criterion is
  `not_satisfied` with `missing_evidence` stating that a full-profile gate run
  at the bundle head is required, not an ambiguous or soft rating.
- A fast-profile gate run is valid evidence of iteration but never satisfies a
  full-gate criterion, regardless of SHA or exit code.

### 4. Assign Ratings

After evaluating all criteria, derive the **Rating**:

- **Green** — all criteria are `satisfied`
- **Yellow** — some criteria are `partially_satisfied` or `indeterminate`, none `not_satisfied`
- **Red** — at least one criterion is `not_satisfied`

The rating is computed automatically by `arm review record` from the results.

### 5. Produce ConformanceAssessment JSON

Assemble all criterion results into a ConformanceAssessment. See `templates/conformance-assessment.json`
for a complete verbatim template. Write the draft once and machine-validate it
with `arm review validate` (step 5b). Do not rewrite `$ASSESSMENT` after that
command exits 0. Do not return a recordable path until it exits 0; if the
retry cap is reached, use the exhausted-retry shape in step 6. The same checks are documented in the
[conformance-assessment schema](https://github.com/scullxbones/armature/blob/main/docs/schemas/conformance-assessment.schema.json);
the input ReviewBundle is validated separately against the [review-bundle schema](https://github.com/scullxbones/armature/blob/main/docs/schemas/review-bundle.schema.json). See
`docs/json-schema-examples.md` for worked examples.

```json artifact_type=conformance-assessment
{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "TokenParser.Parse() handles all 8 token types without panicking, per parser.go and its tests.",
      "citations": [
        {"path": "pkg/parser/parser.go", "line": 42}
      ]
    },
    {
      "id": "acceptance[0]",
      "status": "satisfied",
      "rationale": "All 8 token types parse correctly per parser_test.go.",
      "citations": [
        {"path": "pkg/parser/parser_test.go", "line": 120}
      ]
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}
```

**Constraints:**
- `bundle_id` must match the input ReviewBundle exactly
- `contract_fingerprint` and `delivery_fingerprint` must match the input fingerprints exactly
- `results` must include one result per criterion (definition_of_done + all acceptance criteria)
- Each result must pass `CriterionResult.Valid()` — see `references/rubric.md` for details
- **Every citation must correspond to a specific added/modified (`+`) line in a diff hunk** in the delivery.
  See `references/field-rules.md` for mandatory line-citation validation rules.

### 5a. Self-Check: Validate Citations Against Diff Hunks and Activity Log (Mandatory)

Before writing the assessment and running step 5b, verify that every citation is valid:

1. **For each diff citation in every result:**
   - Confirm the file path matches a file in `delivery.changed_files`
   - If a line number is specified, verify it is an added/modified (`+`) line in that file's diff hunk — unchanged context lines do not count
   - Path-level citations (line omitted or 0) are valid only if the file is in `changed_files`

2. **For each activity citation in every result:**
   - Confirm the `activity_entry_id` is a valid entry ID from the Activity Index (if provided)
   - Confirm the citation follows the upgrade-only rule:
     - Activity citations are valid only for behavioral criteria (e.g., "tests must pass", "must compile")
     - Activity citations cannot replace diff citations on implementation criteria
     - Activity citations cannot suppress a `not_satisfied` the diff supports
   - Confirm the citation is via `activity_entry_id` field, NOT via a path or other form

3. **For each result with non-satisfied status:**
   - Confirm `missing_evidence` is present and explains what evidence is absent
   - If citations exist, they must point to the evidence for *why* the criterion is not/partially satisfied

4. **JSON schema validation:**
   - All required fields are present (`id`, `status`, `rationale`, and citations/missing_evidence as needed)
   - No typos in status values (must be `satisfied`, `partially_satisfied`, `not_satisfied`, or `indeterminate`)
   - `bundle_id`, `contract_fingerprint`, `delivery_fingerprint` match the input exactly
   - Citations array is valid JSON with (`path` and optional `line`/`column`) OR `activity_entry_id` fields, but not the Activity Index

5. **Idempotence check:**
   - If you reviewed this bundle before, fingerprints will match previous results
   - Ensure the current assessment is identical to any prior assessment for the same bundle

**If any check fails, fix the assessment JSON and repeat 5a.** Then write the
file and run step 5b. Step 5a is not a substitute for `arm review validate`.

### 5b. Self-Validate with `arm review validate` (Mandatory)

Choose a **unique** path under `.armature/review/` for `$ASSESSMENT` —
include the issue id, a short bundle-id prefix, **and the reviewer token
the coordinator assigned** (`r1`, `r2`, … or your `ARM_LOG_SLOT`), for
example
`.armature/review/<issue-id>-<bundle-id-8>-<reviewer-token>.json`. Parallel
reviewers of the same issue and bundle use distinct tokens so they do not
overwrite each other; the coordinator unions their chat findings into one
list, then records each distinct path with `arm review record`. Do not reuse
`.armature/review/<issue-id>.json` or
`.armature/review/<issue-id>-<bundle-id-8>.json` across reviewers or
passes; confirmation must not overwrite the first-pass file, and a
second parallel reviewer must not overwrite the first's file the
coordinator still has as `$RESULT_FILE` context.

Write the drafted assessment to that path **once**. Retries in this step
rewrite **only** to apply `arm review validate` suggestions to the same
`$ASSESSMENT`. Do not keep a second draft to write again in step 6.

Then run the same checks `arm review record` performs — schema, criterion-ID
format, citation line-bounds, coverage, activity evidence — **without**
appending an op:

```bash
arm review validate --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE"
```

Both `--assessment` and `--bundle` are required file paths (or `-` for
assessment stdin). Do not pass JSON content as a flag value. `$BUNDLE_FILE`
is the ReviewBundle path the coordinator passed.

**Retry loop (inside the reviewer, not the coordinator):**

A non-zero exit means one of two different things, and only one of them is
retryable. Decide which **before** touching `$ASSESSMENT`.

Prefer `--format json` when classifying. Discriminate on the stdout
object's shape, then on whether a `valid: false` report is
assessment-fixable — not on "was anything printed":

- **Assessment-fixable report** — text mode prints `Assessment is
  invalid:` followed by `  - <message>` / `    suggestion: <fix>` lines;
  `--format json` prints an object carrying `valid` and `failures[]`.
  At least one `suggestion:` can be applied by rewriting `$ASSESSMENT`
  (ids, citations, statuses, `missing_evidence`, or copying fingerprints
  from the prepared bundle when that is the only fix). Retryable (case 2).
- **Bundle/setup report** — same `valid: false` envelope, but every
  `suggestion:` is to re-run `arm review prepare` (parse-bundle /
  decode-review-bundle, bundle integrity, an issue contract fingerprint
  mismatch, or any report whose only fix is regenerating the bundle).
  Copying `fingerprints.contract` from the prepared bundle cannot fix a
  stale issue contract — the assessment already matches that bundle, and
  a different fingerprint fails the earlier bundle check. **Not
  retryable.** Use case 3 (`Validation: error`); do not spend the retry
  cap on a bundle you are forbidden to edit.
- **Operational envelope** — `--format json` prints an object with
  `"error"` and no `valid` (`renderCommandFailure` writes `{"error":...}`
  to stdout; stderr is normally empty). Extract `.error.cause` as the
  Error line. Not retryable (case 3).

1. Exit 0 / `Assessment is valid` → proceed to step 6's **success** bounded
   chat. Rebuild rating and actionable findings from the final validated
   `$ASSESSMENT`. Do not write `$ASSESSMENT` again.
2. **Non-zero with an assessment-fixable report** → apply every
   `suggestion:` to `$ASSESSMENT` (rewrite ids, citations, fingerprints,
   statuses, or `missing_evidence` as directed). Do not edit the bundle.
   Then:
   1. Re-run the same command against the same `$ASSESSMENT` and
      `$BUNDLE_FILE`.
   2. Repeat until valid, at most 3 attempts after the first failure (four
      runs total: the initial validate plus three retries). If still
      invalid, stop and use step 6's **exhausted-retry** shape. Do not
      return the path as a completed assessment.
3. **Non-zero with a bundle/setup report or an operational envelope** →
   not a verdict on your assessment. Unreadable or missing
   `--assessment`/`--bundle`, `--bundle` handed JSON content instead of a
   path, a readable but malformed bundle, a bundle carrying no issue ID,
   a snapshot that will not load, or an issue ID absent from state.
   **Do not retry and do not spend retry attempts.** Stop immediately and
   use step 6's **operational-error** shape so the coordinator can repair
   the setup and re-dispatch.

For machine-readable failures (`valid`, `failures[].message`,
`failures[].suggestion`, or `{error:{cause:...}}`):

```bash
arm review validate --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE" --format json
```

Prefer this form when classifying:

- object with `valid` and assessment-fixable `failures[]` → case 2
- object with `valid: false` whose `suggestion:` is re-run
  `arm review prepare` (only fix, or an issue contract mismatch) → case 3
- object with `"error"` and no `valid` → case 3; Error line is `.error.cause`

This is the retry loop that used to land on the coordinator after
`arm review record` rejected the file. Keep it here. Operational failures
are the exception — those belong to the coordinator, which owns the bundle,
the paths, and issue state.

### 6. Return the ConformanceAssessment

Do **not** write or rewrite `$ASSESSMENT` here. Step 5b already chose the
unique path, wrote the JSON once, and either validated it or exhausted
retries. A second write would clobber a suggestion-fixed, validated file
with a stale pre-validate draft.

This file is a **local recording input**, not the durable record. `arm review record` writes a
compact `AssessmentAttestation` (fingerprints, rating, counts) to the
append-only log; it does not commit this JSON. Do **not** `git add` it
and do **not** call `arm review record` yourself. The coordinator
records each distinct **validated** path with
`arm review record --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE"`,
then uses the conservative path for loop control, so fingerprint
validation is bound to the exact bundle it dispatched.

**Bounded chat response (normative).** Return **exactly one** of the three
shapes below. Never more than one. Never extra prose. Never inline the full
assessment JSON. Never restate the bundle contents. Never `cat` the
assessment file into chat.

**Success — only after step 5b exits 0.** Derive the rating and the
actionable findings from the **final validated** `$ASSESSMENT` JSON,
including after every suggestion-driven rewrite (for example a
`lower the status` remedy). The coordinator builds `FINDINGS_FILE` from
this chat list; a pre-validate draft can drop a newly Yellow/Red
criterion. Your chat/text response contains **only**:

1. the rating (Green / Yellow / Red)
2. the actionable findings
3. the path to the assessment file

The coordinator records the file at the path you return — it does not treat
your chat text as the assessment JSON.

**Exhausted retries — only when step 5b is still invalid after the first
failure plus 3 attempts.** Your chat/text response contains **only**:

1. `Validation: failed`
2. the remaining `arm review validate` failures (each `message` and `suggestion`)
3. `Assessment: not returned`

Do not include a filesystem path the coordinator could pass to
`arm review record`. Do not invent a Green / Yellow / Red rating for a
file that did not validate.

**Operational error — Step 1 preflight failure, or step 5b case 3
(bundle/setup report or `{error:...}` envelope).** Your chat/text
response contains **only**:

1. `Validation: error`
2. the reason: Step 1 missing-field text; or `.error.cause` from the
   JSON envelope; or the setup report's `message` / `suggestion`
3. `Assessment: not returned`

Do not include a filesystem path the coordinator could pass to
`arm review record`. Do not invent a rating. Do not retry, and do not
report this as `Validation: failed` — that shape means your assessment was
judged and rejected, which did not happen here. The distinction tells the
coordinator whether to repair the setup and re-dispatch (this shape) or to
escalate an assessment that could not be made valid (the exhausted-retry
shape).

The two non-success shapes both end in `Assessment: not returned`, and in
neither case does a recordable path exist.

This keeps the coordinator's context free of duplicated JSON it can read
from disk, and keeps remediation dispatches (see the coordinator's bounded
review protocol) working from findings, not transcripts.

---

## Activity Evidence and the Upgrade-Only Rule

When the ReviewBundle includes an Activity Index (summary of execution evidence),
it provides behavioral context for the delivery. The Activity Index itself is never
citable — citations must reference **raw activity log entry IDs** only.

### Upgrade-Only Rule

Activity evidence can **lift** an indeterminate verdict on behavioral criteria only:
- Indeterminate → Satisfied (if evidence supports the criterion)
- Indeterminate → Partially Satisfied (if evidence partially supports the criterion)

Activity evidence **cannot**:
- Substitute for diff citations on implementation criteria (e.g., "code implements feature X").
  This applies to **both** `satisfied` and `partially_satisfied` on `definition_of_done`: if
  every citation on that criterion is activity-only (no diff citation present), `arm review
  record` rejects both statuses, not just `satisfied`.
- Suppress a `not_satisfied` that the diff supports (e.g., if the diff deletes necessary code, activity evidence of successful prior tests does not override this)
- Replace the requirement for concrete code evidence on contract implementation

### When to Reference Activity Evidence

**Valid uses (can cite raw entry IDs):**
- Build/test command exit status as behavioral evidence ("test suite passed, exit code 0")
- Build success for "must compile" criteria
- Test success for "tests must pass" criteria
- Lint pass for "must satisfy lint rules" criteria

**Invalid uses (do NOT cite the index):**
- Summarized counts or aggregate statistics from the Activity Index
- "See entry X in the index" — cite the raw entry ID instead
- Index as a substitute for diff review (diff review is always required)

### Citation Format for Activity Evidence

When citing activity evidence in a Citation object, use the `activity_entry_id` field:

```json
{
  "id": "acceptance[2]",
  "status": "satisfied",
  "rationale": "Test suite passed with exit code 0",
  "citations": [
    {
      "activity_entry_id": "0"
    }
  ]
}
```

**DO NOT cite the Activity Index itself:**
```json
// WRONG: Do not cite the index
{
  "path": "activity-index.json",
  "line": 15
}
```

### Why the Index is Never Citable

The Activity Index is a summary of the raw activity log. A reviewer who reads only
the index cannot verify:
- The full command line and exact options
- The complete output (which may be truncated in the index)
- The output hash (needed to verify integrity against later replay)

Citations must be verifiable against durable, complete evidence. Raw log entry IDs
are durable — the harness can look them up by ID and verify the entry's hash and
timestamp. The index is a **finding aid** — it helps reviewers navigate the log —
but it is not itself evidence.

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

Step 5b writes the ConformanceAssessment JSON once to a unique path
under `.armature/review/` and runs `arm review validate` with the retry
cap (at most 3 attempts after the first failure). Do not write that path
again after a successful validate. Then return the matching step 6 shape:
rating + findings (rebuilt from the final validated assessment) + path if
step 5b exited 0, the exhausted-retry shape if the cap was reached, or the
operational-error shape on setup failure. Do **not** call `arm review record` — that
is the coordinator's responsibility. The coordinator records each distinct
returned (validated) path with `--bundle "$BUNDLE_FILE"`, then uses the
conservative path for loop control, so fingerprint validation is bound to
the exact bundle it prepared.

**Example Workflow:**

```bash
# 1. Receive ReviewBundle file path (from coordinator)
# The coordinator passes: $BUNDLE_FILE

# 2. Review and evaluate; write the full assessment ONCE to a unique path
ASSESSMENT=".armature/review/TASK-42-e3b0c442-r1.json"
#    Parallel reviewers of the same bundle use distinct tokens (r1, r2, …).
#    Confirmation mode: if a findings-scope file was passed, evaluate
#    only those findings (do not start a new comprehensive review).
#    Do not write this path again in step 6 after validate succeeds.

# 3. Self-validate; apply assessment-fixable suggestions to the same
#    $ASSESSMENT and retry at most 3 times after the first failure (step 5b).
#    Retry ONLY for valid:false reports you can fix by rewriting the
#    assessment. A prepare-the-bundle suggestion or an {error:...} envelope
#    is operational — do not retry. Prefer --format json and read .error.cause.
arm review validate --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE" --format json

# 4. Chat response to the coordinator (not the JSON body):
#    After exit 0, rebuilt from the final validated $ASSESSMENT:
#    Rating: Green
#    Findings: (none)   # or the confirmation-scope results
#    Assessment: .armature/review/TASK-42-e3b0c442-r1.json
#    After exhausted retries:
#    Validation: failed
#    Failures: <remaining messages and suggestions>
#    Assessment: not returned
#    After an operational/setup failure (not retried), including Step 1:
#    Validation: error
#    Error: < .error.cause or setup suggestion, e.g. read bundle file: ... >
#    Assessment: not returned

# The coordinator consolidates parallel findings, then records EACH
# distinct path, then uses the conservative one for loop control:
# for f in "${RESULT_FILES[@]}"; do
#   arm review record --issue TASK-42 --assessment "$f" --bundle "$BUNDLE_FILE"
# done
# Reviewer does NOT call arm review record.
```

The durable record is the compact `AssessmentAttestation` on the issue
(fingerprints, rating, counts) — inspectable via materialized state (there
is no dedicated `arm review show`/`arm review list` today). The JSON file
under `.armature/review/` is the local input to `arm review record`, not a
git-native copy of the criterion results. Confirmation scope is the
findings list the coordinator passes, not a reread of that file.

---

## Validation and Idempotence

**Validation:**
```bash
arm review validate --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE"
# Apply each failure's suggestion, rewrite $ASSESSMENT, and re-run —
# at most 3 attempts after the first failure. If it still fails, use
# the exhausted-retry chat shape; do not return a recordable path.
# Only retry when the command printed an assessment-fixable report.
# If it exited non-zero with a prepare-the-bundle suggestion, an
# {error:...} envelope (.error.cause), or a Step 1 preflight miss,
# use the operational-error chat shape and do not retry.
# Do not write a second copy of a pre-validate draft after this
# command has already exited 0. arm review record remains the
# coordinator's enforcement gate; this command is read-only.
```

**Idempotence:**
- Fingerprints identify a bundle+results pair. If the coordinator records
  the same bundle twice with the same results, `arm review record` returns
  the same rating without duplicating the record.
- The reviewer never calls `arm review record`. If the review process is
  interrupted, re-run `arm review validate --assessment "$ASSESSMENT"
  --bundle "$BUNDLE_FILE"` (apply suggestions and retry per step 5b) and
  return the validated path — do not retry `arm review record`.

---

## Error Handling

### Invalid ReviewBundle

- Bundle fails `schema_version` check
- Missing required fields (issue.id, contract.definition_of_done, fingerprints)
- Cannot validate at Step 1, before `$ASSESSMENT` exists

**Action:** Return the operational-error shape (`Validation: error` /
reason / `Assessment: not returned`). Do not write `$ASSESSMENT`. Do not
retry as an assessment failure. The coordinator regenerates the bundle.

### Invalid ConformanceAssessment

`arm review validate` exits non-zero **and prints an assessment-fixable
report** (`Assessment is invalid:`, or `valid: false` with `failures[]`
whose suggestions rewrite `$ASSESSMENT`):

- Results array is empty
- Missing a criterion (e.g., no acceptance[1] when contract has 2 acceptance criteria)
- `bundle_id` does not match input
- Fingerprints do not match

**Action:** Apply each reported suggestion to the assessment JSON and re-run
`arm review validate --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE"`,
at most 3 attempts after the first failure. If the command exits 0, return
the success bounded-chat shape. If it is still invalid after the cap, return
the exhausted-retry shape. Do not return a recordable path until the
command exits 0.

### `arm review validate` operational failure

`arm review validate` exits non-zero **without an assessment-fixable
report** — either a `valid: false` report whose only `suggestion:` is to
re-run `arm review prepare`, or an object with `"error"` and no `valid`
(`.error.cause` on stdout under `--format json`):

- `--assessment` or `--bundle` missing, unreadable, or not a file path
- `--bundle` given JSON content instead of a path
- `$BUNDLE_FILE` is readable but malformed (parse-bundle / decode-review-bundle)
- Bundle carries no issue ID
- Snapshot will not load, or the issue ID is absent from state

**Action:** Do **not** apply assessment suggestions and do **not**
retry — rewriting `$ASSESSMENT` cannot fix the bundle. Do not
consume retry attempts. Return the operational-error shape from step 6 so
the coordinator can fix the bundle, the paths, or issue state and
re-dispatch. This is a setup failure, not a verdict on your assessment.

### `arm review record` is coordinator-owned

Do **not** call `arm review record`. Record failures (missing file,
malformed JSON, unknown issue ID) are the coordinator's to handle. Your
retry loop is only `arm review validate`.

---

## Command Reference

```bash
# Prepare a bundle (done by coordinator, not reviewer)
arm review prepare --issue TASK-42 --base abc123 --head def456 --output bundle.json

# Self-validate the drafted assessment (reviewer; required before return)
arm review validate --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE"

# Record an assessment (done by coordinator, not reviewer)
arm review record --issue TASK-42 --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"

# Show commits included in the bundle's diff range (done by coordinator)
arm review commits TASK-42 --branch task/TASK-42
```
