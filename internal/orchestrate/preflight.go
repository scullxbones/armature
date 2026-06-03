package orchestrate

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scullxbones/armature/internal/sources"
)

// PreflightRequest carries all data required to run pre-dispatch validation.
type PreflightRequest struct {
	// ScopePaths is the list of file or directory paths that the task is
	// permitted to modify. Each must exist on disk.
	ScopePaths []string

	// Acceptance is the raw JSON acceptance-criteria array ([]string).
	// At least one item must be machine-verifiable.
	Acceptance json.RawMessage

	// CitationIDs is the set of source entry IDs that the issue cites.
	// Each ID must resolve to a known entry in Manifest.
	CitationIDs []string

	// Manifest is the source manifest used to resolve citation IDs.
	Manifest sources.Manifest

	// TokenBudget is the configured token limit for the run.
	// A value ≤ 0 is an error.
	TokenBudget int

	// WorkDir is the working directory for the run (informational; not
	// validated by preflight, but may be used by callers for context).
	WorkDir string

	// SandboxRequired indicates that a sandbox binary must be present
	// on the host. When false, SandboxOK is ignored.
	SandboxRequired bool

	// SandboxOK is the result of a prior SandboxAvailable() call.
	// Only meaningful when SandboxRequired is true.
	SandboxOK bool

	// AuthRequired indicates harness auth must be validated before dispatch.
	AuthRequired bool
	// AuthOK is the result of auth resolution/status checks.
	AuthOK bool
	// AuthError is a human-readable auth failure detail.
	AuthError string
}

// PreflightInput is a compatibility alias for the older request name.
type PreflightInput = PreflightRequest

// PreflightResult holds the outcome of RunPreflight.
type PreflightResult struct {
	// OK is true when all checks passed.
	OK bool

	// Errors is the ordered list of descriptive error strings, one per
	// failed check. Empty when OK is true.
	Errors []string
}

// Error returns all preflight errors joined by "; ", or "" when OK.
func (r PreflightResult) Error() string {
	return strings.Join(r.Errors, "; ")
}

// RunPreflight validates all pre-run conditions and returns a PreflightResult.
// Every failed check appends a distinct, descriptive entry to PreflightResult.Errors
// so callers can surface the full set of problems in one pass rather than requiring
// multiple retries to discover them one at a time.
//
// Checks performed (in order):
//  1. Scope paths: at least one must be declared; each must exist on disk.
//  2. Acceptance criteria: parseable JSON array with at least one verifiable item.
//  3. Source citations: each CitationID must resolve in Manifest.
//  4. Token budget: must be > 0.
//  5. Sandbox: when SandboxRequired, SandboxOK must be true.
func RunPreflight(in PreflightRequest) PreflightResult {
	var errs []string

	// --- 1. Scope paths ---
	if len(in.ScopePaths) == 0 {
		errs = append(errs, "scope paths are empty — at least one path must be declared")
	} else {
		for _, p := range in.ScopePaths {
			if _, err := os.Stat(p); err != nil {
				errs = append(errs, fmt.Sprintf("scope path does not exist: %s", p))
			}
		}
	}

	// --- 2. Acceptance criteria ---
	acResult := CheckAcceptanceCriteria(in.Acceptance)
	if !acResult.Passed {
		errs = append(errs, fmt.Sprintf("acceptance criteria: %s", acResult.Message))
	}

	// --- 3. Source citations ---
	for _, id := range in.CitationIDs {
		if _, ok := in.Manifest.Get(id); !ok {
			errs = append(errs, fmt.Sprintf("source citation %q does not resolve in manifest", id))
		}
	}

	// --- 4. Token budget ---
	if in.TokenBudget <= 0 {
		errs = append(errs, fmt.Sprintf("token budget must be > 0 (got %d)", in.TokenBudget))
	}

	// --- 5. Sandbox ---
	if in.SandboxRequired && !in.SandboxOK {
		errs = append(errs, "sandbox is required but not available on this host")
	}

	// --- 6. Harness auth ---
	if in.AuthRequired && !in.AuthOK {
		msg := strings.TrimSpace(in.AuthError)
		if msg == "" {
			msg = "harness auth is unavailable"
		}
		errs = append(errs, "auth: "+msg)
	}

	return PreflightResult{
		OK:     len(errs) == 0,
		Errors: errs,
	}
}
