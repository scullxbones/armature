# Semantic Conformance Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an advisory, skill-driven LLM review protocol that evaluates completed Armature deliveries against Definition of Done and Acceptance, records compact attestations, and renders detailed PR evidence.

**Architecture:** A new pure `internal/review` module owns bundle construction, fingerprints, result validation, rating derivation, attestations, and rendering. Cobra commands adapt materialized Issues and Git ranges into that module; a bundled `armature-reviewer` Skill performs the subjective judgment, while coordinator and auditor Skills own orchestration and deterministic governance respectively. Armature never runs customer checks or a model.

**Tech Stack:** Go 1.23+, Cobra, append-only Armature ops/materialization, embedded Markdown Skills, Python 3 standard library for development-only evaluator metrics.

---

## File and module map

- `internal/review/types.go` — versioned protocol types and enums.
- `internal/review/rating.go` — deterministic traffic-light derivation.
- `internal/review/fingerprint.go` — canonical SHA-256 fingerprints.
- `internal/review/prepare.go` — assessable-delivery validation and Review Bundle construction.
- `internal/review/diffindex.go` — changed-line index for citation validation.
- `internal/review/validate.go` — Reviewer-result validation and attestation creation.
- `internal/review/render.go` — human, JSON, and escaped Markdown output.
- `internal/adapters/git.go` — explicit base/head range reads.
- `internal/ops/{types.go,schema.go}` — `assessment-attested` op declaration and payload schema.
- `internal/materialize/{state.go,engine.go}` — replay and materialized attestation history.
- `internal/output/output.go` — show latest assessment in canonical issue output.
- `cmd/armature/review.go` — `arm review prepare` and `arm review record` adapters.
- `internal/skillsembed/skills/armature-reviewer/` — Reviewer operating procedure and rubric.
- `internal/skillsembed/skills/{armature-coordinator,armature-auditor,armature}/SKILL.md` — orchestration, responsibility, and command-reference updates.
- `internal/review/testdata/evals/cases.json` — development-only human-labeled evaluator corpus.
- `scripts/reviewer_eval_report.py` — development-only evaluator metric summarizer.
- `docs/{commands.md,concepts.md}` — customer-facing command and concept documentation.

## Task 1: Define the review protocol and rating algebra

**Files:**
- Create: `internal/review/types.go`
- Create: `internal/review/rating.go`
- Create: `internal/review/fingerprint.go`
- Create: `internal/review/types_test.go`
- Create: `internal/review/rating_test.go`
- Create: `internal/review/fingerprint_test.go`

- [ ] **Step 1: Write failing protocol and rating tests**

Create table-driven tests that lock the serialized field names and complete rating truth table:

```go
func TestDeriveRating(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []CriterionResult
		want Rating
	}{
		{"all satisfied", []CriterionResult{{Status: StatusSatisfied}}, RatingGreen},
		{"partial", []CriterionResult{{Status: StatusSatisfied}, {Status: StatusPartiallySatisfied}}, RatingYellow},
		{"indeterminate", []CriterionResult{{Status: StatusIndeterminate}}, RatingYellow},
		{"not satisfied wins", []CriterionResult{{Status: StatusIndeterminate}, {Status: StatusNotSatisfied}}, RatingRed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DeriveRating(tt.in))
		})
	}
}

func TestProtocolJSONUsesStableNames(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(ReviewResult{
		SchemaVersion: SchemaVersion,
		BundleID:      "sha256:bundle",
		Reviewer: ReviewerIdentity{SkillVersion: "1.0.0", Model: "model-x"},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "schema_version":1,
	  "bundle_id":"sha256:bundle",
	  "reviewer":{"skill_version":"1.0.0","model":"model-x"},
	  "criteria":null
	}`, string(b))
}
```

- [ ] **Step 2: Run the package tests and confirm the package is missing**

Run: `go test ./internal/review -run 'Test(DeriveRating|ProtocolJSON)' -v`

Expected: FAIL because `internal/review` and its types do not exist.

- [ ] **Step 3: Add the versioned protocol types**

Define these exact public types in `types.go`:

```go
package review

const SchemaVersion = 1

type CriterionSource string
const (
	SourceDefinitionOfDone CriterionSource = "definition_of_done"
	SourceAcceptance       CriterionSource = "acceptance"
)

