# Dogfood Findings Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address high and medium leverage dogfood findings by updating skill documentation and fixing the Python eval test fixture divergence problem.

**Architecture:** All changes are documentation/skill edits and one Python test change — no Go code changes, no new files except what's noted. Each task is independent and safe to implement in any order.

**Tech Stack:** Markdown skill files in `internal/skillsembed/skills/`, Python test in `scripts/`

## Global Constraints

- `internal/skillsembed/skills/<name>/SKILL.md` is the canonical source. `.claude/skills/<name>/SKILL.md` (and the `.gemini/` and `.codex/` equivalents) are deployed copies produced by `arm bootstrap` / `make bootstrap` — never edit them directly; edit the `internal/skillsembed/skills/` copy and re-run bootstrap to refresh the deployed copies if needed for local testing.
- Never use placeholder language ("TBD", "TODO", etc.) in skill text — write the exact wording.
- Run `make validate-skills` after any skill change to confirm the skill passes validation.
- Run `python3 scripts/test_reviewer_eval_report.py` after the Python test change.

---

## File Structure

**Modified:**
- `internal/skillsembed/skills/armature-coordinator/SKILL.md` — Tasks 1, 2, 3
- `internal/skillsembed/skills/armature-planner/SKILL.md` — Task 4
- `internal/skillsembed/skills/armature-worker/SKILL.md` — Tasks 5, 6
- `internal/skillsembed/skills/armature-auditor/SKILL.md` — Task 7
- `internal/skillsembed/skills/armature-reviewer/SKILL.md` — Task 8
- `scripts/test_reviewer_eval_report.py` — Task 9

**Created (heavy reference, kept out of frequently-loaded SKILL.md bodies):**
- `internal/skillsembed/skills/armature-worker/examples/json-roundtrip-test.go` — Task 6
- `internal/skillsembed/skills/armature-reviewer/templates/conformance-assessment.json` — Task 8
- `internal/skillsembed/skills/armature-reviewer/references/field-rules.md` — Task 8

## Progressive Disclosure Note

`armature-worker` and `armature-reviewer` are loaded on every task/review dispatch, so
their SKILL.md bodies are token-budget-sensitive (see superpowers:writing-skills, Token
Efficiency). Where a finding's fix requires a large code example or a verbatim template,
the example/template goes in a separate file under the skill's own directory and SKILL.md
holds only the rule plus a one-line pointer to that file. Do not use `@`-style force-load
links — reference the file by relative path so the agent reads it only when it needs the
detail (e.g., "See `examples/json-roundtrip-test.go` in this skill directory.").

---

### Task 1: Add `--worktree` to coordinator `arm claim` examples

**Finding:** `arm claim --worktree` is required on every invocation but the coordinator skill examples show `arm claim TASK-ID --ttl <minutes>` without it. Every wave dispatch fails until the coordinator reads `arm claim --help`.

**Files:**
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md`

- [ ] **Step 1: Locate the claim examples**

Open `internal/skillsembed/skills/armature-coordinator/SKILL.md`. Search for `arm claim TASK-ID`. You will find it in Step 4 (Dispatch Workers), near:
```
arm claim TASK-ID --ttl <minutes>
```

- [ ] **Step 2: Add `--worktree` to the claim command in Step 4**

Find the paragraph in Step 4 that begins "1. Claim and get context for each task:" and replace the claim command:

Old:
```bash
arm claim TASK-ID --ttl <minutes>
arm render-context TASK-ID --format agent
```

New:
```bash
arm claim TASK-ID --worktree <path> --ttl <minutes>
arm render-context TASK-ID --format agent
```

Where `<path>` is the absolute path to the git worktree created for this worker (e.g., `.worktrees/TASK-ID`). Add a note immediately after the code block:

```
> **`--worktree` is required.** Pass the absolute path to the worker's git worktree.
> Omitting it causes `arm claim` to fail with a missing-worktree error.
> Create the worktree before claiming: `git worktree add .worktrees/TASK-ID HEAD`
```

- [ ] **Step 3: Find any other `arm claim` invocations in the file and add `--worktree`**

Run:
```bash
grep -n "arm claim" internal/skillsembed/skills/armature-coordinator/SKILL.md
```

For each hit that shows a bare `arm claim TASK-ID` without `--worktree`, add the flag. The pre-claim step in the Parallel Dispatch section is a common second location.

- [ ] **Step 4: Validate**

```bash
make validate-skills
```

Expected: no errors mentioning `armature-coordinator`.

- [ ] **Step 5: Commit**

```bash
git add internal/skillsembed/skills/armature-coordinator/SKILL.md
git commit -m "docs(coordinator): add --worktree to arm claim examples"
```

---

### Task 2: Fix `arm review prepare` / `arm review record` example to use `--output` + file path

**Finding:** The coordinator SKILL.md shows `--bundle "$REVIEW_BUNDLE"` where `$REVIEW_BUNDLE` is the stdout JSON. The CLI calls `os.ReadFile()` on the value, so passing JSON content fails with a cryptic file-not-found error. The correct approach is `arm review prepare --output <file>` then pass the file path to `--bundle`.

**Files:**
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md`

