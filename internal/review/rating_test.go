package review

import (
	"testing"

	"github.com/scullxbones/armature/internal/attestation"

	"github.com/stretchr/testify/assert"
)

func TestDeriveRating_AllSatisfied_Green(t *testing.T) {
	t.Parallel()
	results := []CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    Satisfied,
			Rationale: "completed",
		},
		{
			ID:        "acceptance[0]",
			Status:    Satisfied,
			Rationale: "verified",
		},
		{
			ID:        "acceptance[1]",
			Status:    Satisfied,
			Rationale: "validated",
		},
	}

	rating := deriveRating(results)
	assert.Equal(t, attestation.Green, rating)
}

func TestDeriveRating_PartiallySatisfied_Yellow(t *testing.T) {
	t.Parallel()
	results := []CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    Satisfied,
			Rationale: "mostly done",
		},
		{
			ID:              "acceptance[0]",
			Status:          PartiallySatisfied,
			Rationale:       "partial",
			MissingEvidence: "feature X incomplete",
		},
	}

	rating := deriveRating(results)
	assert.Equal(t, attestation.Yellow, rating)
}

func TestDeriveRating_Indeterminate_Yellow(t *testing.T) {
	t.Parallel()
	results := []CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    Satisfied,
			Rationale: "verified",
		},
		{
			ID:              "acceptance[0]",
			Status:          Indeterminate,
			Rationale:       "ambiguous",
			MissingEvidence: "unclear requirement",
		},
	}

	rating := deriveRating(results)
	assert.Equal(t, attestation.Yellow, rating)
}

func TestDeriveRating_NotSatisfied_Red(t *testing.T) {
	t.Parallel()
	results := []CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    Satisfied,
			Rationale: "done",
		},
		{
			ID:              "acceptance[0]",
			Status:          NotSatisfied,
			Rationale:       "missing",
			MissingEvidence: "not implemented",
		},
	}

	rating := deriveRating(results)
	assert.Equal(t, attestation.Red, rating)
}

func TestDeriveRating_Mixed_YellowAndRed_Red(t *testing.T) {
	t.Parallel()

	results := []CriterionResult{
		{
			ID:              "acceptance[0]",
			Status:          PartiallySatisfied,
			Rationale:       "partial",
			MissingEvidence: "feature X incomplete",
		},
		{
			ID:              "acceptance[1]",
			Status:          NotSatisfied,
			Rationale:       "missing",
			MissingEvidence: "not implemented",
		},
	}

	rating := deriveRating(results)
	assert.Equal(t, attestation.Red, rating)
}

func TestDeriveRating_Empty_Green(t *testing.T) {
	t.Parallel()

	results := []CriterionResult{}

	rating := deriveRating(results)
	assert.Equal(t, attestation.Green, rating)
}

func TestDeriveRating_SingleIndeterminate_Yellow(t *testing.T) {
	t.Parallel()
	results := []CriterionResult{
		{
			ID:              "definition_of_done",
			Status:          Indeterminate,
			Rationale:       "unclear",
			MissingEvidence: "ambiguous requirement",
		},
	}

	rating := deriveRating(results)
	assert.Equal(t, attestation.Yellow, rating)
}

func TestCountCriteria(t *testing.T) {
	t.Parallel()
	results := []CriterionResult{
		{ID: "def", Status: Satisfied, Rationale: "ok"},
		{ID: "acc[0]", Status: Satisfied, Rationale: "ok"},
		{ID: "acc[1]", Status: PartiallySatisfied, Rationale: "partial", MissingEvidence: "X"},
		{ID: "acc[2]", Status: NotSatisfied, Rationale: "no", MissingEvidence: "Y"},
		{ID: "acc[3]", Status: Indeterminate, Rationale: "unclear", MissingEvidence: "Z"},
	}

	sat, partial, notSat, indet := countCriteria(results)

	assert.Equal(t, 2, sat)
	assert.Equal(t, 1, partial)
	assert.Equal(t, 1, notSat)
	assert.Equal(t, 1, indet)
}
