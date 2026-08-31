package review_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCriterionStatus_JSONRoundTrip verifies that CriterionStatus marshals as a string
// and can be decoded from the string values that the armature-reviewer skill emits.
func TestCriterionStatus_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	statuses := []review.CriterionStatus{
		review.Satisfied, review.PartiallySatisfied, review.NotSatisfied, review.Indeterminate,
	}
	for _, status := range statuses {
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(status)
			require.NoError(t, err)
			// Must encode as a quoted string, not an integer.
			assert.Equal(t, `"`+status.String()+`"`, string(data))
			var decoded review.CriterionStatus
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, status, decoded)
		})
	}
}

// TestCriterionStatus_UnmarshalJSON_SkillOutput verifies that skill output strings
// like {"status":"satisfied"} can be decoded.
func TestCriterionStatus_UnmarshalJSON_SkillOutput(t *testing.T) {
	t.Parallel()
	input := `{"status":"satisfied"}`
	var result struct {
		Status review.CriterionStatus `json:"status"`
	}
	err := json.Unmarshal([]byte(input), &result)
	require.NoError(t, err)
	assert.Equal(t, review.Satisfied, result.Status)
}

// TestRating_JSONRoundTrip verifies that Rating marshals as a string and can be
// decoded from the string values that the armature-reviewer skill emits.
func TestRating_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	ratings := []review.Rating{review.Green, review.Yellow, review.Red}
	for _, rating := range ratings {
		t.Run(rating.String(), func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(rating)
			require.NoError(t, err)
			// Must encode as a quoted string, not an integer.
			assert.Equal(t, `"`+rating.String()+`"`, string(data))
			var decoded review.Rating
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, rating, decoded)
		})
	}
}

// TestRating_UnmarshalJSON_SkillOutput verifies that skill output like {"rating":"green"}
// can be decoded.
func TestRating_UnmarshalJSON_SkillOutput(t *testing.T) {
	t.Parallel()
	input := `{"rating":"green"}`
	var result struct {
		Rating review.Rating `json:"rating"`
	}
	err := json.Unmarshal([]byte(input), &result)
	require.NoError(t, err)
	assert.Equal(t, review.Green, result.Rating)
}

func TestCriterionStatus_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   review.CriterionStatus
		expected string
	}{
		{review.Satisfied, "satisfied"},
		{review.PartiallySatisfied, "partially_satisfied"},
		{review.NotSatisfied, "not_satisfied"},
		{review.Indeterminate, "indeterminate"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestParseCriterionStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected review.CriterionStatus
		wantErr  bool
	}{
		{"satisfied", review.Satisfied, false},
		{"partially_satisfied", review.PartiallySatisfied, false},
		{"not_satisfied", review.NotSatisfied, false},
		{"indeterminate", review.Indeterminate, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			status, err := review.ParseCriterionStatus(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, status)
			}
		})
	}
}

func TestRating_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rating   review.Rating
		expected string
	}{
		{review.Green, "green"},
		{review.Yellow, "yellow"},
		{review.Red, "red"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.rating.String())
		})
	}
}

func TestParseRating(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected review.Rating
		wantErr  bool
	}{
		{"green", review.Green, false},
		{"yellow", review.Yellow, false},
		{"red", review.Red, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			rating, err := review.ParseRating(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, rating)
			}
		})
	}
}

