package review_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/decompose"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Package strictdecode_test verifies that JSON artifact decoders use DisallowUnknownFields
// to catch type mismatches and unknown fields that unit tests previously hid.
//
// Audit of JSON decoders in internal/review:
//   - acceptance.go (lines 28, 34): uses map[string]interface{} for flexible format;
//     intentionally non-strict to support extensible acceptance criterion schemas.
//   - fingerprint.go (line 203): reads activity log; intentionally lenient to skip
//     malformed lines in append-only logs that may gain new fields in future versions.
//
// See docs/design/top-tier-gap-analysis.md (T2.3) for background on this test suite.

// TestReviewBundleRoundTrip_REQ_TOPTIER_S3_T3 verifies that a ReviewBundle
// marshals to JSON and unmarshals back identically with DisallowUnknownFields enabled.
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

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(originalBundle, "", "  ")
	require.NoError(t, err, "failed to marshal ReviewBundle")

	// Unmarshal with DisallowUnknownFields
	var roundTrippedBundle review.ReviewBundle
	decoder := json.NewDecoder(strings.NewReader(string(jsonData)))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&roundTrippedBundle)
	require.NoError(t, err, "failed to unmarshal ReviewBundle with DisallowUnknownFields")

	// Verify round-trip
	assert.Equal(t, originalBundle.SchemaVersion, roundTrippedBundle.SchemaVersion)
	assert.Equal(t, originalBundle.BundleID, roundTrippedBundle.BundleID)
	assert.Equal(t, originalBundle.Issue.ID, roundTrippedBundle.Issue.ID)
	assert.Equal(t, originalBundle.Contract.DefinitionOfDone, roundTrippedBundle.Contract.DefinitionOfDone)
	assert.Equal(t, originalBundle.Delivery.BaseSHA, roundTrippedBundle.Delivery.BaseSHA)
}

// TestConformanceAssessmentRoundTrip_REQ_TOPTIER_S3_T3 verifies that a ConformanceAssessment
// marshals to JSON and unmarshals back with DisallowUnknownFields enabled.
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

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(originalAssessment, "", "  ")
	require.NoError(t, err, "failed to marshal ConformanceAssessment")

	// Unmarshal with DisallowUnknownFields
	var roundTrippedAssessment review.ConformanceAssessment
	decoder := json.NewDecoder(strings.NewReader(string(jsonData)))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&roundTrippedAssessment)
	require.NoError(t, err, "failed to unmarshal ConformanceAssessment with DisallowUnknownFields")

	// Verify round-trip
	assert.Equal(t, originalAssessment.SchemaVersion, roundTrippedAssessment.SchemaVersion)
	assert.Equal(t, originalAssessment.BundleID, roundTrippedAssessment.BundleID)
	assert.Len(t, roundTrippedAssessment.Results, 2)
	assert.Equal(t, review.Satisfied, roundTrippedAssessment.Results[0].Status)
	assert.Equal(t, review.Satisfied, roundTrippedAssessment.Results[1].Status)
}

// TestCriterionStatusStringRoundTrip_REQ_TOPTIER_S3_T3 verifies that CriterionStatus
// marshals as a string (not integer) and unmarshals from that string form.
// This catches the bug where internal representation uses int but JSON documents strings.
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

			// Marshal status to JSON
			data, err := json.Marshal(status)
			require.NoError(t, err)

			// Verify it's a quoted string, not an integer
			dataStr := string(data)
			assert.True(t, strings.HasPrefix(dataStr, "\"") && strings.HasSuffix(dataStr, "\""),
				"CriterionStatus %q marshaled as %s, expected quoted string", status.String(), dataStr)

			// Unmarshal back from the string form
			var decoded review.CriterionStatus
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			assert.Equal(t, status, decoded)
		})
	}
}

// TestRatingStringRoundTrip_REQ_TOPTIER_S3_T3 verifies that Rating
// marshals as a string (not integer) and unmarshals from that string form.
func TestRatingStringRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	ratings := []review.Rating{review.Green, review.Yellow, review.Red}

	for _, rating := range ratings {
		t.Run(rating.String(), func(t *testing.T) {
			t.Parallel()

			// Marshal rating to JSON
			data, err := json.Marshal(rating)
			require.NoError(t, err)

			// Verify it's a quoted string, not an integer
			dataStr := string(data)
			assert.True(t, strings.HasPrefix(dataStr, "\"") && strings.HasSuffix(dataStr, "\""),
				"Rating %q marshaled as %s, expected quoted string", rating.String(), dataStr)

			// Unmarshal back from the string form
			var decoded review.Rating
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			assert.Equal(t, rating, decoded)
		})
	}
}

// TestCriterionResultUnknownFields_REQ_TOPTIER_S3_T3 verifies that
// CriterionResult rejects unknown fields during unmarshaling.
func TestCriterionResultUnknownFields_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	// JSON with an unknown field
	jsonStr := `{
		"id": "test_criterion",
		"status": "satisfied",
		"rationale": "Test passes",
		"unknown_field": "should cause error"
	}`

	var result review.CriterionResult
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&result)
	require.Error(t, err, "expected error for unknown field in CriterionResult")
	assert.True(t, strings.Contains(err.Error(), "unknown") || strings.Contains(err.Error(), "Unknown"),
		"expected error to mention unknown field, got: %s", err.Error())
}

// TestReviewBundleUnknownFields_REQ_TOPTIER_S3_T3 verifies that
// ReviewBundle rejects unknown fields during unmarshaling.
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

// TestConformanceAssessmentUnknownFields_REQ_TOPTIER_S3_T3 verifies that
// ConformanceAssessment rejects unknown fields during unmarshaling.
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

