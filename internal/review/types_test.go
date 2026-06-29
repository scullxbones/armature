package review_test

import (
	"encoding/json"
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
			name: "valid satisfied",
			result: review.CriterionResult{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "all requirements met",
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
					},
					{
						ID:        "acceptance[0]",
						Status:    review.Satisfied,
						Rationale: "working as designed",
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
