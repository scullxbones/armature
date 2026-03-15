package validate

import "github.com/scullxbones/trellis/internal/materialize"

// Result holds the outcome of a validation run.
type Result struct {
	OK       bool
	Errors   []string
	Warnings []string
}

// Validate checks the materialized state for consistency issues.
// Returns a Result with any errors or warnings found.
func Validate(state *materialize.State) Result {
	var errors []string
	var warnings []string

	for id, issue := range state.Issues {
		// Check orphaned children: parent referenced but not in state
		if issue.Parent != "" {
			if _, ok := state.Issues[issue.Parent]; !ok {
				errors = append(errors, "issue "+id+" references unknown parent "+issue.Parent)
			}
		}

		// Check unknown blockers
		for _, blockerID := range issue.BlockedBy {
			if _, ok := state.Issues[blockerID]; !ok {
				errors = append(errors, "issue "+id+" references unknown blocker "+blockerID)
			}
		}
	}

	// Check circular deps via DFS
	for id := range state.Issues {
		if hasCycle(id, state) {
			errors = append(errors, "issue "+id+" has circular dependency")
		}
	}

	// Stale claims: structured for future use
	_ = warnings

	ok := len(errors) == 0
	return Result{
		OK:       ok,
		Errors:   errors,
		Warnings: warnings,
	}
}

// hasCycle detects if the given issue ID is part of a cycle in BlockedBy graph.
func hasCycle(startID string, state *materialize.State) bool {
	visited := make(map[string]bool)
	return dfs(startID, startID, visited, state, true)
}

func dfs(startID, currentID string, visited map[string]bool, state *materialize.State, first bool) bool {
	if !first && currentID == startID {
		return true
	}
	if visited[currentID] {
		return false
	}
	visited[currentID] = true

	issue, ok := state.Issues[currentID]
	if !ok {
		return false
	}

	for _, blockerID := range issue.BlockedBy {
		if blockerID == startID {
			return true
		}
		if dfs(startID, blockerID, visited, state, false) {
			return true
		}
	}
	return false
}