type CriterionStatus string
const (
	StatusSatisfied          CriterionStatus = "satisfied"
	StatusPartiallySatisfied CriterionStatus = "partially_satisfied"
	StatusNotSatisfied       CriterionStatus = "not_satisfied"
	StatusIndeterminate      CriterionStatus = "indeterminate"
)

type Rating string
const (
	RatingGreen Rating = "green"
	RatingYellow Rating = "yellow"
	RatingRed Rating = "red"
)

type Criterion struct {
	ID     string          `json:"id"`
	Source CriterionSource `json:"source"`
	Text   string          `json:"text"`
}

type IssueDescriptor struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
}

type Contract struct {
	DefinitionOfDone string      `json:"definition_of_done"`
	Acceptance       []string    `json:"acceptance"`
	Criteria         []Criterion `json:"criteria"`
}

type Delivery struct {
	BaseSHA      string   `json:"base_sha"`
	HeadSHA      string   `json:"head_sha"`
	ChangedFiles []string `json:"changed_files"`
	Diff         string   `json:"diff"`
}

type Fingerprints struct {
	Contract string `json:"contract"`
	Delivery string `json:"delivery"`
}

type Bundle struct {
	SchemaVersion int             `json:"schema_version"`
	BundleID      string          `json:"bundle_id"`
	Issue         IssueDescriptor `json:"issue"`
	Contract      Contract        `json:"contract"`
	Delivery      Delivery        `json:"delivery"`
	Fingerprints  Fingerprints    `json:"fingerprints"`
}

type Citation struct {
	Path string `json:"path"`
	Side string `json:"side"`
	Line int    `json:"line"`
}

type CriterionResult struct {
	CriterionID    string          `json:"criterion_id"`
	CriterionText  string          `json:"criterion_text"`
	Status         CriterionStatus `json:"status"`
	Rationale      string          `json:"rationale"`
	Citations      []Citation      `json:"citations"`
	MissingEvidence string         `json:"missing_evidence,omitempty"`
}