func TestCriterionResult_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		result  review.CriterionResult
		wantErr bool
	}{
		{
			name: "satisfied without citations or missing_evidence",
			result: review.CriterionResult{
				ID:        "acceptance[1]",
				Status:    review.Satisfied,
				Rationale: "make check is green per the outcome text",
			},
			wantErr: true,
		},
		{
			name: "valid satisfied with citations",
			result: review.CriterionResult{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "all requirements met",
				Citations: []review.Citation{
					{Path: "internal/review/types.go", Line: 10},
				},
			},
			wantErr: false,
		},
		{
			name: "satisfied with missing_evidence and no citations",
			result: review.CriterionResult{
				ID:              "acceptance[1]",
				Status:          review.Satisfied,
				Rationale:       "gate claim with no remaining citable evidence yet",
				MissingEvidence: "dropped activity citation; no remaining evidence",
			},
			wantErr: false,
		},
		{
			name: "valid with citations",
			result: review.CriterionResult{
				ID:     "acceptance[0]",
				Status: review.PartiallySatisfied,
				Citations: []review.Citation{
					{Path: "internal/review/types.go", Line: 10},
				},
				Rationale: "some requirements met",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			result: review.CriterionResult{
				Status:    review.Satisfied,
				Rationale: "test",
			},
			wantErr: true,
		},
		{
			name: "missing rationale",
			result: review.CriterionResult{
				ID:     "definition_of_done",
				Status: review.Satisfied,
			},
			wantErr: true,
		},
		{
			name: "missing evidence text when no citations",
			result: review.CriterionResult{
				ID:        "definition_of_done",
				Status:    review.NotSatisfied,
				Rationale: "not satisfied",
			},
			wantErr: true,
		},
		{
			name: "valid not satisfied with citations",
			result: review.CriterionResult{
				ID:     "definition_of_done",
				Status: review.NotSatisfied,
				Citations: []review.Citation{
					{Path: "file.go", Line: 1},
				},
				Rationale: "not satisfied",
			},
			wantErr: false,
		},
		{
			name: "valid not satisfied with missing evidence text",
			result: review.CriterionResult{
				ID:              "definition_of_done",
				Status:          review.NotSatisfied,
				Rationale:       "not satisfied",
				MissingEvidence: "feature X not implemented",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.result.Valid()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReviewBundle_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		bundle  review.ReviewBundle
		wantErr bool
	}{
		{
			name: "valid bundle",
			bundle: review.ReviewBundle{
				SchemaVersion: 1,
				BundleID:      "sha256:abc123",
				Issue: review.IssueInfo{
					ID:      "TASK-1",
					Type:    "task",
					Title:   "Test Task",
					Outcome: "completed successfully",
				},
				Contract: review.Contract{
					DefinitionOfDone: "all tests pass",
					Acceptance:       []string{"feature works", "documented"},
				},
				Delivery: review.Delivery{
					BaseSHA:      "abc123",
					HeadSHA:      "def456",
					ChangedFiles: []string{"file.go"},
				},
				Fingerprints: review.Fingerprints{
					Contract: "sha256:contract123",
					Delivery: "sha256:delivery123",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid schema version",
			bundle: review.ReviewBundle{
				SchemaVersion: 999,
			},
			wantErr: true,
		},
		{
			name: "missing issue ID",
			bundle: review.ReviewBundle{
				SchemaVersion: 1,
				BundleID:      "sha256:abc123",
				Issue: review.IssueInfo{
					Type: "task",
				},
			},
			wantErr: true,
		},
		{
			name: "missing issue title",
			bundle: review.ReviewBundle{
				SchemaVersion: 1,
				BundleID:      "sha256:abc123",
				Issue: review.IssueInfo{
					ID:    "TASK-1",
					Type:  "task",
					Title: "",
				},
				Fingerprints: review.Fingerprints{
					Contract: "sha256:contract123",
					Delivery: "sha256:delivery123",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.bundle.Valid()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCriterionResult_MissingStatus(t *testing.T) {
	t.Parallel()
	// Test that JSON with missing "status" key is rejected.
	input := `{"id":"definition_of_done","rationale":"test"}`
	var result review.CriterionResult
	err := json.Unmarshal([]byte(input), &result)
	assert.Error(t, err, "expected error when status key is missing")
	assert.Contains(t, err.Error(), "missing required field")
}

func TestDecodeReviewBundle_RejectsSchemaGaps_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()

	valid := mustMarshalBundleJSON(t, validDecodeBundle())

	t.Run("issue type outside enum", func(t *testing.T) {
		t.Parallel()
		data := mutateRawJSON(t, valid, func(obj map[string]any) {
			issue, ok := obj["issue"].(map[string]any)
			require.True(t, ok)
			issue["type"] = "nonsense"
		})
		_, err := review.DecodeReviewBundle(data)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "type")
	})

	t.Run("malformed delivery SHA", func(t *testing.T) {
		t.Parallel()
		data := mutateRawJSON(t, valid, func(obj map[string]any) {
			delivery, ok := obj["delivery"].(map[string]any)
			require.True(t, ok)
			delivery["head_sha"] = "not-a-git-sha"
		})
		_, err := review.DecodeReviewBundle(data)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "sha")
	})

	t.Run("omitted changed_files", func(t *testing.T) {
		t.Parallel()
		data := mutateRawJSON(t, valid, func(obj map[string]any) {
			delivery, ok := obj["delivery"].(map[string]any)
			require.True(t, ok)
			delete(delivery, "changed_files")
		})
		_, err := review.DecodeReviewBundle(data)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "changed_files")
	})

	t.Run("empty issue title", func(t *testing.T) {
		t.Parallel()
		data := mutateRawJSON(t, valid, func(obj map[string]any) {
			issue, ok := obj["issue"].(map[string]any)
			require.True(t, ok)
			issue["title"] = ""
		})
		_, err := review.DecodeReviewBundle(data)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "title")
	})

	t.Run("omitted issue title", func(t *testing.T) {
		t.Parallel()
		data := mutateRawJSON(t, valid, func(obj map[string]any) {
			issue, ok := obj["issue"].(map[string]any)
			require.True(t, ok)
			delete(issue, "title")
		})
		_, err := review.DecodeReviewBundle(data)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "title")
	})
}

func TestDecodeConformanceAssessment_RejectsNullCitations_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()

	valid := `{
  "schema_version": 1,
  "bundle_id": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "contract_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "delivery_fingerprint": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "done",
      "citations": [{"path": "impl.go", "line": 1}]
    }
  ]
}`
	_, err := review.DecodeConformanceAssessment([]byte(valid))
	require.NoError(t, err)

	nullCitations := strings.Replace(valid, `"citations": [{"path": "impl.go", "line": 1}]`, `"citations": null`, 1)
	_, err = review.DecodeConformanceAssessment([]byte(nullCitations))
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "citations")
}

func TestDecodeConformanceAssessment_RejectsInvalidCitationColumn_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()

	valid := `{
  "schema_version": 1,
  "bundle_id": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "contract_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "delivery_fingerprint": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "done",
      "citations": [{"path": "impl.go", "line": 1%s}]
    }
  ]
}`

	_, err := review.DecodeConformanceAssessment([]byte(fmt.Sprintf(valid, "")))
	require.NoError(t, err, "omitted column must remain valid")

	_, err = review.DecodeConformanceAssessment([]byte(fmt.Sprintf(valid, `, "column": 1`)))
	require.NoError(t, err, "column >= 1 must remain valid")

	_, err = review.DecodeConformanceAssessment([]byte(fmt.Sprintf(valid, `, "column": 0`)))
	require.Error(t, err, "explicit column 0 must be rejected")
	assert.Contains(t, strings.ToLower(err.Error()), "column")

	_, err = review.DecodeConformanceAssessment([]byte(fmt.Sprintf(valid, `, "column": -1`)))
	require.Error(t, err, "negative column must be rejected")
	assert.Contains(t, strings.ToLower(err.Error()), "column")
}

