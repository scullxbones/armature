package review_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewBundleRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	originalBundle := review.ReviewBundle{
		SchemaVersion: review.SchemaVersion,
		BundleID:      "bundle-123",
		Issue: review.IssueInfo{
			ID:      "TOPTIER-S3-T3",
			Type:    "task",
			Title:   "Strict-decode round-trip suite",
			Outcome: "All JSON decoders use DisallowUnknownFields",
		},
		Contract: review.Contract{
			DefinitionOfDone: "All JSON artifact decoders use DisallowUnknownFields",
			Scope:            []string{"internal/decompose/strictdecode_test.go", "internal/review/strictdecode_test.go"},
			Acceptance:       []string{"Round-trip tests cover full flow", "Type mismatches are caught"},
		},
		Delivery: review.Delivery{
			BaseSHA:      "abc123",
			HeadSHA:      "def456",
			ChangedFiles: []string{"internal/decompose/plan.go", "internal/review/types.go"},
		},
		Fingerprints: review.Fingerprints{
			Contract: "contract-fp",
			Delivery: "delivery-fp",
		},
	}

	jsonData, err := json.MarshalIndent(originalBundle, "", "  ")
	require.NoError(t, err, "failed to marshal ReviewBundle")

	var roundTrippedBundle review.ReviewBundle
	decoder := json.NewDecoder(strings.NewReader(string(jsonData)))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&roundTrippedBundle)
	require.NoError(t, err, "failed to unmarshal ReviewBundle with DisallowUnknownFields")

	assert.Equal(t, originalBundle.SchemaVersion, roundTrippedBundle.SchemaVersion)
	assert.Equal(t, originalBundle.BundleID, roundTrippedBundle.BundleID)
	assert.Equal(t, originalBundle.Issue.ID, roundTrippedBundle.Issue.ID)
	assert.Equal(t, originalBundle.Contract.DefinitionOfDone, roundTrippedBundle.Contract.DefinitionOfDone)
	assert.Equal(t, originalBundle.Delivery.BaseSHA, roundTrippedBundle.Delivery.BaseSHA)
}

func TestConformanceAssessmentRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	originalAssessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "contract-fp",
		DeliveryFingerprint: "delivery-fp",
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "All decoders updated and tested",
				Citations: []review.Citation{
					{Path: "internal/decompose/plan.go", Line: 41},
					{Path: "internal/review/types.go", Line: 54},
				},
			},
			{
				ID:        "acceptance[0]",
				Status:    review.Satisfied,
				Rationale: "Round-trip tests written for both packages",
				Citations: []review.Citation{
					{Path: "internal/decompose/strictdecode_test.go"},
					{Path: "internal/review/strictdecode_test.go"},
				},
			},
		},
	}

	jsonData, err := json.MarshalIndent(originalAssessment, "", "  ")
	require.NoError(t, err, "failed to marshal ConformanceAssessment")

	var roundTrippedAssessment review.ConformanceAssessment
	decoder := json.NewDecoder(strings.NewReader(string(jsonData)))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&roundTrippedAssessment)
	require.NoError(t, err, "failed to unmarshal ConformanceAssessment with DisallowUnknownFields")

	assert.Equal(t, originalAssessment.SchemaVersion, roundTrippedAssessment.SchemaVersion)
	assert.Equal(t, originalAssessment.BundleID, roundTrippedAssessment.BundleID)
	assert.Len(t, roundTrippedAssessment.Results, 2)
	assert.Equal(t, review.Satisfied, roundTrippedAssessment.Results[0].Status)
	assert.Equal(t, review.Satisfied, roundTrippedAssessment.Results[1].Status)
}

func TestCriterionStatusStringRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	statuses := []review.CriterionStatus{
		review.Satisfied,
		review.PartiallySatisfied,
		review.NotSatisfied,
		review.Indeterminate,
	}

	for _, status := range statuses {
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(status)
			require.NoError(t, err)

			dataStr := string(data)
			assert.True(t, strings.HasPrefix(dataStr, "\"") && strings.HasSuffix(dataStr, "\""),
				"CriterionStatus %q marshaled as %s, expected quoted string", status.String(), dataStr)

			var decoded review.CriterionStatus
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			assert.Equal(t, status, decoded)
		})
	}
}

func TestRatingStringRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	ratings := []review.Rating{review.Green, review.Yellow, review.Red}

	for _, rating := range ratings {
		t.Run(rating.String(), func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(rating)
			require.NoError(t, err)

			dataStr := string(data)
			assert.True(t, strings.HasPrefix(dataStr, "\"") && strings.HasSuffix(dataStr, "\""),
				"Rating %q marshaled as %s, expected quoted string", rating.String(), dataStr)

			var decoded review.Rating
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			assert.Equal(t, rating, decoded)
		})
	}
}