The current skill already uses `BUNDLE_FILE=$(mktemp)` and `--output "$BUNDLE_FILE"` in the Step a.2 reviewer dispatch section. The finding may refer to an older version or a secondary example. Verify first, then patch any remaining instances.

- [ ] **Step 1: Search for any remaining incorrect usage**

```bash
grep -n "REVIEW_BUNDLE\|--bundle.*\$REVIEW_BUNDLE\|review prepare" internal/skillsembed/skills/armature-coordinator/SKILL.md
```

If you find any line that passes raw JSON content (not a file path) to `--bundle`, proceed to step 2. If no such line exists, this task is already complete — skip to step 4 (validate and skip commit).

- [ ] **Step 2: Replace any incorrect `--bundle` usage**

If found, replace the pattern of:
```bash
REVIEW_BUNDLE=$(arm review prepare ...)
arm review record --bundle "$REVIEW_BUNDLE"
```

With:
```bash
BUNDLE_FILE=$(mktemp)
arm review prepare ... --output "$BUNDLE_FILE"
arm review record --bundle "$BUNDLE_FILE"
```

Add a comment above the `arm review prepare` line:
```bash
# --output writes the bundle to a file. Pass the FILE PATH to --bundle, not the JSON content.
# arm review record calls os.ReadFile() on the --bundle value.
```

- [ ] **Step 3: Check the Command Reference section at the bottom of the skill**

Find the "Command Reference" or similar section with example invocations. Verify that any `arm review record` example shows `--bundle "$BUNDLE_FILE"` (file path), not inline JSON.

- [ ] **Step 4: Validate**

```bash
make validate-skills
```

- [ ] **Step 5: Commit (only if changes were made)**

```bash
git add internal/skillsembed/skills/armature-coordinator/SKILL.md
git commit -m "docs(coordinator): clarify arm review prepare --output / --bundle file path usage"
```

---

### Task 3: Add parallel branch overlap warning to coordinator skill

**Finding:** Parallel branches touching the same files silently reverted each other's changes with a clean git merge (no conflict markers). The coordinator skill has no guidance to diff overlapping files after merging.

**Files:**
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md`

- [ ] **Step 1: Find the merge/integration section**

Search for the section headed "After Workers Return" or "Check for scope conflicts and merge conflicts" (it is step b in the wave checklist). It currently reads:

```
### b. Check for scope conflicts and merge conflicts

If workers operated in separate git worktrees or branches, merge them into the
story feature branch now. Resolve any conflicts before proceeding.
```

- [ ] **Step 2: Add the semantic overlap audit**

Replace section b with:

```markdown
### b. Check for scope conflicts and merge conflicts

If workers operated in separate git worktrees or branches, merge them into the
story feature branch now. Resolve any conflicts before proceeding.

**Semantic revert audit (parallel waves only):** Git's line-level merge can cleanly
combine two branches that semantically cancel each other — one adds a function, another
removes it in a refactor, and the merge picks the deletion. After merging parallel
branches, run a diff audit on any files touched by more than one task:

```bash
# Find files changed by multiple tasks in this wave
git diff --name-only "$WAVE_BASE_SHA"..HEAD | sort > /tmp/wave_files.txt

# For any file you expected to be modified, verify the result looks correct
git diff "$WAVE_BASE_SHA"..HEAD -- <overlapping-file>
```

