package promotion

import (
	"fmt"
	"strings"
)

// PlanOpts carries viewer identity and the CLI decision predicate so every
// printed argv is checked with the same code claim/transition use.
type PlanOpts struct {
	Viewer string // current worker ID; empty treats every non-empty holder as foreign when Decide is nil
	Now    int64  // unix seconds for TTL-aware Decide / guidance expiry

	// Decide is the CLI decision predicate for one suggested arm argv against
	// graph g (same refusals as claim/transition). Required in production
	// print paths; when nil, only WouldViolateTreeConsistency gates claims.
	Decide func(argv string, g Graph) error

	// Producer, when non-nil, replaces buildRemediationsPlan for this call only
	// (parallel-safe; no package-global test hook).
	Producer func(id string, g Graph, opts PlanOpts) Plan
}

// IssueFlag returns the --issue value from an arm argv, or empty.
func IssueFlag(argv string) string {
	fields := strings.Fields(argv)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "--issue" {
			return fields[i+1]
		}
	}
	return ""
}

// DefaultDecideOnlyTree is a minimal Decide for tests that only need the
// shared tree-consistency gate (no lease clocks). Production uses cmd's
// remediationDecide wired to claim.ForeignLiveLeaseBlocks + RequireOwner.
func DefaultDecideOnlyTree(viewer string) func(string, Graph) error {
	return func(argv string, g Graph) error {
		argv = strings.TrimSpace(argv)
		if !strings.HasPrefix(argv, "arm ") {
			return nil
		}
		issue := IssueFlag(argv)
		if issue == "" {
			return nil
		}
		if strings.Contains(argv, "arm claim ") && WouldViolateTreeConsistency(issue, g) {
			return fmt.Errorf("cannot claim %s: under a merged ancestor (tree consistency)", issue)
		}
		return nil
	}
}
