package promotion

import (
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
)

// PlanOpts carries viewer identity so Options match CLI ownership gates.
type PlanOpts struct {
	Viewer string // current worker ID; empty treats every holder as foreign
}

// cmdGate is the shared product predicate for a suggested arm argv.
// Same rules the CLI enforces: claim refuses under merged ancestors and
// refuses foreign live leases (--force is scope-overlap only); transition
// to cancelled on claimed/in-progress requires the published owner
// (ClaimedBy == viewer). reopen stays allowed.
//
// Used by plan construction/validation so printed steps cannot drift from CLI.
func cmdGate(argv string, g Graph, viewer string) error {
	argv = strings.TrimSpace(argv)
	if argv == "" || !strings.HasPrefix(argv, "arm ") {
		return nil
	}
	issue := issueFlag(argv)
	if issue == "" {
		return nil
	}
	node := g[issue]
	switch {
	case strings.Contains(argv, "arm claim "):
		if WouldViolateTreeConsistency(issue, g) {
			return fmt.Errorf("cannot claim %s: under a merged ancestor (tree consistency)", issue)
		}
		if foreignLiveLeaseBlocks(node, viewer) {
			return fmt.Errorf("cannot claim %s: foreign live lease held by %s (claim --force only overrides scope overlap)", issue, node.ClaimedBy)
		}
	case strings.Contains(argv, "--to cancelled"):
		if foreignLiveLeaseBlocks(node, viewer) {
			return fmt.Errorf("NOT-CLAIM-OWNER")
		}
	case strings.Contains(argv, "arm unassign "),
		strings.Contains(argv, "arm reopen "):
		// Privileged unassign / reopen — not owner-gated.
	}
	return nil
}

// foreignLiveLeaseBlocks mirrors claim.foreignLiveLeaseBlocks for graph facts:
// claimed/in-progress with a holder other than viewer is treated as a live
// foreign lease (Node has no TTL clocks; any holder is assumed live).
func foreignLiveLeaseBlocks(node Node, viewer string) bool {
	switch node.Status {
	case ops.StatusClaimed, ops.StatusInProgress:
	default:
		return false
	}
	holder := strings.TrimSpace(node.ClaimedBy)
	if holder == "" {
		return false
	}
	if viewer != "" && holder == viewer {
		return false
	}
	return true
}

func issueFlag(argv string) string {
	fields := strings.Fields(argv)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "--issue" {
			return fields[i+1]
		}
	}
	return ""
}