Do not rely solely on a clean `git merge` exit code. If any file was in scope for
two or more tasks, read its final diff and confirm both tasks' changes are present.
```

- [ ] **Step 3: Add to Common Failure Modes table**

Find the "Common Failure Modes" table at the bottom of the skill. Add a row:

```markdown
| Parallel branch semantic revert | Two branches touch the same file; git merges cleanly but one task's change is silently dropped | After merging parallel branches, run `git diff "$WAVE_BASE_SHA"..HEAD -- <file>` on every file touched by >1 task; confirm all changes are present |
```

- [ ] **Step 4: Validate**

```bash
make validate-skills
```

- [ ] **Step 5: Commit**

```bash
git add internal/skillsembed/skills/armature-coordinator/SKILL.md
git commit -m "docs(coordinator): add semantic revert audit for parallel branch merges"
```

---

### Task 4: Add `context_files` warning as decomposition signal in planner skill

**Finding:** When `arm validate --ci` emits `context_files` WARNINGs, the planner dismissed them as noise. T5 genuinely spanned 3 domain modules and should have been split. The planner skill's "Common Failure Modes" table doesn't mention these warnings as a decomposition signal.

**Files:**
- Modify: `internal/skillsembed/skills/armature-planner/SKILL.md`

- [ ] **Step 1: Find the Common Failure Modes table**

Search for "Common Failure Modes" in `internal/skillsembed/skills/armature-planner/SKILL.md`. The table has columns `Failure`, `Symptom`, `Fix`.

- [ ] **Step 2: Add a row for `context_files` warnings**

Add this row to the table:

```markdown
| `context_files` WARNINGs from `arm validate` | Planner rationalizes them as noise; auditor later promotes them to errors with `--strict` | Each WARNING is a decomposition signal: inspect the task's scope. If it spans multiple domain modules or >4 files, split the task. Run `arm amend --context-file` only after confirming the scope is genuinely focused. |
```

- [ ] **Step 3: Find the `arm validate` step in the planner workflow (near the release gate)**

Search for the section that runs `arm validate` and `arm validate --ci`. It likely shows:

```bash
arm validate --ci   # must exit 0 with no ERRORs; scope overlaps resolved
```

Add a sentence immediately after:

```
**Treat `context_files` WARNINGs as decomposition prompts, not presentation noise.**
Each WARNING means a task scope spans multiple domain modules. Inspect the task and split
it before `--strict` is added at audit time and turns these warnings into blocking errors.
```

- [ ] **Step 4: Validate**

```bash
make validate-skills
```

- [ ] **Step 5: Commit**

```bash
git add internal/skillsembed/skills/armature-planner/SKILL.md
git commit -m "docs(planner): treat context_files warnings as decomposition signals"
```

---

### Task 5: Document `_REQ_<TASK-ID>` naming convention in worker skill

**Finding:** The `_REQ_<TASK-ID>` traceability convention exists in the planner skill but not the worker skill. A worker wrote correct tests with descriptive names and was rated red because reviewer expected `TestDeriveRating_REQ_SMTC-S1-T1` naming. The worker never saw the planner skill.

**Files:**
- Modify: `internal/skillsembed/skills/armature-worker/SKILL.md`

- [ ] **Step 1: Find the Pre-Transition Verification section**

Search for "Pre-Transition Verification" in `internal/skillsembed/skills/armature-worker/SKILL.md`. It currently describes `go build ./...` and `make check` but says nothing about test naming.

- [ ] **Step 2: Add test naming convention as a cross-reference, not a re-explanation**

The `_REQ_<TASK-ID>` convention is already documented in `armature-planner/SKILL.md`.
Duplicating its full rationale here would drift the two copies out of sync the next time
either is edited. Add only the testable rule, the pattern, and a pointer — not a second
explanation:

Add a new subsection immediately after the `make check` block (before "Completion order"):

```markdown
**Test naming convention (`_REQ_` suffix):** Tests written to satisfy an acceptance
criterion must be named `Test<Description>_REQ_<ISSUE-ID>` exactly as the criterion
spec states (e.g. `TestDeriveRating_REQ_SMTC-S1-T1`). `make trace-report` and the
reviewer both key off this exact name — a correct test under a descriptive-only name
(e.g. `TestDeriveRating_AllSatisfied_Green`) is rated red. See
`armature-planner/SKILL.md` for the full convention.
```

- [ ] **Step 3: Add a row to Common Mistakes table**

Find the "Common Mistakes" table at the bottom of the worker skill. Add:

```markdown
| Writing tests without `_REQ_<ISSUE-ID>` suffix | Reviewer rates criterion red even if test logic is correct; `make trace-report` shows gap | Name tests `Test<Description>_REQ_<ISSUE-ID>` to match the acceptance criterion spec |
```

- [ ] **Step 4: Validate**

```bash
make validate-skills
```

- [ ] **Step 5: Commit**

```bash
git add internal/skillsembed/skills/armature-worker/SKILL.md
git commit -m "docs(worker): document _REQ_ test naming convention"
```

---

### Task 6: Add JSON round-trip fixture requirement to worker DoD

**Finding:** A JSON string/int type mismatch between skill docs and Go types was hidden by struct-only tests. All tests were green while the end-to-end reviewer→`arm review record` flow was broken. The fix: require a round-trip JSON fixture test whenever a task both adds a Go type and documents that type in a skill.

**Files:**
- Modify: `internal/skillsembed/skills/armature-worker/SKILL.md`

- [ ] **Step 1: Find Step 5 (Pre-Transition Verification)**

In the worker skill, locate the "Pre-Transition Verification (mandatory)" section.

- [ ] **Step 2a: Create the reusable example file**

Create `internal/skillsembed/skills/armature-worker/examples/json-roundtrip-test.go`:

```go
// Example: round-trip JSON fixture test for a Go type documented in a skill or CONTEXT.md.
// Adapt the type, field, and expected JSON form to your task. This test fails immediately
// if the type uses integer marshaling while the skill/CONTEXT.md documents strings (or vice versa).
func TestCriterionStatus_JSONRoundTrip_REQ_<ISSUE-ID>(t *testing.T) {
    raw := `{"status":"satisfied"}`
    var result CriterionResult
    if err := json.Unmarshal([]byte(raw), &result); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if result.Status != StatusSatisfied {
        t.Errorf("got %v, want StatusSatisfied", result.Status)
    }
    out, _ := json.Marshal(result)
    if !strings.Contains(string(out), `"satisfied"`) {
        t.Errorf("marshal produced %s, want string form", out)
    }
}
```

- [ ] **Step 2b: Add the rule to SKILL.md as a short pointer, not an inline copy**

Add a new bullet after the `make check` requirement. Keep this to the rule and a file
pointer — the worker skill loads on every dispatch, so the example stays in its own file
and is read only when a worker needs it:

```markdown
**Cross-layer JSON fixture test (when applicable):** If your task both adds or modifies
a Go type AND documents that type's JSON format in a skill or CONTEXT.md (field names,
value format, string vs integer), add at least one test that exercises the serialization
path — not just struct construction. A test using only Go struct literals never exercises
`MarshalJSON`/`UnmarshalJSON` and cannot catch a mismatch between a documented string value
and an integer enum representation. See `examples/json-roundtrip-test.go` in this skill
directory for a worked example to adapt.
```

- [ ] **Step 3: Add a row to Common Mistakes**

```markdown
| Struct-only tests when task touches a JSON-documented Go type | Tests are green but end-to-end serialization is broken; reviewer agent submits `"status":"satisfied"` but Go unmarshals 0 | Add a round-trip JSON fixture test (unmarshal from string, assert Go value; marshal Go value, assert string form) |
```

- [ ] **Step 4: Validate**

```bash
make validate-skills
```

- [ ] **Step 5: Commit**

```bash
git add internal/skillsembed/skills/armature-worker/SKILL.md internal/skillsembed/skills/armature-worker/examples/json-roundtrip-test.go
git commit -m "docs(worker): require JSON round-trip fixture test for typed skill docs"
```

---

### Task 7: Clarify `arm validate --strict` scope in auditor skill (context_files category)

**Finding:** Auditor Step 4 frames `--strict` as checking scope overlap. It also upgrades "missing `context_files` on broad scope" warnings to errors — a different category that requires `arm amend --context-file` fixes. This wasn't documented, so auditors were surprised when `--strict` blocked on what looked like already-resolved issues.

**Files:**
- Modify: `internal/skillsembed/skills/armature-auditor/SKILL.md`

- [ ] **Step 1: Find Step 4 — Scope Overlap Resolution**

In the auditor skill, find the section "Step 4 — Scope Overlap Resolution". It currently describes `arm validate --strict` only in the context of scope overlap WARNINGs.

- [ ] **Step 2: Expand the description of `--strict`**

After the existing description of scope overlap, add:

```markdown
**`--strict` also upgrades `context_files` WARNINGs to errors.** These are separate from
scope overlap warnings. A task triggers a `context_files` WARNING when its scope is
broad (e.g., covers 3+ domain modules) and the task record lacks a `context_files`
field. Under `--strict`, each such WARNING becomes a blocking error requiring:

