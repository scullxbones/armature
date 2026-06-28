package review

// DeriveRating produces a conformance rating from criterion results.
// The rating algebra follows the prioritization: Red > Yellow > Green.
//
//   - Green: all criteria are satisfied
//   - Yellow: at least one criterion is partially_satisfied or indeterminate,
//     and no criteria are not_satisfied
//   - Red: at least one criterion is not_satisfied
func DeriveRating(results []CriterionResult) Rating {
	if len(results) == 0 {
		return Green
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
		return Red
	}
	if hasPartialOrIndeterminate {
		return Yellow
	}
	return Green
}

// CountCriteria counts criterion results by their status.
// Returns: satisfied, partially_satisfied, not_satisfied, indeterminate counts.
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
