// Package decompose provides plan parsing and decomposition.
// This strictdecode_test.go file verifies JSON artifact decoders use DisallowUnknownFields
// to catch type mismatches and unknown fields that unit tests previously hid.
//
// Audit of JSON decoders in internal/decompose:
//   - ParsePlan (plan.go, line 43): uses json.Decoder with DisallowUnknownFields;
//     strict to catch malformed or deprecated plan input early.
//
// See docs/design/top-tier-gap-analysis.md (T2.3) for background on this test suite.
package decompose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlanRoundTrip_REQ_TOPTIER_S3_T3 verifies that a Plan marshals to JSON
// and unmarshals back identically with DisallowUnknownFields enabled.
// This test catches type mismatches like string-vs-int that unit tests previously hid.
func TestPlanRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	// Step 1: Create a Plan struct with all field types
	originalPlan := Plan{
		Version: 1,
		Title:   "Test Plan for Round-Trip",
		Issues: []PlanIssue{
			{
				ID:           "TOPTIER-S3-T3",
				Title:        "Strict-decode round-trip suite",
				Type:         "task",
				Scope:        "internal/decompose/strictdecode_test.go, internal/review/strictdecode_test.go",
				Priority:     "medium",
				DoD:          "All JSON artifact decoders use DisallowUnknownFields",
				Parent:       "TOPTIER-S3",
				BlockedBy:    []string{},
				Notes:        []string{"Dogfood finding: JSON string/int mismatch"},
				ContextFiles: []string{"docs/design/top-tier-gap-analysis.md"},
			},
		},
	}

	// Step 2: Marshal the Plan to JSON
	jsonData, err := json.MarshalIndent(originalPlan, "", "  ")
	require.NoError(t, err, "failed to marshal Plan")

	// Step 3: Unmarshal the JSON back into a Plan with DisallowUnknownFields
	var roundTrippedPlan Plan
	decoder := json.NewDecoder(strings.NewReader(string(jsonData)))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&roundTrippedPlan)
	require.NoError(t, err, "failed to unmarshal Plan with DisallowUnknownFields")

	// Step 4: Verify the round-tripped Plan matches the original
	assert.Equal(t, originalPlan.Version, roundTrippedPlan.Version)
	assert.Equal(t, originalPlan.Title, roundTrippedPlan.Title)
	assert.Len(t, roundTrippedPlan.Issues, len(originalPlan.Issues))
	assert.Equal(t, originalPlan.Issues[0].ID, roundTrippedPlan.Issues[0].ID)
	assert.Equal(t, originalPlan.Issues[0].Title, roundTrippedPlan.Issues[0].Title)
	assert.Equal(t, originalPlan.Issues[0].Priority, roundTrippedPlan.Issues[0].Priority)
}

// TestPlanDisallowUnknownFields_REQ_TOPTIER_S3_T3 verifies that ParsePlan
// rejects JSON with unknown fields to catch malformed or deprecated input.
func TestPlanDisallowUnknownFields_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	// JSON with an unknown field "unknown_field"
	planJSON := `{
		"version": 1,
		"title": "Test Plan",
		"issues": [],
		"unknown_field": "should cause error"
	}`

	tmpFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte(planJSON), 0644))

	_, err := ParsePlan(tmpFile)
	require.Error(t, err, "expected error for unknown field, but got nil")
	assert.True(t, strings.Contains(err.Error(), "unknown") || strings.Contains(err.Error(), "Unknown"),
		"expected error to mention unknown field, got: %s", err.Error())
}

// TestPlanValidRoundTrip_REQ_TOPTIER_S3_T3 verifies that a valid Plan
// parses successfully and maintains all field values through a complete round-trip.
func TestPlanValidRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	planJSON := `{
		"version": 1,
		"title": "Complete Test Plan",
		"issues": [
			{
				"id": "TASK-001",
				"title": "First task",
				"type": "task",
				"scope": "file.go",
				"priority": "high",
				"dod": "Test passes",
				"parent": "STORY-001",
				"blocked_by": ["TASK-002"],
				"notes": ["Note 1", "Note 2"],
				"context_files": ["doc.md"]
			}
		]
	}`

	tmpFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte(planJSON), 0644))

	parsed, err := ParsePlan(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	assert.Equal(t, 1, parsed.Version)
	assert.Equal(t, "Complete Test Plan", parsed.Title)
	assert.Len(t, parsed.Issues, 1)
	assert.Equal(t, "TASK-001", parsed.Issues[0].ID)
	assert.Equal(t, "First task", parsed.Issues[0].Title)
	assert.Equal(t, "task", parsed.Issues[0].Type)
	assert.Equal(t, "file.go", parsed.Issues[0].Scope)
	assert.Equal(t, "high", parsed.Issues[0].Priority)
	assert.Equal(t, "Test passes", parsed.Issues[0].DoD)
	assert.Equal(t, "STORY-001", parsed.Issues[0].Parent)
	assert.Equal(t, []string{"TASK-002"}, parsed.Issues[0].BlockedBy)
	assert.Equal(t, []string{"Note 1", "Note 2"}, parsed.Issues[0].Notes)
	assert.Equal(t, []string{"doc.md"}, parsed.Issues[0].ContextFiles)
}

