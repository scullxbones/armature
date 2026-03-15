package decompose

import "fmt"

// PlanContext returns a summary of the plan suitable for use as context.
// This is a stub that returns the plan title and issue count.
func PlanContext(plan *Plan) string {
	return fmt.Sprintf("Plan: %s (%d issues)", plan.Title, len(plan.Issues))
}
