package review

import "github.com/scullxbones/armature/internal/attestation"

func DeriveRating(results []CriterionResult) attestation.Rating {
	if len(results) == 0 {
		return attestation.Green
	}

	hasNotSatisfied := false
	hasPartialOrIndeterminate := false

	for _, result := range results {
		switch result.Status {
		case NotSatisfied:
			hasNotSatisfied = true
		case PartiallySatisfied, Indeterminate:
			hasPartialOrIndeterminate = true
		}
	}

	if hasNotSatisfied {
		return attestation.Red
	}
	if hasPartialOrIndeterminate {
		return attestation.Yellow
	}
	return attestation.Green
}

func CountCriteria(results []CriterionResult) (int, int, int, int) {
	var satisfied, partiallySatisfied, notSatisfied, indeterminate int

	for _, result := range results {
		switch result.Status {
		case Satisfied:
			satisfied++
		case PartiallySatisfied:
			partiallySatisfied++
		case NotSatisfied:
			notSatisfied++
		case Indeterminate:
			indeterminate++
		}
	}

	return satisfied, partiallySatisfied, notSatisfied, indeterminate
}
