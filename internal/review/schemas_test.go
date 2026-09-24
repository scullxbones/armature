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

func TestArtifactSchemas_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	schemasDir := filepath.Join(repoRoot, "docs", "schemas")

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

			_, err := os.Stat(schemaPath)
			require.NoError(t, err, "schema file should exist: %s", schemaPath)

			data, err := os.ReadFile(schemaPath)
			require.NoError(t, err, "should be able to read schema file")

			var schemaObj any
			err = json.Unmarshal(data, &schemaObj)
			require.NoError(t, err, "schema file should contain valid JSON")

			schema, ok := schemaObj.(map[string]any)
			require.True(t, ok, "schema should be a JSON object")

			require.Contains(t, schema, "$schema", "schema should have $schema field")
			require.Contains(t, schema, "title", "schema should have title field")
			require.Contains(t, schema, "type", "schema should have type field")
			require.Contains(t, schema, "properties", "schema should have properties field")

			compiler := jsonschema.NewCompiler()
			require.NoError(t, compiler.AddResource(schemaFile, bytes.NewReader(data)))
			_, err = compiler.Compile(schemaFile)
			require.NoError(t, err, "schema should compile as valid JSON Schema: %s", schemaFile)
		})
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	require.NoError(t, err, "should be able to get working directory")

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

func TestReviewBundleSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
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

func TestConformanceAssessmentSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

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

func TestActivityIndexSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

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

func TestPlanSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

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
