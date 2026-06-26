package harnesspolicy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerificationServiceCheckAcceptanceCriteriaRejectsAllUnverifiable(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()
	acceptance := json.RawMessage(`[
		"The UI looks good",
		"A human reviewer approves the change"
	]`)

	result := service.CheckAcceptanceCriteria(acceptance)

	assert.False(t, result.Passed)
	assert.Equal(t, CheckAcceptanceCriteria, result.Name)
	assert.Contains(t, result.Message, "unverifiable")
}

func TestVerificationServiceCheckAcceptanceCriteriaAcceptsMachineVerifiableItem(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()
	acceptance := json.RawMessage(`["The UI looks good", "go test ./... passes"]`)

	result := service.CheckAcceptanceCriteria(acceptance)

	assert.True(t, result.Passed)
	assert.Equal(t, CheckAcceptanceCriteria, result.Name)
}

func TestVerificationServiceCheckAcceptanceCriteriaRejectsMissingArray(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()

	result := service.CheckAcceptanceCriteria(nil)

	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "empty or absent")
}

func TestVerificationServiceCheckCitationsRejectsUncitedSource(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()
	checks := []CitationCheck{
		{SourceEntryID: "src-1", Accepted: true},
		{SourceEntryID: "src-2", Accepted: false},
	}

	result := service.CheckCitations(checks)

	assert.False(t, result.Passed)
	assert.Equal(t, CheckCitations, result.Name)
	assert.Contains(t, result.Message, "src-2")
}

func TestVerificationServiceCheckCitationsAcceptsEmptyChecks(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()

	result := service.CheckCitations(nil)

	assert.True(t, result.Passed)
	assert.Equal(t, CheckCitations, result.Name)
}

func TestVerificationServiceCheckAcceptanceCriteria_NullJSON(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()

	result := service.CheckAcceptanceCriteria(json.RawMessage("null"))

	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "empty or absent")
}

func TestVerificationServiceCheckAcceptanceCriteria_EmptyArray(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()

	result := service.CheckAcceptanceCriteria(json.RawMessage(`[]`))

	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "empty or absent")
}

func TestVerificationServiceCheckAcceptanceCriteria_MalformedJSON(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()

	result := service.CheckAcceptanceCriteria(json.RawMessage(`"not-an-array"`))

	assert.False(t, result.Passed)
	assert.Contains(t, result.Message, "not parseable")
}

func TestVerificationServiceRunReturnsAcceptanceAndCitationResults(t *testing.T) {
	t.Parallel()
	service := NewVerificationService()
	request := VerificationRequest{
		Acceptance: json.RawMessage(`["go test ./... passes"]`),
		Citations: []CitationCheck{
			{SourceEntryID: "src-1", Accepted: true},
		},
	}

	results := service.Run(request)

	assert.Len(t, results, 2)
	assert.Equal(t, CheckAcceptanceCriteria, results[0].Name)
	assert.Equal(t, CheckCitations, results[1].Name)
	assert.True(t, results[0].Passed)
	assert.True(t, results[1].Passed)
}
