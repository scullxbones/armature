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

// ratingSeverity ranks Conformance Ratings for disagreement enrichment.
// Order: Green < Yellow < Red. Unknown values sort below Green.
func ratingSeverity(r Rating) int {
	switch r {
	case Green:
		return 1
	case Yellow:
		return 2
	case Red:
		return 3
	default:
		return 0
	}
}

// MaxRating returns the highest-severity Conformance Rating (Green < Yellow < Red).
// With no arguments it returns Green. EffectiveRating built from this helper is
// advisory only and does not confer merge authority (Constitution I5/N4).
func MaxRating(ratings ...Rating) Rating {
	if len(ratings) == 0 {
		return Green
	}
	max := ratings[0]
	for _, r := range ratings[1:] {
		if ratingSeverity(r) > ratingSeverity(max) {
			max = r
		}
	}
	return max
}
