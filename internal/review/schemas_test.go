package review_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

// TestArtifactSchemas_REQ_TOPTIER_S2_T1 verifies that all JSON Schema files
// under docs/schemas/ are valid JSON and properly structure the artifact schemas.
// This test validates the schemas themselves, not the documents that reference them.
func TestArtifactSchemas_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	// Find the repo root by looking for the docs directory
	repoRoot := findRepoRoot(t)
	schemasDir := filepath.Join(repoRoot, "docs", "schemas")

	// Expected schema files
	schemaFiles := []string{
		"plan.schema.json",
		"review-bundle.schema.json",
		"conformance-assessment.schema.json",
		"activity-index.schema.json",
	}

	for _, schemaFile := range schemaFiles {
		t.Run(schemaFile, func(t *testing.T) {
			t.Parallel()
			schemaPath := filepath.Join(schemasDir, schemaFile)

			// Verify the file exists
			_, err := os.Stat(schemaPath)
			require.NoError(t, err, "schema file should exist: %s", schemaPath)

			// Verify the file contains valid JSON
			data, err := os.ReadFile(schemaPath)
			require.NoError(t, err, "should be able to read schema file")

			var schemaObj any
			err = json.Unmarshal(data, &schemaObj)
			require.NoError(t, err, "schema file should contain valid JSON")

			// Verify it's an object with required schema-level fields
			schema, ok := schemaObj.(map[string]any)
			require.True(t, ok, "schema should be a JSON object")

			// Check for required schema fields
			require.Contains(t, schema, "$schema", "schema should have $schema field")
			require.Contains(t, schema, "title", "schema should have title field")
			require.Contains(t, schema, "type", "schema should have type field")
			require.Contains(t, schema, "properties", "schema should have properties field")

			// Verify the schema itself compiles as a valid JSON Schema document.
			compiler := jsonschema.NewCompiler()
			require.NoError(t, compiler.AddResource(schemaFile, bytes.NewReader(data)))
			_, err = compiler.Compile(schemaFile)
			require.NoError(t, err, "schema should compile as valid JSON Schema: %s", schemaFile)
		})
	}
}

// findRepoRoot locates the repository root by searching for docs/schemas directory
func findRepoRoot(t *testing.T) string {
	t.Helper()

	// Start from current working directory and walk up
	cwd, err := os.Getwd()
	require.NoError(t, err, "should be able to get working directory")

	// Try up to 10 levels up
	for range 10 {
		schemasDir := filepath.Join(cwd, "docs", "schemas")
		if _, err := os.Stat(schemasDir); err == nil {
			return cwd
		}
		cwd = filepath.Dir(cwd)
	}

	t.Fatalf("could not find repo root (docs/schemas directory)")
	return ""
}

// validateAgainstSchema compiles the named schema file (relative to docs/schemas)
// and validates the given JSON document against it, failing the test with the
// full validation error on mismatch.
func validateAgainstSchema(t *testing.T, schemaFile string, docJSON string) {
	t.Helper()

	repoRoot := findRepoRoot(t)
	schemaPath := filepath.Join(repoRoot, "docs", "schemas", schemaFile)

	schemaData, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "should be able to read schema file")

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource(schemaFile, bytes.NewReader(schemaData)))
	sch, err := compiler.Compile(schemaFile)
	require.NoError(t, err, "schema should compile: %s", schemaFile)

	var doc any
	require.NoError(t, json.Unmarshal([]byte(docJSON), &doc), "example JSON should be valid")

	err = sch.Validate(doc)
	require.NoError(t, err, "example should validate against %s", schemaFile)
}

// validateSchemaRejects compiles the named schema file (relative to
// docs/schemas) and asserts that the given JSON document fails validation
// against it, proving the schema actually rejects invalid input rather than
// accepting anything.
func validateSchemaRejects(t *testing.T, schemaFile string, docJSON string) {
	t.Helper()

	repoRoot := findRepoRoot(t)
	schemaPath := filepath.Join(repoRoot, "docs", "schemas", schemaFile)

	schemaData, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "should be able to read schema file")

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource(schemaFile, bytes.NewReader(schemaData)))
	sch, err := compiler.Compile(schemaFile)
	require.NoError(t, err, "schema should compile: %s", schemaFile)

	var doc any
	require.NoError(t, json.Unmarshal([]byte(docJSON), &doc), "example JSON should be valid JSON")

	err = sch.Validate(doc)
	require.Error(t, err, "invalid example should be rejected by %s", schemaFile)
}