```bash
arm amend ISSUE-ID --context-file path/to/relevant/doc.md
```

Run `arm validate --strict` early (before story completion) so these errors surface
while workers are still available. Discovering them at audit time requires retroactive
`arm amend` calls across multiple issues before the story can pass.
```

- [ ] **Step 3: Update the Step 4 table (if one exists) or the common failure modes**

Find the summary table in the auditor skill (if any) that lists what each validation step checks. Add `context_files` as a second check for Step 4:

| Check | Command | Pass Condition |
|---|---|---|
| Scope overlap | `arm validate --strict` | Zero scope overlap warnings |
| Broad scope without context_files | `arm validate --strict` | Zero context_files warnings (each requires `arm amend --context-file` to resolve) |

- [ ] **Step 4: Validate**

```bash
make validate-skills
```

- [ ] **Step 5: Commit**

```bash
git add internal/skillsembed/skills/armature-auditor/SKILL.md
git commit -m "docs(auditor): document context_files category in --strict upgrade"
```

---

### Task 8: Add citation self-validation and verbatim JSON template to reviewer skill

**Finding 1:** Reviewer agents cite specific line numbers without verifying them against diff hunk `@@` headers. New files are especially prone — the model invents a plausible line that exceeds the actual diff length, and `arm review record` rejects the assessment.

**Finding 2:** Haiku-tier models given a prose description of the ConformanceAssessment schema produce structurally invalid JSON (wrong field names, missing top-level fields, wrong criterion ID format). Sonnet with a verbatim JSON template produces valid JSON on first attempt.

**Files:**
- Modify: `internal/skillsembed/skills/armature-reviewer/SKILL.md`

- [ ] **Step 1: Add line-range verification rule to Step 3 (Evaluate Each Acceptance Criterion)**

In the reviewer skill, find "Step 3: Evaluate Each Acceptance Criterion". Add a note before the JSON example block:

```markdown
**Line citation verification (mandatory before citing any line number):** Before including
`"line": N` in any citation, confirm that `N` falls within a `@@` hunk in the diff for
that file. For new files, the diff hunk header is `@@ -0,0 +1,M @@` — valid lines are
1 through M. For modified files, check each `@@ -A,B +C,D @@` header: valid lines in
the new version are C through C+D-1.