func TestCriterionResultAllowsSchemaValidExtensionFields_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	jsonStr := `{
		"id": "test_criterion",
		"status": "satisfied",
		"rationale": "Test passes",
		"extension_field": "schema-valid metadata"
	}`

	var result review.CriterionResult
	err := json.Unmarshal([]byte(jsonStr), &result)
	require.NoError(t, err, "schema-valid extension field must not be rejected")
	assert.Equal(t, "test_criterion", result.ID)
	assert.Equal(t, review.Satisfied, result.Status)
}

func TestCriterionResultAllowsSchemaValidExtensionFieldsOnCitation_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	jsonStr := `{
		"id": "test_criterion",
		"status": "satisfied",
		"rationale": "Test passes",
		"citations": [
			{
				"path": "internal/review/types.go",
				"line": 42,
				"confidence": "high"
			}
		]
	}`

	var result review.CriterionResult
	err := json.Unmarshal([]byte(jsonStr), &result)
	require.NoError(t, err, "schema-valid extension field on citation must not be rejected")
	require.Len(t, result.Citations, 1)
	assert.Equal(t, "internal/review/types.go", result.Citations[0].Path)
	assert.Equal(t, 42, result.Citations[0].Line)
}

func TestCriterionResultMissingStatusRejected_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	jsonStr := `{
		"id": "test_criterion",
		"rationale": "Test passes"
	}`

	var result review.CriterionResult
	err := json.Unmarshal([]byte(jsonStr), &result)
	require.Error(t, err, "expected error for missing required status field")
	assert.Contains(t, err.Error(), "status")
}

func TestReviewBundleUnknownFields_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	jsonStr := `{
		"schema_version": 1,
		"bundle_id": "test-bundle",
		"issue": {"id": "TASK-1", "type": "task", "title": "Test", "outcome": "Done"},
		"contract": {"definition_of_done": "Test", "acceptance": []},
		"delivery": {"base_sha": "abc", "head_sha": "def", "changed_files": []},
		"fingerprints": {"contract": "c", "delivery": "d"},
		"unknown_field": "should cause error"
	}`

	var bundle review.ReviewBundle
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&bundle)
	require.Error(t, err, "expected error for unknown field in ReviewBundle")
	assert.True(t, strings.Contains(err.Error(), "unknown") || strings.Contains(err.Error(), "Unknown"),
		"expected error to mention unknown field, got: %s", err.Error())
}

func TestConformanceAssessmentUnknownFields_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	jsonStr := `{
		"schema_version": 1,
		"bundle_id": "test-bundle",
		"results": [{"id": "dod", "status": "satisfied", "rationale": "OK"}],
		"contract_fingerprint": "c",
		"delivery_fingerprint": "d",
		"unknown_field": "should cause error"
	}`

	var assessment review.ConformanceAssessment
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&assessment)
	require.Error(t, err, "expected error for unknown field in ConformanceAssessment")
	assert.True(t, strings.Contains(err.Error(), "unknown") || strings.Contains(err.Error(), "Unknown"),
		"expected error to mention unknown field, got: %s", err.Error())
}

func TestBundleStrictDecode_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		jsonContent string
		expectError bool
		errorMsg    string
	}{
		{
			name: "schema_version_as_string_should_fail",
			jsonContent: `{
				"schema_version": "1",
				"bundle_id": "test-bundle",
				"issue": {"id": "TASK-1", "type": "task", "title": "Test", "outcome": "Done"},
				"contract": {"definition_of_done": "Test", "acceptance": []},
				"delivery": {"base_sha": "abc", "head_sha": "def", "changed_files": []},
				"fingerprints": {"contract": "c", "delivery": "d"}
			}`,
			expectError: true,
			errorMsg:    "type",
		},
		{
			name: "valid_bundle_should_pass",
			jsonContent: `{
				"schema_version": 1,
				"bundle_id": "test-bundle",
				"issue": {"id": "TASK-1", "type": "task", "title": "Test", "outcome": "Done"},
				"contract": {"definition_of_done": "Test", "acceptance": []},
				"delivery": {"base_sha": "abc", "head_sha": "def", "changed_files": []},
				"fingerprints": {"contract": "c", "delivery": "d"}
			}`,
			expectError: false,
			errorMsg:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var bundle review.ReviewBundle
			decoder := json.NewDecoder(strings.NewReader(tc.jsonContent))
			decoder.DisallowUnknownFields()
			err := decoder.Decode(&bundle)

			if tc.expectError {
				require.Error(t, err, "expected error for %s", tc.name)
				assert.True(t, strings.Contains(err.Error(), tc.errorMsg) ||
					strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errorMsg)),
					"expected error to contain %q, got: %s", tc.errorMsg, err.Error())
			} else {
				require.NoError(t, err, "expected no error for %s, but got: %v", tc.name, err)
			}
		})
	}
}