// TestReviewBundleSchema_ValidExample_REQ_TOPTIER_S2_T1 validates that the
// review-bundle schema accepts a valid ReviewBundle example.
func TestReviewBundleSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	// Valid minimal review bundle example. SHAs and fingerprints use the
	// lengths the schema actually requires: head/base SHA = 40 hex chars
	// (git commit SHA), fingerprints = 64 hex chars (SHA-256).
	bundleJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "issue": {
    "id": "TASK-001",
    "type": "task",
    "title": "Test task",
    "outcome": "Implemented feature X"
  },
  "contract": {
    "definition_of_done": "Feature is implemented and tested",
    "acceptance": ["Passes unit tests", "Code is documented"]
  },
  "delivery": {
    "base_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "head_sha": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "changed_files": ["src/main.go", "src/main_test.go"]
  },
  "fingerprints": {
    "contract": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "delivery": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  }
}`

	validateAgainstSchema(t, "review-bundle.schema.json", bundleJSON)
}

// TestConformanceAssessmentSchema_ValidExample_REQ_TOPTIER_S2_T1 validates that
// the conformance-assessment schema accepts a valid assessment example.
func TestConformanceAssessmentSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	// Valid minimal conformance assessment example
	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "Feature is fully implemented and tested",
      "citations": [
        {"path": "src/main.go", "line": 10}
      ]
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateAgainstSchema(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestConformanceAssessmentSchema_AllowsPathLevelCitation_REQ_TOPTIER_S2_T1
// verifies that line zero is accepted as the documented path-level citation.
func TestConformanceAssessmentSchema_AllowsPathLevelCitation_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [{
    "id": "definition_of_done",
    "status": "satisfied",
    "rationale": "Path-level evidence is sufficient",
    "citations": [{"path": "src/main.go", "line": 0}]
  }],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateAgainstSchema(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestActivityIndexSchema_ValidExample_REQ_TOPTIER_S2_T1 validates that the
// activity-index schema accepts a valid index example.
func TestActivityIndexSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	// Valid minimal activity index example
	indexJSON := `{
  "schema_version": 1,
  "log_path": "/path/to/armature-activity.log",
  "log_digest": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "entry_count": 2,
  "delivery_head_count": 2,
  "earlier_count": 0,
  "entries": [
    {
      "id": "0",
      "command": "go test ./...",
      "exit_status": 0,
      "head_anchor": true,
      "category": "test",
      "log_pointer": "0"
    },
    {
      "id": "1",
      "command": "make build",
      "exit_status": 0,
      "head_anchor": true,
      "category": "build",
      "log_pointer": "1"
    }
  ]
}`

	validateAgainstSchema(t, "activity-index.schema.json", indexJSON)
}

// TestPlanSchema_ValidExample_REQ_TOPTIER_S2_T1 validates that the plan schema
// accepts a valid plan example.
func TestPlanSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	// Valid minimal plan example
	planJSON := `{
  "version": 1,
  "title": "Feature decomposition",
  "issues": [
    {
      "id": "FEATURE-S1-T1",
      "title": "Implement core logic",
      "type": "task",
      "scope": "src/main.go",
      "priority": "high",
      "dod": "Core logic implemented and tested",
      "parent": "FEATURE-S1",
      "blocked_by": [],
      "notes": [],
      "acceptance": ["Tests pass", "Code reviewed"]
    }
  ]
}`

	validateAgainstSchema(t, "plan.schema.json", planJSON)
}

// TestPlanSchema_AllowsNullOptionalLists_REQ_TOPTIER_S2_T1 verifies that the
// nullable list fields emitted by the planner validate when absent data is
// encoded as null.
func TestPlanSchema_AllowsNullOptionalLists_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	planJSON := `{
  "version": 1,
  "title": "Feature decomposition",
  "issues": [
    {
      "id": "FEATURE-S1-T1",
      "title": "Implement core logic",
      "type": "task",
      "context_files": null,
      "blocked_by": null,
      "notes": null,
      "acceptance": null
    }
  ]
}`

	validateAgainstSchema(t, "plan.schema.json", planJSON)
}

func TestPlanAndReviewBundleSchemasAcceptAllIssueTypes_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	for _, issueType := range []string{"feature", "bug"} {
		t.Run(issueType, func(t *testing.T) {
			t.Parallel()

			planJSON := `{"version":1,"title":"Issue type coverage","issues":[{"id":"TYPE-1","title":"Accepted type","type":"` + issueType + `"}]}`
			validateAgainstSchema(t, "plan.schema.json", planJSON)

			bundleJSON := `{"schema_version":1,"bundle_id":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",` +
				`"issue":{"id":"TYPE-1","type":"` + issueType + `","title":"Accepted type","outcome":"Implemented"},` +
				`"contract":{"definition_of_done":"Done","acceptance":[]},` +
				`"delivery":{"base_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
				`"head_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","changed_files":[]},` +
				`"fingerprints":{"contract":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
				`"delivery":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}`
			validateAgainstSchema(t, "review-bundle.schema.json", bundleJSON)
		})
	}
}