When uncertain, use a **path-level citation** by omitting `line`:
```json
{"path": "pkg/parser/parser.go"}
```
A path-level citation validates against file presence in `changed_files`, not a line range.
It is always safe. Prefer it over a guessed line number.
```

- [ ] **Step 2a: Create the verbatim JSON template as its own file**

Create `internal/skillsembed/skills/armature-reviewer/templates/conformance-assessment.json`:

```json
{
  "schema_version": 1,
  "bundle_id": "REPLACE_WITH_bundle_id_FROM_INPUT",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "REPLACE with rationale",
      "citations": [
        {"path": "pkg/example/example.go", "line": 42}
      ]
    },
    {
      "id": "acceptance[0]",
      "status": "satisfied",
      "rationale": "REPLACE with rationale",
      "citations": [
        {"path": "pkg/example/example.go"}
      ]
    }
  ],
  "contract_fingerprint": "REPLACE_WITH_fingerprints.contract_FROM_INPUT",
  "delivery_fingerprint": "REPLACE_WITH_fingerprints.delivery_FROM_INPUT"
}
```

- [ ] **Step 2b: Create the field rules reference as its own file**

Create `internal/skillsembed/skills/armature-reviewer/references/field-rules.md`:

```markdown
# ConformanceAssessment Field Rules

- `schema_version`: must be `1` (integer, not string)
- `bundle_id`: must match the input bundle's `bundle_id` exactly
- `results[*].id`: use `"definition_of_done"` for DoD; use `"acceptance[0]"`, `"acceptance[1]"` etc. for criteria (bracket notation, zero-indexed)
- `results[*].status`: one of `"satisfied"`, `"partially_satisfied"`, `"not_satisfied"`, `"indeterminate"`
- `citations[*].path`: must be a path from `delivery.changed_files` in the bundle
- `citations[*].line`: optional; omit (or use path-level citation) when uncertain of line range
- `contract_fingerprint` / `delivery_fingerprint`: copy verbatim from `fingerprints.contract` / `fingerprints.delivery` in the input bundle
```

- [ ] **Step 2c: Point to both files from Step 5 (Produce ConformanceAssessment JSON) in SKILL.md**

Find "Step 5: Produce ConformanceAssessment JSON". After the prose description, add a
short pointer instead of inlining the template — the reviewer skill loads on every
review dispatch, and Haiku-tier models need the template verbatim but don't need it
duplicated in the skill body to find it:

```markdown
**Copy-paste the template exactly.** Use `templates/conformance-assessment.json` in this
skill directory as your starting point; fill in the values and do not rename any field.
Field-by-field rules (status enum, ID format, fingerprint sourcing) are in
`references/field-rules.md` in this skill directory. Schema errors (wrong field names,
missing top-level fields, wrong criterion ID format) will cause `arm review record` to
reject the assessment.
```

- [ ] **Step 3: Add a self-check step before "Return the ConformanceAssessment"**

Between Step 5 and Step 6 (Return), add:

```markdown
### 5a. Self-Check Before Returning