func TestDecodeConformanceAssessment_RejectsNullCitationLine_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()

	valid := `{
  "schema_version": 1,
  "bundle_id": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "contract_fingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "delivery_fingerprint": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "done",
      "citations": [{"path": "impl.go"%s}]
    }
  ]
}`

	_, err := review.DecodeConformanceAssessment([]byte(fmt.Sprintf(valid, `, "line": 1`)))
	require.NoError(t, err, "integer line must remain valid")

	_, err = review.DecodeConformanceAssessment([]byte(fmt.Sprintf(valid, ``)))
	require.NoError(t, err, "omitted line must remain valid")

	_, err = review.DecodeConformanceAssessment([]byte(fmt.Sprintf(valid, `, "line": null`)))
	require.Error(t, err, "explicit line null must be rejected")
	assert.Contains(t, strings.ToLower(err.Error()), "line")
	assert.NotContains(t, strings.ToLower(err.Error()), "column")
}

func validDecodeBundle() review.ReviewBundle {
	return review.ReviewBundle{
		SchemaVersion: review.SchemaVersion,
		BundleID:      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Issue: review.IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Decode contract",
			Outcome: "done",
		},
		Contract: review.Contract{
			DefinitionOfDone: "tests pass",
			Acceptance:       []string{"works"},
		},
		Delivery: review.Delivery{
			BaseSHA:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			HeadSHA:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			ChangedFiles: []string{"impl.go"},
		},
		Fingerprints: review.Fingerprints{
			Contract: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Delivery: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		},
	}
}

