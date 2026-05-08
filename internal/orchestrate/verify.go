package orchestrate

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// CitationCheck carries citation acceptance data for a single source entry.
// It is used by CheckCitations to verify that all sources referenced by an
// issue have been accepted by a worker.
type CitationCheck struct {
	// SourceEntryID is the unique identifier of the source entry.
	SourceEntryID string
	// Accepted is true when the source has been formally cited or accepted.
	Accepted bool
}

// NormalizeScope converts a raw scope path to a canonical form for boundary
// checks: forward slashes (filepath.ToSlash), no redundant path elements
// (filepath.Clean), and a guaranteed trailing slash so HasPrefix comparisons
// cannot match a directory that merely shares a prefix with another.
//
//	"internal\\foo"       → "internal/foo/"
//	"internal/../foo"     → "foo/"
//	"internal/orchestrate/" → "internal/orchestrate/"
func NormalizeScope(scope string) string {
	// ToSlash converts OS-specific separators to forward slashes.
	slashed := filepath.ToSlash(scope)
	// Clean resolves "..", "//", etc.
	cleaned := filepath.Clean(slashed)
	// Ensure trailing slash to prevent false-positive prefix matches.
	if !strings.HasSuffix(cleaned, "/") {
		cleaned += "/"
	}
	return cleaned
}

// RunPipeline executes the provided adapters in order, collecting CheckResults
// into the returned OrchestrateState. It stops execution on the first adapter
// that produces a CheckResult with SeverityError (hard fail). SeverityWarning
// results do not stop the pipeline.
//
// After running all adapters, RunPipeline appends built-in checks for:
//   - acceptance criteria (CheckAcceptanceCriteria)
//   - citations (CheckCitations)
//
// The context is checked before each adapter; a cancelled context causes
// RunPipeline to return an error immediately.
func RunPipeline(
	ctx context.Context,
	adapters []HarnessAdapter,
	cfg HarnessConfig,
	opts RunOptions,
	acceptance json.RawMessage,
	citations []CitationCheck,
) (OrchestrateState, error) {
	state := OrchestrateState{
		StartedAt: time.Now().Unix(),
	}

	for _, adapter := range adapters {
		// Honour context cancellation between checks.
		if err := ctx.Err(); err != nil {
			return state, fmt.Errorf("pipeline cancelled: %w", err)
		}

		result, err := adapter.Run(ctx, cfg, opts)
		if err != nil {
			return state, fmt.Errorf("adapter %s: %w", adapter.Name(), err)
		}

		state.Checks = append(state.Checks, result)

		// Hard fail — stop immediately.
		if result.Severity == SeverityError && !result.Passed {
			state.Failed = true
			state.FinishedAt = time.Now().Unix()
			return state, nil
		}
	}

	// Built-in acceptance-criteria check.
	if err := ctx.Err(); err != nil {
		return state, fmt.Errorf("pipeline cancelled: %w", err)
	}
	acResult := CheckAcceptanceCriteria(acceptance)
	state.Checks = append(state.Checks, acResult)
	if acResult.Severity == SeverityError && !acResult.Passed {
		state.Failed = true
		state.FinishedAt = time.Now().Unix()
		return state, nil
	}

	// Built-in citations check.
	if err := ctx.Err(); err != nil {
		return state, fmt.Errorf("pipeline cancelled: %w", err)
	}
	citResult := CheckCitations(citations)
	state.Checks = append(state.Checks, citResult)
	if citResult.Severity == SeverityError && !citResult.Passed {
		state.Failed = true
	}

	state.FinishedAt = time.Now().Unix()
	return state, nil
}

// verifiableKeywords are substrings that indicate a plain-text acceptance
// criterion can be machine-verified (e.g. a test name or make target).
var verifiableKeywords = []string{
	"passes",
	"green",
	"make check",
	"go test",
	"npm test",
	"pytest",
}

// isVerifiable returns true when the criterion string contains at least one
// keyword that suggests it can be checked automatically.
func isVerifiable(criterion string) bool {
	lower := strings.ToLower(criterion)
	for _, kw := range verifiableKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// CheckAcceptanceCriteria inspects the raw acceptance JSON (an array of
// strings). It returns a failing CheckResult when:
//   - the array is nil/empty, OR
//   - every item in the array is human-only / unverifiable.
//
// At least one item must be machine-verifiable (contains a keyword such as
// "passes", "green", "make check", etc.) for the check to pass.
func CheckAcceptanceCriteria(acceptance json.RawMessage) CheckResult {
	result := CheckResult{
		Name:     "acceptance-criteria",
		Severity: SeverityError,
	}

	if len(acceptance) == 0 || string(acceptance) == "null" {
		result.Passed = false
		result.Message = "acceptance array is empty or absent"
		return result
	}

	var items []string
	if err := json.Unmarshal(acceptance, &items); err != nil {
		result.Passed = false
		result.Message = fmt.Sprintf("acceptance array is not parseable: %v", err)
		return result
	}

	if len(items) == 0 {
		result.Passed = false
		result.Message = "acceptance array is empty — nothing to verify"
		return result
	}

	for _, item := range items {
		if isVerifiable(item) {
			result.Passed = true
			result.Message = "at least one machine-verifiable acceptance criterion present"
			return result
		}
	}

	// Reached the end without finding any verifiable criterion.
	result.Passed = false
	result.Message = fmt.Sprintf(
		"all %d acceptance criteria are unverifiable (human-only); "+
			"add at least one machine-checkable criterion (e.g. 'make check green')",
		len(items),
	)
	return result
}

// CheckCitations verifies that every CitationCheck in the list has been
// accepted. It correlates per SourceEntryID, reporting the first uncited entry
// in the failure message.
//
// An empty or nil list is treated as fully cited (passes).
func CheckCitations(checks []CitationCheck) CheckResult {
	result := CheckResult{
		Name:     "citations",
		Severity: SeverityError,
	}

	var uncited []string
	for _, c := range checks {
		if !c.Accepted {
			uncited = append(uncited, c.SourceEntryID)
		}
	}

	if len(uncited) == 0 {
		result.Passed = true
		result.Message = "all sources cited"
		return result
	}

	result.Passed = false
	result.Message = fmt.Sprintf(
		"uncited source(s): %s",
		strings.Join(uncited, ", "),
	)
	return result
}