Before returning the assessment, verify:

1. `schema_version` is present and equals `1`
2. `bundle_id` matches the input bundle's `bundle_id`
3. `results` contains exactly one entry per criterion: one `definition_of_done` plus one `acceptance[N]` for each acceptance criterion (zero-indexed, bracket notation)
4. Every `citations[*].path` appears in the bundle's `delivery.changed_files` list
5. Every `citations[*].line` (if present) falls within a `@@` hunk of the diff for that file; replace any uncertain line citation with a path-level citation
6. `contract_fingerprint` and `delivery_fingerprint` match the input fingerprints exactly

If any check fails, fix the assessment JSON before returning it. Do not return an
assessment that will fail `arm review record` and require coordinator patching.
```

- [ ] **Step 4: Validate**

```bash
make validate-skills
```

- [ ] **Step 5: Commit**

```bash
git add internal/skillsembed/skills/armature-reviewer/SKILL.md internal/skillsembed/skills/armature-reviewer/templates/conformance-assessment.json internal/skillsembed/skills/armature-reviewer/references/field-rules.md
git commit -m "docs(reviewer): add line-range self-validation and verbatim JSON template"
```

---

### Task 9: Replace inline Python eval fixtures with canonical file loads

**Finding:** `scripts/test_reviewer_eval_report.py` maintains inline `setUp()` copies of the eval cases and expected results. When the canonical `internal/review/testdata/evals/cases.json` or `scripts/testdata/reviewer_eval_results.json` changes, the inline copies silently diverge. Tests pass because they validate the inline copy against itself.

**Files:**
- Modify: `scripts/test_reviewer_eval_report.py`

- [ ] **Step 1: Understand the current structure**

Read `scripts/test_reviewer_eval_report.py`. The `setUp()` method defines:
- `self.cases` — a list of dicts with `id`, `expected_rating`, `expected_statuses`
- `self.perfect_results` — a list of dicts with `case_id`, `rating`, `statuses`

The canonical sources for these are:
- `internal/review/testdata/evals/cases.json` — full case records; each has `id`, `expected_rating`, `expected_statuses`, plus bundle data
- `scripts/testdata/reviewer_eval_results.json` — list of `{case_id, rating, statuses}`

The tests use `self.cases` and `self.perfect_results` as inputs to `compute_metrics()`.

- [ ] **Step 2: Add imports and a loader at the top of the file**

After the existing imports, add:

```python
import json
from pathlib import Path

SCRIPT_DIR = Path(__file__).parent
CASES_FILE = SCRIPT_DIR.parent / "internal" / "review" / "testdata" / "evals" / "cases.json"
RESULTS_FILE = SCRIPT_DIR / "testdata" / "reviewer_eval_results.json"
```

- [ ] **Step 3: Replace `setUp()` with canonical file loads**

Replace the entire `setUp()` method body with:

```python
def setUp(self):
    """Load fixtures from canonical source files."""
    with open(CASES_FILE) as f:
        raw_cases = json.load(f)
    # compute_metrics() expects dicts with id, expected_rating, expected_statuses
    self.cases = [
        {
            "id": c["id"],
            "expected_rating": c["expected_rating"],
            "expected_statuses": c["expected_statuses"],
        }
        for c in raw_cases
    ]

    with open(RESULTS_FILE) as f:
        self.perfect_results = json.load(f)
