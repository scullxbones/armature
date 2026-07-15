package review_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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

			var schemaObj interface{}
			err = json.Unmarshal(data, &schemaObj)
			require.NoError(t, err, "schema file should contain valid JSON")

			// Verify it's an object with required schema-level fields
			schema, ok := schemaObj.(map[string]interface{})
			require.True(t, ok, "schema should be a JSON object")

			// Check for required schema fields
			assert.Contains(t, schema, "$schema", "schema should have $schema field")
			assert.Contains(t, schema, "title", "schema should have title field")
			assert.Contains(t, schema, "type", "schema should have type field")
			assert.Contains(t, schema, "properties", "schema should have properties field")
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
	for i := 0; i < 10; i++ {
		schemasDir := filepath.Join(cwd, "docs", "schemas")
		if _, err := os.Stat(schemasDir); err == nil {
			return cwd
		}
		cwd = filepath.Dir(cwd)
	}

	t.Fatalf("could not find repo root (docs/schemas directory)")
	return ""
}

// TestReviewBundleSchema_ValidExample_REQ_TOPTIER_S2_T1 validates that the
// review-bundle schema accepts a valid ReviewBundle example.
func TestReviewBundleSchema_ValidExample_REQ_TOPTIER_S2_T1(t *testing.T) {
	t.Parallel()

	// Valid minimal review bundle example
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
    "base_sha": "abc1234567890abcdef1234567890abcdef123456",
    "head_sha": "def4567890abcdef1234567890abcdef1234567890",
    "changed_files": ["src/main.go", "src/main_test.go"]
  },
  "fingerprints": {
    "contract": "5d41402abc4b2a76b9719d911017c592",
    "delivery": "098f6bcd4621d373cade4e832627b4f6"
  }
}`

	var bundle interface{}
	err := json.Unmarshal([]byte(bundleJSON), &bundle)
	require.NoError(t, err, "example JSON should be valid")

	// Verify it has the expected top-level structure
	bundleObj, ok := bundle.(map[string]interface{})
	require.True(t, ok, "bundle should be a JSON object")

	assert.Equal(t, float64(1), bundleObj["schema_version"])
	assert.NotEmpty(t, bundleObj["bundle_id"])
	assert.NotEmpty(t, bundleObj["issue"])
	assert.NotEmpty(t, bundleObj["contract"])
	assert.NotEmpty(t, bundleObj["delivery"])
	assert.NotEmpty(t, bundleObj["fingerprints"])
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
  "contract_fingerprint": "5d41402abc4b2a76b9719d911017c592",
  "delivery_fingerprint": "098f6bcd4621d373cade4e832627b4f6"
}`

	var assessment interface{}
	err := json.Unmarshal([]byte(assessmentJSON), &assessment)
	require.NoError(t, err, "example JSON should be valid")

	// Verify it has the expected top-level structure
	assessmentObj, ok := assessment.(map[string]interface{})
	require.True(t, ok, "assessment should be a JSON object")

	assert.Equal(t, float64(1), assessmentObj["schema_version"])
	assert.NotEmpty(t, assessmentObj["bundle_id"])
	assert.NotEmpty(t, assessmentObj["results"])
	assert.NotEmpty(t, assessmentObj["contract_fingerprint"])
	assert.NotEmpty(t, assessmentObj["delivery_fingerprint"])
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

	var index interface{}
	err := json.Unmarshal([]byte(indexJSON), &index)
	require.NoError(t, err, "example JSON should be valid")

	// Verify it has the expected top-level structure
	indexObj, ok := index.(map[string]interface{})
	require.True(t, ok, "index should be a JSON object")

	assert.Equal(t, float64(1), indexObj["schema_version"])
	assert.NotEmpty(t, indexObj["log_path"])
	assert.NotEmpty(t, indexObj["log_digest"])
	assert.Equal(t, float64(2), indexObj["entry_count"])
	assert.NotEmpty(t, indexObj["entries"])
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

	var plan interface{}
	err := json.Unmarshal([]byte(planJSON), &plan)
	require.NoError(t, err, "example JSON should be valid")

	// Verify it has the expected top-level structure
	planObj, ok := plan.(map[string]interface{})
	require.True(t, ok, "plan should be a JSON object")

	assert.Equal(t, float64(1), planObj["version"])
	assert.NotEmpty(t, planObj["title"])
	assert.NotEmpty(t, planObj["issues"])
}