// TestPlanSchema_InvalidExample_REQ_TOPTIER_S2_T1 asserts that the plan
// schema rejects an issue that is missing the required "type" field, proving
// the schema's required-fields constraint is actually enforced.
func TestPlanSchema_InvalidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	planJSON := `{
  "version": 1,
  "title": "Feature decomposition",
  "issues": [
    {
      "id": "FEATURE-S1-T1",
      "title": "Implement core logic"
    }
  ]
}`

	validateSchemaRejects(t, "plan.schema.json", planJSON)
}

// TestReviewBundleSchema_InvalidExample_REQ_TOPTIER_S2_T1 asserts that the
// review-bundle schema rejects a bundle_id missing the required
// "sha256:" prefix, proving the pattern constraint is actually enforced.
func TestReviewBundleSchema_InvalidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	bundleJSON := `{
  "schema_version": 1,
  "bundle_id": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "issue": {
    "id": "TASK-001",
    "type": "task",
    "title": "Test task",
    "outcome": "Implemented feature X"
  },
  "contract": {
    "definition_of_done": "Feature is implemented and tested",
    "acceptance": ["Passes unit tests", "Code is documented"]
  },
  "delivery": {
    "base_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "head_sha": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "changed_files": ["src/main.go", "src/main_test.go"]
  },
  "fingerprints": {
    "contract": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "delivery": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  }
}`

	validateSchemaRejects(t, "review-bundle.schema.json", bundleJSON)
}