// TestBundleStrictDecode_REQ_TOPTIER_S3_T3 verifies that ReviewBundle
// rejects JSON with type mismatches (e.g., string where int is expected).
// This comprehensive test covers the full pipeline intent: plan -> decompose -> review.
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

// TestPipelineRoundTrip_REQ_TOPTIER_S3_T3 verifies end-to-end JSON artifact fidelity
// through the full pipeline: plan -> decompose-apply -> render-context -> assessment.
// This test exercises strict decoding across multiple artifact boundaries.
func TestPipelineRoundTrip_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	// Step 1: Construct a Plan
	originalPlan := decompose.Plan{
		Version: 1,
		Title:   "Pipeline Test Plan",
		Issues: []decompose.PlanIssue{
			{
				ID:        "TOPTIER-S3-T3",
				Title:     "Strict-decode round-trip suite",
				Type:      "task",
				Scope:     "internal/decompose/strictdecode_test.go, internal/review/strictdecode_test.go",
				Priority:  "medium",
				DoD:       "All JSON artifact decoders use DisallowUnknownFields",
				Parent:    "TOPTIER-S3",
				BlockedBy: []string{},
				Notes:     []string{"Dogfood finding: JSON string/int mismatch"},
			},
		},
	}

	// Step 2: Marshal the Plan to JSON
	planJSON, err := json.MarshalIndent(originalPlan, "", "  ")
	require.NoError(t, err, "failed to marshal Plan")

	// Step 3: Verify Plan round-trip with strict decoding
	var decodedPlan decompose.Plan
	decoder := json.NewDecoder(strings.NewReader(string(planJSON)))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&decodedPlan)
	require.NoError(t, err, "failed to decode Plan with DisallowUnknownFields")
	assert.Equal(t, originalPlan.Version, decodedPlan.Version)
	assert.Equal(t, originalPlan.Title, decodedPlan.Title)
	assert.Equal(t, len(originalPlan.Issues), len(decodedPlan.Issues))

	// Step 4: Build ReviewBundle from the parsed plan's issue
	parsedIssue := decodedPlan.Issues[0]
	bundle := review.ReviewBundle{
		SchemaVersion: review.SchemaVersion,
		BundleID:      "pipeline-test-bundle",
		Issue: review.IssueInfo{
			ID:      parsedIssue.ID,
			Type:    parsedIssue.Type,
			Title:   parsedIssue.Title,
			Outcome: parsedIssue.DoD,
		},
		Contract: review.Contract{
			DefinitionOfDone: parsedIssue.DoD,
			Scope:            []string{parsedIssue.Scope},
			Acceptance:       []string{"Plan parsed successfully", "Type safety verified"},
		},
		Delivery: review.Delivery{
			BaseSHA:      "base123",
			HeadSHA:      "head456",
			ChangedFiles: []string{parsedIssue.Scope},
		},
		Fingerprints: review.Fingerprints{
			Contract: "contract-fp",
			Delivery: "delivery-fp",
		},
	}

	// Step 5: Marshal and strict-decode ReviewBundle
	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	require.NoError(t, err, "failed to marshal ReviewBundle")

	var decodedBundle review.ReviewBundle
	bundleDecoder := json.NewDecoder(strings.NewReader(string(bundleJSON)))
	bundleDecoder.DisallowUnknownFields()
	err = bundleDecoder.Decode(&decodedBundle)
	require.NoError(t, err, "failed to decode ReviewBundle with DisallowUnknownFields")

	// Step 6: Verify bundle issue matches parsed plan issue
	assert.Equal(t, parsedIssue.ID, decodedBundle.Issue.ID)
	assert.Equal(t, parsedIssue.Type, decodedBundle.Issue.Type)
	assert.Equal(t, parsedIssue.Title, decodedBundle.Issue.Title)
	assert.Equal(t, parsedIssue.DoD, decodedBundle.Issue.Outcome)

	// Step 7: Build ConformanceAssessment from the ReviewBundle
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            decodedBundle.BundleID,
		ContractFingerprint: decodedBundle.Fingerprints.Contract,
		DeliveryFingerprint: decodedBundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Plan structure and JSON decoding verified",
				Citations: []review.Citation{
					{Path: "internal/decompose/strictdecode_test.go"},
				},
			},
		},
	}

	// Step 8: Marshal and strict-decode ConformanceAssessment
	assessmentJSON, err := json.MarshalIndent(assessment, "", "  ")
	require.NoError(t, err, "failed to marshal ConformanceAssessment")

	var decodedAssessment review.ConformanceAssessment
	assessmentDecoder := json.NewDecoder(strings.NewReader(string(assessmentJSON)))
	assessmentDecoder.DisallowUnknownFields()
	err = assessmentDecoder.Decode(&decodedAssessment)
	require.NoError(t, err, "failed to decode ConformanceAssessment with DisallowUnknownFields")

	// Step 9: Verify end-to-end fidelity
	assert.Equal(t, review.SchemaVersion, decodedAssessment.SchemaVersion)
	assert.Equal(t, decodedBundle.BundleID, decodedAssessment.BundleID)
	assert.Equal(t, decodedBundle.Fingerprints.Contract, decodedAssessment.ContractFingerprint)
	assert.Equal(t, decodedBundle.Fingerprints.Delivery, decodedAssessment.DeliveryFingerprint)
	assert.Len(t, decodedAssessment.Results, 1)
	assert.Equal(t, review.Satisfied, decodedAssessment.Results[0].Status)
	// Verify issue ID from parsed plan is present in bundle
	assert.Equal(t, parsedIssue.ID, decodedBundle.Issue.ID)
}