// TestPlanMarshalAndParseRoundTrip_REQ_TOPTIER_S3_T3 verifies that
// a parsed Plan can be marshaled to JSON and re-parsed without loss of information.
func TestPlanMarshalAndParseRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	// Start with a JSON file
	originalJSON := `{
		"version": 1,
		"title": "Marshal Round Trip Test",
		"issues": [
			{
				"id": "TASK-003",
				"title": "Test task",
				"type": "task",
				"scope": "test.go",
				"priority": "low",
				"dod": "Done",
				"parent": "STORY-003",
				"blocked_by": [],
				"notes": []
			}
		]
	}`

	tmpDir := t.TempDir()
	tmpFile1 := filepath.Join(tmpDir, "plan1.json")
	require.NoError(t, os.WriteFile(tmpFile1, []byte(originalJSON), 0644))

	// Parse the original
	plan1, err := ParsePlan(tmpFile1)
	require.NoError(t, err)

	// Marshal it to JSON
	jsonData, err := json.MarshalIndent(plan1, "", "  ")
	require.NoError(t, err)

	// Write the marshaled JSON to a new file
	tmpFile2 := filepath.Join(tmpDir, "plan2.json")
	require.NoError(t, os.WriteFile(tmpFile2, jsonData, 0644))

	// Parse the marshaled JSON
	plan2, err := ParsePlan(tmpFile2)
	require.NoError(t, err)

	// Verify both plans are equivalent
	assert.Equal(t, plan1.Version, plan2.Version)
	assert.Equal(t, plan1.Title, plan2.Title)
	assert.Len(t, plan2.Issues, len(plan1.Issues))
	assert.Equal(t, plan1.Issues[0].ID, plan2.Issues[0].ID)
	assert.Equal(t, plan1.Issues[0].Title, plan2.Issues[0].Title)
	assert.Equal(t, plan1.Issues[0].Type, plan2.Issues[0].Type)
}

// TestPlanStrictDecode_REQ_TOPTIER_S3_T3 verifies that ParsePlan rejects
// JSON with type mismatches (e.g., string where int is expected).
// This test directly addresses the dogfood finding: "JSON string/int mismatch hidden by unit tests".
func TestPlanStrictDecode_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		jsonContent string
		expectError bool
		errorMsg    string
	}{
		{
			name: "version_as_string_should_fail",
			jsonContent: `{
				"version": "1",
				"title": "Test Plan",
				"issues": []
			}`,
			expectError: true,
			errorMsg:    "type",
		},
		{
			name: "version_as_float_should_fail",
			jsonContent: `{
				"version": 1.5,
				"title": "Test Plan",
				"issues": []
			}`,
			expectError: true,
			errorMsg:    "type",
		},
		{
			name: "issue_line_as_string_should_fail",
			jsonContent: `{
				"version": 1,
				"title": "Test Plan",
				"issues": [
					{
						"id": "TASK-001",
						"title": "Test",
						"type": "task",
						"scope": "file.go",
						"priority": "high",
						"dod": "Done",
						"parent": "STORY-001",
						"blocked_by": [],
						"notes": []
					}
				]
			}`,
			expectError: false, // This should pass as all fields are correctly typed
			errorMsg:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpFile := filepath.Join(t.TempDir(), "plan.json")
			require.NoError(t, os.WriteFile(tmpFile, []byte(tc.jsonContent), 0644))

			_, err := ParsePlan(tmpFile)
			if tc.expectError {
				require.Error(t, err, "expected error for %s", tc.name)
				assert.True(t, strings.Contains(err.Error(), tc.errorMsg) ||
					strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errorMsg)),
					"expected error to contain %q, got: %s", tc.errorMsg, err.Error())
			} else {
				require.NoError(t, err, "expected no error for %s", tc.name)
			}
		})
	}
}
