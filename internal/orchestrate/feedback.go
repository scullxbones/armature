package orchestrate

// Feedback holds structured feedback that is injected into a retry prompt.
// It is constructed by AssembleFeedback and consumed by AssemblePrompt.
type Feedback struct {
	// ScopeConstraint is a negative constraint telling the agent which files
	// it must not touch (non-empty starting from retry 2).
	ScopeConstraint string

	// NamedFiles is the explicit list of files the agent should touch
	// (non-empty starting from retry 3).
	NamedFiles []string

	// FailedChecks is the subset of check results that did not pass.
	FailedChecks []CheckResult
}

// AssembleFeedback builds a Feedback value appropriate for the given retry
// number.
//
//   - retryNum 0–1: no extra constraints; only failed checks are surfaced.
//   - retryNum 2:   adds a negative scope constraint ("only modify these
//     files"); no explicit file list yet.
//   - retryNum 3+:  adds both the scope constraint and an explicit named-file
//     list.
//
// scopeFiles is the full list of file paths the agent is permitted to touch.
// checks is the full list of CheckResult values from the most recent run.
func AssembleFeedback(retryNum int, scopeFiles []string, checks []CheckResult) Feedback {
	var fb Feedback

	// Surface only the failed checks regardless of retry number.
	for _, c := range checks {
		if !c.Passed {
			fb.FailedChecks = append(fb.FailedChecks, c)
		}
	}

	if retryNum >= 2 {
		fb.ScopeConstraint = "Do not modify files outside the permitted scope. Only touch the files you have been given permission to change."
	}

	if retryNum >= 3 {
		// Copy the slice to avoid aliasing.
		named := make([]string, len(scopeFiles))
		copy(named, scopeFiles)
		fb.NamedFiles = named
	}

	return fb
}