type ReviewerIdentity struct {
	SkillVersion string `json:"skill_version"`
	Model        string `json:"model,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

type ReviewResult struct {
	SchemaVersion int               `json:"schema_version"`
	BundleID      string            `json:"bundle_id"`
	Reviewer      ReviewerIdentity  `json:"reviewer"`
	Criteria      []CriterionResult `json:"criteria"`
}

type StatusCounts struct {
	Satisfied          int `json:"satisfied"`
	PartiallySatisfied int `json:"partially_satisfied"`
	NotSatisfied       int `json:"not_satisfied"`
	Indeterminate      int `json:"indeterminate"`
}

type Attestation struct {
	SchemaVersion       int              `json:"schema_version"`
	BundleID            string           `json:"bundle_id"`
	ContractFingerprint string           `json:"contract_fingerprint"`
	DeliveryFingerprint string           `json:"delivery_fingerprint"`
	BaseSHA             string           `json:"base_sha"`
	HeadSHA             string           `json:"head_sha"`
	Reviewer             ReviewerIdentity `json:"reviewer"`
	Rating               Rating           `json:"rating"`
	Counts               StatusCounts     `json:"counts"`
	ResultFingerprint    string           `json:"result_fingerprint"`
	RecordedBy           string           `json:"recorded_by,omitempty"`
	RecordedAt           int64            `json:"recorded_at,omitempty"`
}
```

- [ ] **Step 4: Implement deterministic rating and counts**

`DeriveRating` must return red if any criterion is not satisfied, yellow if any is partial or indeterminate, and green otherwise. An empty result list is invalid input and must return an error from `ValidateResult`; do not give it a green rating.

```go
func DeriveRating(results []CriterionResult) Rating {
	rating := RatingGreen
	for _, result := range results {
		switch result.Status {
		case StatusNotSatisfied:
			return RatingRed
		case StatusPartiallySatisfied, StatusIndeterminate:
			rating = RatingYellow
		}
	}
	return rating
}
```

- [ ] **Step 5: Implement canonical fingerprints**

Use `encoding/json` plus SHA-256; prefix rendered hashes with `sha256:`. Provide `Fingerprint(v any) (string, error)` and `BundleID(issueID, contractFingerprint, deliveryFingerprint string) string`. Tests must prove map-free structs produce stable output and that a changed criterion or diff changes the corresponding hash.

- [ ] **Step 6: Run and pass the package tests**

Run: `go test ./internal/review -v`

Expected: PASS.

- [ ] **Step 7: Commit the protocol core**

```bash
git add internal/review/types.go internal/review/rating.go internal/review/fingerprint.go internal/review/*_test.go
git commit -m "feat: define semantic review protocol"
```

## Task 2: Add explicit Git ranges and prepare Review Bundles

**Files:**
- Modify: `internal/adapters/git.go`
- Modify: `internal/adapters/git_test.go`
- Create: `internal/review/prepare.go`
- Create: `internal/review/prepare_test.go`

- [ ] **Step 1: Write failing adapter range tests**

Add tests proving a range can target a head other than current `HEAD`:

```go
func TestDiffRangeUsesExplicitHead(t *testing.T) {
	repo := initTestRepo(t)
	base := commitFile(t, repo, "file.txt", "base\n", "base")
	head := commitFile(t, repo, "file.txt", "reviewed\n", "reviewed")
	_ = commitFile(t, repo, "file.txt", "later\n", "later")

	c := adapters.New(repo)
	diff, err := c.DiffRange(base, head)
	require.NoError(t, err)
	assert.Contains(t, diff, "+reviewed")
	assert.NotContains(t, diff, "+later")
}
```

Use a local `commitFile` test helper that writes, stages, commits, and returns `git rev-parse HEAD`.
Add a second test with changes to `code.go` and `.armature/ops/worker.log`; pass `.`, `:(exclude).armature/**`, and `:(exclude).arm/**`, then assert only `code.go` appears in both diff and name-only output.

- [ ] **Step 2: Run the adapter test and confirm failure**

Run: `go test ./internal/adapters -run 'Test(DiffRange|DiffNameOnlyRange|ResolveRevision)' -v`

Expected: FAIL because the explicit-range methods do not exist.

- [ ] **Step 3: Implement the read-only Git methods**

Add:

```go
func (c *Client) ResolveRevision(ref string) (string, error) {
	out, err := c.cmd("rev-parse", "--verify", ref+"^{commit}").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *Client) DiffRange(baseSHA, headSHA string, pathspecs ...string) (string, error) {
	args := []string{"diff", "--no-ext-diff", baseSHA, headSHA}
	if len(pathspecs) > 0 { args = append(args, append([]string{"--"}, pathspecs...)...) }
	out, err := c.cmd(args...).Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s %s: %w", baseSHA, headSHA, err)
	}
	return string(out), nil
}

func (c *Client) DiffNameOnlyRange(baseSHA, headSHA string, pathspecs ...string) ([]string, error) {
	args := []string{"diff", "--name-only", "--no-ext-diff", baseSHA, headSHA}
	if len(pathspecs) > 0 { args = append(args, append([]string{"--"}, pathspecs...)...) }
	out, err := c.cmd(args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s %s: %w", baseSHA, headSHA, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" { return []string{}, nil }
	return strings.Split(raw, "\n"), nil
}
```

Keep `DiffFrom` and `DiffNameOnly` as compatibility wrappers using `HEAD`.

- [ ] **Step 4: Write failing Prepare tests**

Use a fake `RangeReader` to cover: Done and Merged accepted; Open rejected; empty Outcome rejected; empty diff rejected; a coordination-only range rejected; malformed Acceptance rejected; missing DoD and missing Acceptance each become an empty criterion for Reviewer `indeterminate`; base/head are resolved; changed files sort deterministically; and both range calls receive the required exclusion pathspecs.

```go
type fakeRangeReader struct {
	diff  string
	files []string
}
func (f fakeRangeReader) ResolveRevision(ref string) (string, error) { return "resolved-" + ref, nil }
func (f fakeRangeReader) DiffRange(_, _ string, _ ...string) (string, error) { return f.diff, nil }
func (f fakeRangeReader) DiffNameOnlyRange(_, _ string, _ ...string) ([]string, error) { return f.files, nil }
```

- [ ] **Step 5: Implement `Prepare`**

Define a materialization-independent input and port:

```go
type IssueInput struct {
	ID               string
	Type             string
	Status           string
	Title            string
	Outcome          string
	DefinitionOfDone string
	Acceptance       json.RawMessage
}

type RangeReader interface {
	ResolveRevision(ref string) (string, error)
	DiffRange(baseSHA, headSHA string, pathspecs ...string) (string, error)
	DiffNameOnlyRange(baseSHA, headSHA string, pathspecs ...string) ([]string, error)
}

func Prepare(issue IssueInput, baseRef, headRef string, git RangeReader) (Bundle, error)
```

`Prepare` must:

1. accept only `done` or `merged` Issues;
2. require non-blank Outcome;
3. parse Acceptance as `[]string` when present;
4. always create `definition_of_done`, using empty text when missing;
5. create an empty `acceptance` criterion when the list is absent, otherwise create `acceptance[0]`, `acceptance[1]`, and so on;
6. resolve both refs to full SHAs;
7. pass `.` plus Git exclusion pathspecs for `.armature/**` and `.arm/**` to both range reads;
8. require a non-empty changed-file list and diff after those exclusions;
9. sort changed files;
10. fingerprint Contract and Delivery; and
11. derive Bundle ID from Issue ID and both fingerprints.

- [ ] **Step 6: Run focused and package tests**

Run: `go test ./internal/adapters ./internal/review -v`

Expected: PASS.

- [ ] **Step 7: Commit bundle preparation**

```bash
git add internal/adapters/git.go internal/adapters/git_test.go internal/review/prepare.go internal/review/prepare_test.go
git commit -m "feat: prepare review bundles from git ranges"
```

## Task 3: Validate Reviewer results and diff citations

**Files:**
- Create: `internal/review/diffindex.go`
- Create: `internal/review/diffindex_test.go`
- Create: `internal/review/validate.go`
- Create: `internal/review/validate_test.go`

- [ ] **Step 1: Write failing unified-diff index tests**

Use a fixture containing modified, added, deleted, and renamed files. Assert `Contains(path, side, line)` accepts only lines present in a hunk and rejects context outside the hunk, absolute paths, `..`, and files absent from the diff.

```go
func TestDiffIndexContainsChangedLines(t *testing.T) {
	index, err := BuildDiffIndex("diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -2,2 +2,2 @@\n-old\n+new\n context\n")
	require.NoError(t, err)
	assert.True(t, index.Contains(Citation{Path: "a.go", Side: "old", Line: 2}))
	assert.True(t, index.Contains(Citation{Path: "a.go", Side: "new", Line: 2}))
	assert.False(t, index.Contains(Citation{Path: "a.go", Side: "new", Line: 3}))
}
```

- [ ] **Step 2: Implement `DiffIndex`**

Parse `diff --git`, `---`, `+++`, and `@@ -old,count +new,count @@` headers. Track changed old lines for `-` rows and changed new lines for `+` rows; context rows advance both counters but are not citable. Normalize `a/` and `b/` prefixes, reject absolute or escaping paths with `filepath.IsAbs` and `filepath.Clean`, and represent changed locations as `map[string]map[string]map[int]struct{}`.

- [ ] **Step 3: Write failing result-validation tests**

Cover every schema rule from the design: wrong schema or bundle ID, missing/duplicate criterion, text mismatch, unknown status, blank rationale, absent citations without `missing_evidence`, invalid citation, missing Reviewer Skill version, and empty criteria. Include a valid result that derives yellow and exact counts.

- [ ] **Step 4: Implement validation and attestation creation**

Expose:

```go
type ValidationOutput struct {
	Rating            Rating
	Counts            StatusCounts
	ResultFingerprint string
}

func ValidateResult(bundle Bundle, result ReviewResult) (ValidationOutput, error)

func NewAttestation(bundle Bundle, result ReviewResult, validated ValidationOutput) Attestation

func IsDuplicate(attestations []Attestation, candidate Attestation) bool

func Applicable(attestations []Attestation, bundle Bundle) *Attestation
```

Validate criterion identity and verbatim text against `bundle.Contract.Criteria`, validate each citation against `DiffIndex`, require `missing_evidence` when citations are empty, derive counts/rating, and fingerprint the canonical `ReviewResult`. `Applicable` returns the newest attestation whose bundle, contract, and delivery fingerprints all match.

- [ ] **Step 5: Run and pass review tests**

Run: `go test ./internal/review -v`

Expected: PASS.

- [ ] **Step 6: Commit result validation**

```bash
git add internal/review/diffindex.go internal/review/diffindex_test.go internal/review/validate.go internal/review/validate_test.go
git commit -m "feat: validate semantic review results"
```

## Task 4: Render safe detailed and summary output

**Files:**
- Create: `internal/review/render.go`
- Create: `internal/review/render_test.go`

- [ ] **Step 1: Write failing rendering tests**

Assert Markdown includes Issue ID, base/head SHAs, rating, criterion statuses, rationales, citations, and missing-evidence text. Include model content containing `<script>`, `<!--`, pipes, and backticks; assert raw HTML is escaped and table cells remain valid.

- [ ] **Step 2: Implement renderers**

Expose:

```go
func RenderMarkdown(bundle Bundle, result ReviewResult, validated ValidationOutput) string
func RenderHuman(bundle Bundle, result ReviewResult, validated ValidationOutput) string
func RenderJSON(bundle Bundle, result ReviewResult, validated ValidationOutput) ([]byte, error)
```

Use `html.EscapeString` before Markdown interpolation, escape `|`, newlines, and backticks in table cells, and render semantic text labels alongside color names. JSON output must use a typed envelope containing Bundle ID, Issue ID, Rating, Counts, Reviewer, and Criterion Results.

- [ ] **Step 3: Run tests and inspect one golden output**

Run: `go test ./internal/review -run 'TestRender' -v`

Expected: PASS; no raw `<script>` or `<!--` appears in output.

- [ ] **Step 4: Commit rendering**

```bash
git add internal/review/render.go internal/review/render_test.go
git commit -m "feat: render semantic review evidence"
```

## Task 5: Persist compact assessment attestations through ops

**Files:**
- Modify: `internal/ops/types.go`
- Modify: `internal/ops/schema.go`
- Modify: `internal/ops/types_test.go`
- Modify: `internal/ops/ops_test.go`
- Modify: `internal/materialize/state.go`
- Modify: `internal/materialize/engine.go`
- Modify: `internal/materialize/engine_test.go`
- Modify: `internal/output/output.go`
- Modify: `internal/output/output_test.go`

- [ ] **Step 1: Write failing op round-trip and schema tests**

Add `OpAssessmentAttested = "assessment-attested"` to the expected registered set. Test that `Payload{Assessment: json.RawMessage([]byte("{\"bundle_id\":\"sha256:test\"}"))}` survives `MarshalOp`/`ParseLine`, and that generated SCHEMA documents `assessment-attested: assessment`.

- [ ] **Step 2: Add the op and payload field**

Add:

```go
const OpAssessmentAttested = "assessment-attested"

type Payload struct {
	Assessment json.RawMessage `json:"assessment,omitempty"`
}
```

Add `Assessment` to the existing `Payload` struct without removing or renaming any existing field.

Document the op in `schemaOpDocs`.

- [ ] **Step 3: Write failing materialization tests**

Test that replay appends an Attestation, stamps `RecordedBy` and `RecordedAt` from the op rather than trusting payload values, ignores an exact duplicate result fingerprint, preserves distinct later results, and tolerates a missing target consistently with other issue-scoped low-stakes ops.

- [ ] **Step 4: Implement attestation materialization**

Import `internal/review` from `materialize`, add:

```go
AssessmentAttestations []review.Attestation `json:"assessment_attestations,omitempty"`
```

Register `ops.OpAssessmentAttested` and implement `applyAssessmentAttested`: unmarshal `Payload.Assessment`, overwrite `RecordedBy`/`RecordedAt`, append only when `ResultFingerprint` is new, and update Issue timestamp.

- [ ] **Step 5: Surface attestations in canonical Issue output**

Add `AssessmentAttestations []review.Attestation` to `output.IssueJSON`. Human output should print the latest attestation as:

```text
Review:    yellow (head abc1234; 1 satisfied, 1 indeterminate)
```

Do not print full rationales because they are intentionally not persisted.

- [ ] **Step 6: Run affected tests**

Run: `go test ./internal/ops ./internal/materialize ./internal/output -v`

Expected: PASS, including `TestGenerateSchema_DocumentsEveryRegisteredOpType`.

- [ ] **Step 7: Commit attestation persistence**

```bash
git add internal/ops internal/materialize internal/output
git commit -m "feat: materialize assessment attestations"
```

## Task 6: Add `arm review prepare` and `arm review record`

**Files:**
- Create: `cmd/armature/review.go`
- Create: `cmd/armature/review_test.go`
- Modify: `cmd/armature/main.go`
- Modify: `cmd/armature/main_test.go`

- [ ] **Step 1: Write failing command-registration and prepare tests**

Assert `arm review --help` lists `prepare` and `record`. In a bootstrapped temp repo, create a Task with DoD and Acceptance, transition it to Done with an Outcome, create base/head commits, and assert:

```bash
arm review prepare TASK-1 --base <base> --head <head> --format json
```

returns a parseable `review.Bundle` with full SHAs, exact criteria, changed file, and non-empty fingerprints. Add negative tests for Open Issue, empty Outcome, invalid refs, and empty diff.

- [ ] **Step 2: Implement the review command group and prepare adapter**

Register `newReviewCmd()` in the workflow command group. `prepare` must load a current snapshot, map `materialize.Issue` to `review.IssueInput`, call `review.Prepare` with `adapters.New(ctx.RepoPath)`, and encode the bundle. Human, agent, and JSON formats all emit JSON because the output is machine input; human uses indentation.

- [ ] **Step 3: Write failing record tests**

Generate a valid `ReviewResult` from the prepared bundle and write it to a temp file. Assert `record`:

- reconstructs the bundle from the same base/head;
- appends exactly one `assessment-attested` op;
- emits Markdown with `--format markdown`;
- emits typed JSON with `--format json`;
- reads `--input -` from command stdin;
- does not append a second op for an exact duplicate;
- rejects a stale bundle ID and invalid citation; and
- appends nothing on validation failure.

- [ ] **Step 4: Implement record with write-after-validation ordering**

The record path must be:

```go
bundle, err := prepareReviewBundle(ctx, issueID, baseRef, headRef)
if err != nil { return err }
result, err := decodeReviewResult(input)
if err != nil { return err }
validated, err := review.ValidateResult(bundle, result)
if err != nil { return err }
attestation := review.NewAttestation(bundle, result, validated)
duplicate := review.IsDuplicate(issue.AssessmentAttestations, attestation)
if !duplicate {
	attestation.RecordedBy = workerID
	attestation.RecordedAt = nowEpoch()
	raw, err := json.Marshal(attestation)
	if err != nil { return err }
	op := ops.Op{Type: ops.OpAssessmentAttested, TargetID: issueID, Timestamp: attestation.RecordedAt, WorkerID: workerID, Payload: ops.Payload{Assessment: raw}}
	if err := appendLowStakesOp(state, logPath, op); err != nil { return err }
}
```

Only after validation and optional append should the command render output. Support file input and `-` for stdin. Return validation errors through the existing structured error path.

- [ ] **Step 5: Run command and full Go tests**

Run: `go test ./cmd/armature ./internal/review ./internal/materialize -v`

Expected: PASS.

- [ ] **Step 6: Commit the CLI slice**

```bash
git add cmd/armature/review.go cmd/armature/review_test.go cmd/armature/main.go cmd/armature/main_test.go
git commit -m "feat: add semantic review commands"
```

## Task 7: Add the `armature-reviewer` Skill

**Files:**
- Create: `internal/skillsembed/skills/armature-reviewer/SKILL.md`
- Create: `internal/skillsembed/skills/armature-reviewer/references/rubric.md`
- Modify: `internal/skillsembed/plugin.json`
- Modify: `cmd/armature/bootstrap_test.go`

- [ ] **Step 1: Write a failing bootstrap deployment test**

Bootstrap into a temp destination and assert `armature-reviewer/SKILL.md` and `references/rubric.md` are deployed with the other embedded Skills.

- [ ] **Step 2: Author the Reviewer Skill**

The frontmatter must name `armature-reviewer`. Its procedure must require:

1. a fresh context with no implementation role;
2. the supplied Review Bundle as the only binding contract;
3. no edits, remediation, customer check execution, activity-log reading, or inferred requirements;
4. `git diff BASE HEAD` as the assessment subject;
5. bounded read-only `rg`/file inspection only to interpret changed code;
6. exactly one result for every bundle criterion;
7. the four-state rubric from `references/rubric.md`;
8. structured citations or explicit `missing_evidence`;
9. JSON matching `review.ReviewResult`; and
10. no model-selected overall rating.

Include this output skeleton with no prose outside JSON:

```json
{
  "schema_version": 1,
  "bundle_id": "sha256:from-bundle",
  "reviewer": {"skill_version": "1.0.0"},
  "criteria": [
    {
      "criterion_id": "definition_of_done",
      "criterion_text": "verbatim text from bundle",
      "status": "indeterminate",
      "rationale": "Concise evidence-based explanation.",
      "citations": [],
      "missing_evidence": "What the delivery diff cannot establish."
    }
  ]
}
```

- [ ] **Step 3: Write the rubric reference**

Define satisfied, partially satisfied, not satisfied, and indeterminate with positive and negative examples. Explicitly state that tests, comments, and Outcome claims are evidence only when the diff connects them to implementation behavior; their mere presence cannot justify green.

- [ ] **Step 4: Update plugin metadata and pass Skill validation**

Add Reviewer to the plugin description. Run:

```bash
make validate-skills
go test ./cmd/armature -run 'Test.*ReviewerSkill' -v
```

Expected: Skill validation and deployment test PASS.

- [ ] **Step 5: Commit the Reviewer Skill**

```bash
git add internal/skillsembed/skills/armature-reviewer internal/skillsembed/plugin.json cmd/armature/bootstrap_test.go
git commit -m "feat: bundle armature reviewer skill"
```

## Task 8: Add the development-only evaluator corpus and metrics

**Files:**
- Create: `internal/review/testdata/evals/cases.json`
- Create: `internal/review/testdata/evals/README.md`
- Create: `scripts/reviewer_eval_report.py`
- Create: `scripts/test_reviewer_eval_report.py`
- Create: `scripts/testdata/reviewer_eval_results.json`

- [ ] **Step 1: Write failing metric-script tests**

Use Python `unittest` fixtures covering exact agreement, false green, valid indeterminate, unsupported citation, and malformed schema. Assert the report includes:

```text
cases: 5
schema_compliance: 80.0%
criterion_agreement: 75.0%
false_green_rate: 50.0%
unsupported_citation_rate: 25.0%
indeterminate_agreement: 100.0%
```

- [ ] **Step 2: Implement the metric summarizer**

The script takes `--cases CASES_JSON --results RESULTS_JSON`. Use only Python standard library. Match by `case_id` and `criterion_id`, validate required result fields and allowed statuses, derive ratings using the same red/yellow/green algebra, and compute the five metrics. Exit non-zero only for unreadable/malformed input, not for poor model scores; phase one records a baseline rather than enforcing a threshold.

- [ ] **Step 3: Add the labeled corpus**

Create at least eight complete cases with tiny unified diffs and human labels:

1. fully satisfied behavior and test;
2. one compound criterion partially satisfied;
3. required behavior absent;
4. runtime property not observable from diff (`indeterminate`);
5. misleading Outcome with unrelated diff;
6. test-only claim with no implementation support;
7. surrounding unchanged interface needed to interpret changed implementation;
8. ambiguous empty Acceptance criterion (`indeterminate`).

Every case contains `case_id`, a complete Review Bundle, expected criterion statuses, expected rating, and valid citable changed lines. The README gives the exact fresh-context dispatch prompt and states that generated result files go under `/tmp`, never into the product repository.

- [ ] **Step 4: Run deterministic corpus and script tests**

Run:

```bash
python3 -m unittest scripts/test_reviewer_eval_report.py -v
python3 scripts/reviewer_eval_report.py --cases internal/review/testdata/evals/cases.json --results scripts/testdata/reviewer_eval_results.json
```

For the second command, create `scripts/testdata/reviewer_eval_results.json` as a checked-in synthetic result fixture whose expected metrics are asserted by the unit test. Expected: exit zero and the documented metrics.

- [ ] **Step 5: Run a real baseline without making it a product gate**

Using the current supported harness, dispatch one fresh `armature-reviewer` context per corpus case, collect JSON under `/tmp/armature-reviewer-eval-results.json`, then run the metric script. Copy the metric summary into the implementation handoff; do not commit provider output and do not add this invocation to `make check`.

- [ ] **Step 6: Commit evaluator development tooling**

```bash
git add internal/review/testdata/evals scripts/reviewer_eval_report.py scripts/test_reviewer_eval_report.py scripts/testdata/reviewer_eval_results.json
git commit -m "test: add semantic reviewer eval corpus"
```

## Task 9: Wire Reviewer, Coordinator, Auditor, and product documentation

**Files:**
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md`
- Modify: `internal/skillsembed/skills/armature-auditor/SKILL.md`
- Modify: `internal/skillsembed/skills/armature/SKILL.md`
- Modify: `docs/commands.md`
- Modify: `docs/concepts.md`

- [ ] **Step 1: Update the Coordinator workflow**

Insert Reviewer dispatch after each Worker delivery commit and before `arm merged`. Require the Coordinator to record base/head, run `arm review prepare`, dispatch a fresh agent with `armature-reviewer` as its first instruction, run `arm review record`, retry once on invalid/unavailable output, and retain Markdown for PR assembly. State explicitly that yellow, red, stale, or unavailable results never block or trigger remediation in phase one.

Add parent Story review after Story Outcome is recorded, using feature-base/head. The PR section must render:

```markdown
## Semantic conformance

| Issue | Rating | Satisfied | Partial | Unsatisfied | Indeterminate |
|---|---|---:|---:|---:|---:|
| TASK-1 | yellow | 2 | 0 | 0 | 1 |
```

- [ ] **Step 2: Separate Auditor responsibility**

Keep citation integrity, source freshness, Outcome concreteness, scope-overlap, and doctor checks. Remove the instruction to semantically decide whether Outcome proves every Acceptance criterion. Add advisory assessment coverage reporting via `arm show ISSUE --format json`; absence and rating must not enter the Auditor's pass/fail table in phase one.

- [ ] **Step 3: Update the quick-reference Skill**

Add:

```text
arm review prepare ISSUE --base BASE --head HEAD --format agent
arm review record ISSUE --base BASE --head HEAD --input result.json --format markdown
```

State that these commands validate and record externally produced review results; they do not invoke a model.

- [ ] **Step 4: Document commands and concepts**

Add complete `review prepare` and `review record` sections to `docs/commands.md`, including flags, examples, report-only behavior, stdin input, and output formats. Add a semantic-conformance concept to `docs/concepts.md` distinguishing customer-owned deterministic checks, Reviewer judgment, compact attestations, and ephemeral detailed reports. Mention that Armature's own development workflow is not customer configuration.

- [ ] **Step 5: Validate Skills and documentation diff**

Run:

```bash
make validate-skills
git diff --check
```

Expected: both exit zero.

- [ ] **Step 6: Commit workflow and docs**

```bash
git add internal/skillsembed/skills/armature-coordinator/SKILL.md internal/skillsembed/skills/armature-auditor/SKILL.md internal/skillsembed/skills/armature/SKILL.md docs/commands.md docs/concepts.md
git commit -m "docs: wire semantic review into delivery workflow"
```

## Task 10: Close end-to-end coverage and repository validation

**Files:**
- Modify: `cmd/armature/review_test.go`
- Modify: `internal/materialize/pipeline_test.go`
- Modify: `docs/superpowers/specs/2026-06-27-semantic-conformance-review-design.md` only if implementation names differ from the approved protocol

- [ ] **Step 1: Add a full single-branch lifecycle test**

The test must bootstrap a repository, create/claim/transition a Task, commit a delivery, prepare a bundle, record a valid yellow result, rematerialize from an empty state directory, and assert:

- the attestation survives full replay;
- `arm show --format json` exposes it;
- duplicate record is idempotent;
- amending DoD makes `review.Applicable` return nil for the old bundle; and
- Markdown still renders the detailed ephemeral result.

- [ ] **Step 2: Add dual-branch replay coverage**

Use the existing dual-branch test setup to record an attestation on `_armature`, rematerialize, and assert the same compact state. Do not persist the full Review Result anywhere under `.armature/`.

- [ ] **Step 3: Run focused tests uncached**

Run:

```bash
go test -count=1 ./internal/review ./internal/adapters ./internal/ops ./internal/materialize ./internal/output ./cmd/armature
python3 -m unittest scripts/test_reviewer_eval_report.py -v
make validate-skills
```

Expected: all commands exit zero.

- [ ] **Step 4: Run the Armature repository's development checks**

Run:

```bash
make check
```

Expected: lint, tests, coverage, mutation checks, Skill validation, and build all pass. This command validates development of Armature itself; it is not invoked by the product review protocol and imposes no customer configuration.

- [ ] **Step 5: Verify scope and retained artifacts**

Run:

```bash
git diff --check
git status --short
find .armature -type f -iname '*review*' -o -iname '*assessment*'
```

Expected: no whitespace errors; only intended source/docs/test changes; no persisted detailed assessment files. Compact attestations exist only in op fixtures created by tests or runtime ops, not as repository report artifacts.

- [ ] **Step 6: Commit closeout tests**

```bash
git add cmd/armature/review_test.go internal/materialize/pipeline_test.go docs/superpowers/specs/2026-06-27-semantic-conformance-review-design.md
git commit -m "test: cover semantic review lifecycle"
```

- [ ] **Step 7: Record final evidence in the implementation handoff**

Report the commit list, focused-test results, `make check` result, real evaluator baseline metrics, and any `assessment_unavailable` dogfood findings. Do not claim semantic gating readiness; phase one remains advisory.
