package review_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
)

func TestDeriveRating_AllSatisfied_Green(t *testing.T) {
	t.Parallel()
	results := []review.CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    review.Satisfied,
			Rationale: "completed",
		},
		{
			ID:        "acceptance[0]",
			Status:    review.Satisfied,
			Rationale: "verified",
		},
		{
			ID:        "acceptance[1]",
			Status:    review.Satisfied,
			Rationale: "validated",
		},
	}

	rating := review.DeriveRating(results)
	assert.Equal(t, review.Green, rating)
}

func TestDeriveRating_PartiallySatisfied_Yellow(t *testing.T) {
	t.Parallel()
	results := []review.CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    review.Satisfied,
			Rationale: "mostly done",
		},
		{
			ID:              "acceptance[0]",
			Status:          review.PartiallySatisfied,
			Rationale:       "partial",
			MissingEvidence: "feature X incomplete",
		},
	}

	rating := review.DeriveRating(results)
	assert.Equal(t, review.Yellow, rating)
}

func TestDeriveRating_Indeterminate_Yellow(t *testing.T) {
	t.Parallel()
	results := []review.CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    review.Satisfied,
			Rationale: "verified",
		},
		{
			ID:              "acceptance[0]",
			Status:          review.Indeterminate,
			Rationale:       "ambiguous",
			MissingEvidence: "unclear requirement",
		},
	}

	rating := review.DeriveRating(results)
	assert.Equal(t, review.Yellow, rating)
}

func TestDeriveRating_NotSatisfied_Red(t *testing.T) {
	t.Parallel()
	results := []review.CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    review.Satisfied,
			Rationale: "done",
		},
		{
			ID:              "acceptance[0]",
			Status:          review.NotSatisfied,
			Rationale:       "missing",
			MissingEvidence: "not implemented",
		},
	}

	rating := review.DeriveRating(results)
	assert.Equal(t, review.Red, rating)
}

func TestDeriveRating_Mixed_YellowAndRed_Red(t *testing.T) {
	t.Parallel()
	// Red takes precedence: any not_satisfied → red
	results := []review.CriterionResult{
		{
			ID:              "acceptance[0]",
			Status:          review.PartiallySatisfied,
			Rationale:       "partial",
			MissingEvidence: "feature X incomplete",
		},
		{
			ID:              "acceptance[1]",
			Status:          review.NotSatisfied,
			Rationale:       "missing",
			MissingEvidence: "not implemented",
		},
	}

	rating := review.DeriveRating(results)
	assert.Equal(t, review.Red, rating)
}

func TestDeriveRating_Empty_Green(t *testing.T) {
	t.Parallel()
	// Empty result set is treated as green (no violations)
	results := []review.CriterionResult{}

	rating := review.DeriveRating(results)
	assert.Equal(t, review.Green, rating)
}

func TestDeriveRating_SingleIndeterminate_Yellow(t *testing.T) {
	t.Parallel()
	results := []review.CriterionResult{
		{
			ID:              "definition_of_done",
			Status:          review.Indeterminate,
			Rationale:       "unclear",
			MissingEvidence: "ambiguous requirement",
		},
	}

	rating := review.DeriveRating(results)
	assert.Equal(t, review.Yellow, rating)
}

func TestCountCriteria(t *testing.T) {
	t.Parallel()
	results := []review.CriterionResult{
		{ID: "def", Status: review.Satisfied, Rationale: "ok"},
		{ID: "acc[0]", Status: review.Satisfied, Rationale: "ok"},
		{ID: "acc[1]", Status: review.PartiallySatisfied, Rationale: "partial", MissingEvidence: "X"},
		{ID: "acc[2]", Status: review.NotSatisfied, Rationale: "no", MissingEvidence: "Y"},
		{ID: "acc[3]", Status: review.Indeterminate, Rationale: "unclear", MissingEvidence: "Z"},
	}

	sat, partial, notSat, indet := review.CountCriteria(results)

	assert.Equal(t, 2, sat)
	assert.Equal(t, 1, partial)
	assert.Equal(t, 1, notSat)
	assert.Equal(t, 1, indet)
}