// TestConformanceAssessmentSchema_InvalidExample_REQ_TOPTIER_S2_T1 asserts
// that the conformance-assessment schema rejects a result with a status
// value outside the enum, proving the enum constraint is actually enforced.
func TestConformanceAssessmentSchema_InvalidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "definition_of_done",
      "status": "mostly_satisfied",
      "rationale": "Feature is fully implemented and tested",
      "citations": [
        {"path": "src/main.go", "line": 10}
      ]
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateSchemaRejects(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestConformanceAssessmentSchema_RejectsSatisfiedWithoutEvidence_REQ_LNGHZN_S8_T2
// asserts that evidence-free satisfaction is unrepresentable: a satisfied
// result with neither citations nor missing_evidence is rejected, matching
// CriterionResult.Valid().
func TestConformanceAssessmentSchema_RejectsSatisfiedWithoutEvidence_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "acceptance[1]",
      "status": "satisfied",
      "rationale": "make check is green per the outcome text"
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateSchemaRejects(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestConformanceAssessmentSchema_RejectsSatisfiedWithOnlyMissingEvidence_REQ_LNGHZN_S8_T2
// asserts that missing_evidence cannot stand in for citations on a satisfied
// result: that shape is unrepresentable in the schema, matching
// CriterionResult.Valid().
func TestConformanceAssessmentSchema_RejectsSatisfiedWithOnlyMissingEvidence_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "acceptance[1]",
      "status": "satisfied",
      "rationale": "make check is green per the outcome text",
      "missing_evidence": "dropped activity citation; no remaining evidence"
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateSchemaRejects(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestConformanceAssessmentSchema_RequiresMissingEvidenceWhenNoCitations_REQ_TOPTIER_S2_T1
// asserts that the conformance-assessment schema rejects a non-satisfied
// result that has no citations and no missing_evidence, matching the runtime
// rule enforced by CriterionResult.Valid() in internal/review/types.go.
func TestConformanceAssessmentSchema_RequiresMissingEvidenceWhenNoCitations_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "acceptance[0]",
      "status": "not_satisfied",
      "rationale": "Not implemented"
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateSchemaRejects(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestConformanceAssessmentSchema_AllowsMissingEvidenceWhenNoCitations_REQ_TOPTIER_S2_T1
// asserts that the same shape is accepted once missing_evidence is supplied.
func TestConformanceAssessmentSchema_AllowsMissingEvidenceWhenNoCitations_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "acceptance[0]",
      "status": "not_satisfied",
      "rationale": "Not implemented",
      "missing_evidence": "No code found implementing this criterion"
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateAgainstSchema(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestConformanceAssessmentSchema_RejectsCitationWithBothPathAndActivityEntryID_REQ_TOPTIER_S2_T1
// asserts that a citation setting both path and activity_entry_id is rejected,
// matching CriterionResult.Valid()'s mutual-exclusivity check.
func TestConformanceAssessmentSchema_RejectsCitationWithBothPathAndActivityEntryID_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "Implemented",
      "citations": [
        {"path": "src/main.go", "activity_entry_id": "0"}
      ]
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateSchemaRejects(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestConformanceAssessmentSchema_RejectsEmptyCitation_REQ_TOPTIER_S2_T1 asserts
// that a citation object with neither path nor activity_entry_id is rejected.
func TestConformanceAssessmentSchema_RejectsEmptyCitation_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "Implemented",
      "citations": [
        {}
      ]
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateSchemaRejects(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestConformanceAssessmentSchema_RejectsNonNumericActivityEntryID_REQ_TOPTIER_S2_T1
// asserts that activity citations use the numeric raw entry IDs required by
// ValidateActivityCitations.
func TestConformanceAssessmentSchema_RejectsNonNumericActivityEntryID_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	assessmentJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "Implemented",
      "citations": [
        {"activity_entry_id": "index:0"}
      ]
    }
  ],
  "contract_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "delivery_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}`

	validateSchemaRejects(t, "conformance-assessment.schema.json", assessmentJSON)
}

// TestReviewBundleSchema_AllowsEmptyDefinitionOfDone_REQ_TOPTIER_S2_T1 asserts
// that the review-bundle schema does not require definition_of_done to be
// non-empty, matching the CLI which never enforces that (ReviewBundle.Valid()
// doesn't check it; apply.go only warns advisory).
func TestReviewBundleSchema_AllowsEmptyDefinitionOfDone_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	bundleJSON := `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "issue": {
    "id": "TASK-001",
    "type": "task",
    "title": "Test task",
    "outcome": "Implemented feature X"
  },
  "contract": {
    "definition_of_done": "",
    "acceptance": []
  },
  "delivery": {
    "base_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "head_sha": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "changed_files": []
  },
  "fingerprints": {
    "contract": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "delivery": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  }
}`

	validateAgainstSchema(t, "review-bundle.schema.json", bundleJSON)
}

// TestPlanSchema_AllowsArbitraryPriorityString_REQ_TOPTIER_S2_T1 asserts that
// the plan schema accepts any string for priority, matching the CLI which
// never validates priority values anywhere.
func TestPlanSchema_AllowsArbitraryPriorityString_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	planJSON := `{
  "version": 1,
  "title": "Feature decomposition",
  "issues": [
    {
      "id": "FEATURE-S1-T1",
      "title": "Implement core logic",
      "type": "task",
      "priority": "urgent-ish"
    }
  ]
}`

	validateAgainstSchema(t, "plan.schema.json", planJSON)
}

// TestPlanSchema_RejectsNonArrayAcceptance_REQ_TOPTIER_S2_T1 asserts that the
// plan schema now constrains acceptance to an array of string-or-object,
// matching the constraint already declared by cmd/armature/decompose.go
// --schema output.
func TestPlanSchema_RejectsNonArrayAcceptance_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	planJSON := `{
  "version": 1,
  "title": "Feature decomposition",
  "issues": [
    {
      "id": "FEATURE-S1-T1",
      "title": "Implement core logic",
      "type": "task",
      "acceptance": "not an array"
    }
  ]
}`

	validateSchemaRejects(t, "plan.schema.json", planJSON)
}

// TestActivityIndexSchema_InvalidExample_REQ_TOPTIER_S2_T1 asserts that the
// activity-index schema rejects a document missing the required
// "entry_count" field, proving the required-fields constraint is actually
// enforced.
func TestActivityIndexSchema_InvalidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	indexJSON := `{
  "schema_version": 1,
  "log_path": "/path/to/armature-activity.log",
  "log_digest": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "delivery_head_count": 2,
  "earlier_count": 0,
  "entries": [
    {
      "id": "0",
      "command": "go test ./...",
      "exit_status": 0,
      "head_anchor": true,
      "category": "test",
      "log_pointer": "0"
    }
  ]
}`

	validateSchemaRejects(t, "activity-index.schema.json", indexJSON)
}