```

- [ ] **Step 4: Update test comments to match the canonical case count**

The tests have inline comments counting criteria (e.g., "case-001: 1 (AC1) … Total: 13 criteria"). These counts were derived from the inline fixture. After loading from the canonical file, these counts may differ. Update the comments in `test_status_accuracy_metric` and similar tests to say:

```python
# Counts derived from cases.json at test time; see canonical file for current values.
```

Or, if the tests assert a specific value (e.g., `assertEqual(metrics['status_accuracy'], 1.0)`), those assertions still hold since `perfect_results` still perfectly matches `cases` — the assertion itself is correct, only the comment is stale.

- [ ] **Step 5: Run the tests**

```bash
python3 scripts/test_reviewer_eval_report.py -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add scripts/test_reviewer_eval_report.py
git commit -m "test(eval): load eval fixtures from canonical JSON files instead of inline copies"
```

---

### Task 11: Fix `armature-planner` plan JSON example to include the required `version`/`title` wrapper

**Finding:** The "Complete Well-Formed Task Example" in `armature-planner/SKILL.md` shows
only a single issue object (`{"id": "STORY-T1", ...}`), not wrapped in the top-level shape
`arm decompose-apply` actually requires: `{"version": 1, "title": "...", "issues": [...]}`.
A planner who copies the example literally gets `Error: unsupported plan version: 0` on
`--dry-run`, with no clue from the error message or the skill text about what `version`
means or that it's required. This has recurred across multiple planning sessions — the
skill tells planners to run `arm decompose-apply --example` to inspect the schema, but the
"complete" example in the skill body reads as authoritative on its own and doesn't
cross-reference `--example` at the point of use.

**Files:**
- Modify: `internal/skillsembed/skills/armature-planner/SKILL.md`

- [ ] **Step 1: Locate the Complete Well-Formed Task Example**

Search for `Complete Well-Formed Task Example` in `internal/skillsembed/skills/armature-planner/SKILL.md`.
It currently shows a single task object starting with `{"id": "STORY-T1", ...}` and ending
with the closing `}` of that object — no top-level `version`, `title`, or `issues` wrapper.

- [ ] **Step 2: Wrap the example in the real top-level plan shape**

Replace the JSON block so it demonstrates a complete, valid plan file rather than a bare
task object. Old:

```json
{
  "id": "STORY-T1",
  "title": "Add token parser",
  "type": "task",
  ...
}
```

New:

```json
{
  "version": 1,
  "title": "Add token parser story",
  "issues": [
    {
      "id": "STORY-T1",
      "title": "Add token parser",
      "type": "task",
      "parent": "STORY-ID",
      "priority": "high",
      "blocked_by": [],
      "dod": "Parser handles all five token types from spec §3.2. Returns typed AST nodes. All existing tests pass; new tests cover added branches.",
      "scope": "cmd/parse/main.go, internal/ast/node.go (new), internal/ast/node_test.go (new)",
      "acceptance": [
        "TestParseTokenTypes_REQ_STORY_T1 passes",
        "TestParseEdgeCases_REQ_STORY_T1 passes",
        "make check green",
        "no new lint errors"
      ]
    }
  ]
}
```

- [ ] **Step 3: Add an explicit warning immediately above the example**

```markdown
> **Every plan file needs the top-level wrapper.** `arm decompose-apply` requires
> `{"version": 1, "title": "...", "issues": [...]}` at the top level. A file containing
> only a bare issue object (or an `issues` array without `version`) fails with
> `Error: unsupported plan version: 0`. When unsure, run `arm decompose-apply --example`
> to print the current schema before writing a plan file from scratch.
```

- [ ] **Step 4: Add a row to the Anti-Patterns table**

Find the "Anti-Patterns to Avoid" table and add:

```markdown
| Plan file missing top-level `version`/`title`/`issues` wrapper | `arm decompose-apply` fails with `unsupported plan version: 0` | Wrap all issues in `{"version": 1, "title": "...", "issues": [...]}` |
```

- [ ] **Step 5: Validate**

```bash
make validate-skills
```

- [ ] **Step 6: Commit**

```bash
git add internal/skillsembed/skills/armature-planner/SKILL.md
git commit -m "docs(planner): wrap plan JSON example in required version/title/issues shape"
```

---

### Task 10: Audit all `internal/skillsembed/skills/*/SKILL.md` for progressive disclosure (review only — do not implement)

**Goal:** This task is scoping, not remediation. Produce a follow-up task list (as a new
plan or new tasks appended here) identifying where the deployed skills violate the
superpowers:writing-skills token-efficiency and progressive-disclosure guidance applied
in Tasks 5, 6, and 8 above. Do not edit any SKILL.md as part of this task.

**Why this matters:** `armature-coordinator`, `armature-worker`, and `armature-reviewer`
load on every wave dispatch, every task claim, and every review respectively — they are
exactly the "frequently-loaded skill" category the writing-skills token-efficiency
guidance targets (<200 words total recommended; current sizes are far over). Heavy
reference content (worked examples, verbatim templates, full command tables) belongs in
separate files the skill points to, not inlined into the body that loads every time.

**Current word counts** (via `wc -w` on each `SKILL.md`, measured 2026-06-30):

| Skill | Words | Loaded on |
|---|---|---|
| `armature-coordinator` | 3373 | every wave dispatch |
| `armature-planner` | 2105 | every decomposition/release session |
| `armature-reviewer` | 1384 | every review dispatch |
| `armature-worker` | 1260 | every task claim |
| `armature-auditor` | 1122 | every pre-merge audit |
| `armature` (quick reference) | 331 | frequently, as a lookup |

All are well over the <500-word target for "other skills," and the top three are
hot-path skills that should be closer to the <200-word frequently-loaded target.

**Files:**
- Read-only: `internal/skillsembed/skills/armature-coordinator/SKILL.md`
- Read-only: `internal/skillsembed/skills/armature-planner/SKILL.md`
- Read-only: `internal/skillsembed/skills/armature-reviewer/SKILL.md`
- Read-only: `internal/skillsembed/skills/armature-worker/SKILL.md`
- Read-only: `internal/skillsembed/skills/armature-auditor/SKILL.md`
- Output: a new plan file or a new task list (not yet created) enumerating extraction candidates

- [ ] **Step 1: Catalog every fenced code/JSON block over ~15 lines in each SKILL.md**

For each skill, run:
```bash
grep -n '^```' internal/skillsembed/skills/<skill>/SKILL.md
```
and identify blocks whose start/end line delta exceeds ~15 lines. These are extraction
candidates — heavy reference material that a future task should move to a sibling file
(`examples/`, `templates/`, or `references/` under the skill's own directory, following
the pattern established in Tasks 6 and 8) with a short pointer left in SKILL.md.

Known candidates already spotted in this review (verify line ranges before filing, the
file may have shifted):
- `armature-coordinator/SKILL.md` — "Dispatch Protocol" section (~56 lines) and
  "Semantic Review (Reviewer Dispatch)" section (~126 lines, the largest single block in
  any of these skills) are both prose+code mixes dense enough to warrant a
  `references/` extraction with a pointer left in the Step-by-Step flow.
- `armature-planner/SKILL.md` — "Writing Good Plan JSON" section (~93 lines, includes a
  full "Complete Well-Formed Task Example" JSON block) is reusable reference material,
  not a decision an agent makes differently each time — candidate for
  `templates/well-formed-task.json` plus a trimmed prose section.
- `armature-reviewer/SKILL.md` — "Input: ReviewBundle" section's "ReviewBundle Example"
  (~47 lines of JSON) is a worked example, not a decision; candidate for
  `examples/review-bundle.json` referenced from Step 1 of the review process. (Note:
  this is in addition to the `templates/conformance-assessment.json` and
  `references/field-rules.md` extractions already planned in Task 8 above — both
  examples in this skill should land in the same `examples/`/`templates/` directories
  rather than one inline and one extracted.)

- [ ] **Step 2: Check description fields against SDO guidance**

For each skill's frontmatter `description`, confirm it states only triggering
conditions ("Use when...") and does not summarize the skill's internal workflow — per
the writing-skills SDO section, a description that summarizes process becomes a shortcut
agents take instead of reading the full skill, which is especially costly for
discipline-style steps like the auditor's pre-merge gate or the coordinator's wave
checklist.

- [ ] **Step 3: Check for duplicated content across skills**

Grep across all five skills for repeated prose blocks (e.g., the `_REQ_<TASK-ID>`
convention duplicated between planner and worker before Task 5's fix, or the DAG Hygiene
Mandate block that appears near-identically in coordinator, planner, worker, and
auditor). Flag any duplication beyond the DAG Hygiene Mandate (which may be an
intentional repeated guardrail — confirm with the user before proposing its extraction)
as a cross-reference candidate, following the Task 5 pattern of "rule + pointer to one
canonical source" instead of N copies.

- [ ] **Step 4: Write the findings as a new plan**

Produce `docs/superpowers/plans/<date>-skillsembed-progressive-disclosure.md` listing
each extraction candidate as its own task (file to create, file to extract from, exact
old/new text), in the same format as Tasks 5, 6, and 8 of this plan. Do not implement any
extraction as part of this task — filing the plan is the deliverable.

No commit step — this task produces a plan file only, reviewed before any SKILL.md is
touched.