func mustMarshalBundleJSON(t *testing.T, bundle review.ReviewBundle) []byte {
	t.Helper()
	data, err := json.Marshal(bundle)
	require.NoError(t, err)
	return data
}

func mutateRawJSON(t *testing.T, src []byte, mut func(map[string]any)) []byte {
	t.Helper()
	var obj map[string]any
	require.NoError(t, json.Unmarshal(src, &obj))
	mut(obj)
	out, err := json.Marshal(obj)
	require.NoError(t, err)
	return out
}

func TestConformanceAssessment_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		assess  review.ConformanceAssessment
		wantErr bool
	}{
		{
			name: "valid assessment",
			assess: review.ConformanceAssessment{
				SchemaVersion: 1,
				BundleID:      "sha256:abc123",
				Results: []review.CriterionResult{
					{
						ID:        "definition_of_done",
						Status:    review.Satisfied,
						Rationale: "implemented correctly",
						Citations: []review.Citation{
							{Path: "internal/review/types.go", Line: 10},
						},
					},
					{
						ID:        "acceptance[0]",
						Status:    review.Satisfied,
						Rationale: "working as designed",
						Citations: []review.Citation{
							{Path: "internal/review/types.go", Line: 20},
						},
					},
				},
				ContractFingerprint: "sha256:contract123",
				DeliveryFingerprint: "sha256:delivery123",
			},
			wantErr: false,
		},
		{
			name: "invalid schema version",
			assess: review.ConformanceAssessment{
				SchemaVersion: 999,
			},
			wantErr: true,
		},
		{
			name: "missing results",
			assess: review.ConformanceAssessment{
				SchemaVersion:       1,
				BundleID:            "sha256:abc123",
				Results:             []review.CriterionResult{},
				ContractFingerprint: "sha256:contract123",
				DeliveryFingerprint: "sha256:delivery123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.assess.Valid()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestActivity_JSONRoundTrip_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	activity := review.Activity{
		Digest:            "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		EntryCount:        5,
		DeliveryHeadCount: 3,
		EarlierCount:      2,
		LogPath:           ".git/armature-activity.log",
	}

	// Marshal to JSON
	data, err := json.Marshal(activity)
	require.NoError(t, err)

	// Unmarshal back
	var decoded review.Activity
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Verify all fields are preserved
	assert.Equal(t, activity.Digest, decoded.Digest)
	assert.Equal(t, activity.EntryCount, decoded.EntryCount)
	assert.Equal(t, activity.DeliveryHeadCount, decoded.DeliveryHeadCount)
	assert.Equal(t, activity.EarlierCount, decoded.EarlierCount)
	assert.Equal(t, activity.LogPath, decoded.LogPath)
}

func TestReviewBundle_WithActivity_JSONRoundTrip_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	bundle := review.ReviewBundle{
		SchemaVersion: review.SchemaVersion,
		BundleID:      "sha256:bundle123",
		Issue: review.IssueInfo{
			ID:      "EXECEV-T2",
			Type:    "task",
			Title:   "Test task",
			Outcome: "done",
		},
		Contract: review.Contract{
			DefinitionOfDone: "All tests pass",
			Acceptance:       []string{"Feature works"},
		},
		Delivery: review.Delivery{
			BaseSHA:      "abc123",
			HeadSHA:      "def456",
			ChangedFiles: []string{"file.go"},
		},
		Fingerprints: review.Fingerprints{
			Contract: "fp_contract",
			Delivery: "fp_delivery",
		},
		Activity: &review.Activity{
			Digest:            "fp_activity",
			EntryCount:        3,
			DeliveryHeadCount: 2,
			EarlierCount:      1,
			LogPath:           ".git/armature-activity.log",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(bundle)
	require.NoError(t, err)

	// Unmarshal back
	var decoded review.ReviewBundle
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Verify bundle
	assert.Equal(t, bundle.SchemaVersion, decoded.SchemaVersion)
	assert.Equal(t, bundle.BundleID, decoded.BundleID)

	// Verify Activity is preserved
	require.NotNil(t, decoded.Activity)
	assert.Equal(t, bundle.Activity.Digest, decoded.Activity.Digest)
	assert.Equal(t, bundle.Activity.EntryCount, decoded.Activity.EntryCount)
	assert.Equal(t, bundle.Activity.DeliveryHeadCount, decoded.Activity.DeliveryHeadCount)
	assert.Equal(t, bundle.Activity.EarlierCount, decoded.Activity.EarlierCount)
	assert.Equal(t, bundle.Activity.LogPath, decoded.Activity.LogPath)
}

func TestReviewBundle_WithoutActivity_JSONRoundTrip_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	bundle := review.ReviewBundle{
		SchemaVersion: review.SchemaVersion,
		BundleID:      "sha256:bundle123",
		Issue: review.IssueInfo{
			ID:      "EXECEV-T2",
			Type:    "task",
			Title:   "Test task",
			Outcome: "done",
		},
		Contract: review.Contract{
			DefinitionOfDone: "All tests pass",
		},
		Delivery: review.Delivery{
			BaseSHA: "abc123",
			HeadSHA: "def456",
		},
		Fingerprints: review.Fingerprints{
			Contract: "fp_contract",
			Delivery: "fp_delivery",
		},
		// Activity is nil
		Activity: nil,
	}

	// Marshal to JSON
	data, err := json.Marshal(bundle)
	require.NoError(t, err)

	// Verify Activity field is omitted from JSON (due to omitempty)
	assert.NotContains(t, string(data), "activity")

	// Unmarshal back
	var decoded review.ReviewBundle
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Verify Activity is still nil
	assert.Nil(t, decoded.Activity)
}

func TestAssessmentAttestation_WithActivityDigest_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	attestation := review.AssessmentAttestation{
		SchemaVersion:           review.SchemaVersion,
		BundleID:                "sha256:bundle123",
		ContractFingerprint:     "fp_contract",
		DeliveryFingerprint:     "fp_delivery",
		ActivityDigest:          "fp_activity",
		BaseSHA:                 "abc123",
		HeadSHA:                 "def456",
		Rating:                  review.Green,
		ResultFingerprint:       "fp_result",
		SatisfiedCount:          1,
		PartiallySatisfiedCount: 0,
		NotSatisfiedCount:       0,
		IndeterminateCount:      0,
	}

	// Marshal to JSON
	data, err := json.Marshal(attestation)
	require.NoError(t, err)

	// Unmarshal back
	var decoded review.AssessmentAttestation
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Verify ActivityDigest is preserved
	assert.Equal(t, attestation.ActivityDigest, decoded.ActivityDigest)
}

func TestAssessmentAttestation_WithoutActivityDigest_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	attestation := review.AssessmentAttestation{
		SchemaVersion:           review.SchemaVersion,
		BundleID:                "sha256:bundle123",
		ContractFingerprint:     "fp_contract",
		DeliveryFingerprint:     "fp_delivery",
		ActivityDigest:          "", // empty
		BaseSHA:                 "abc123",
		HeadSHA:                 "def456",
		Rating:                  review.Green,
		ResultFingerprint:       "fp_result",
		SatisfiedCount:          1,
		PartiallySatisfiedCount: 0,
		NotSatisfiedCount:       0,
		IndeterminateCount:      0,
	}

	// Marshal to JSON
	data, err := json.Marshal(attestation)
	require.NoError(t, err)

	// Verify ActivityDigest field is omitted from JSON (due to omitempty)
	assert.NotContains(t, string(data), "activity_digest")

	// Unmarshal back
	var decoded review.AssessmentAttestation
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Verify ActivityDigest is empty
	assert.Empty(t, decoded.ActivityDigest)
}
