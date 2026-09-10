package worktree

import (
	"fmt"
	"slices"
	"strings"
)

// InventoryRow is one observed worktree used as PlanProvision input.
// Path, Branch, and Binding are facts gathered by cmd/; this package
// does not inspect git.
type InventoryRow struct {
	Path    string
	Branch  string
	Binding string
}

// ProvisionInput is the borrowed, read-only fact set for a proposed
// Managed Worktree destination. Empty IssueID, Dest, or ExpectedBranch is
// an input error. DestExists is supplied by the caller and unused by the
// decision itself.
type ProvisionInput struct {
	IssueID        string
	Dest           string
	ExpectedBranch string
	Inventory      []InventoryRow
	DestExists     bool
	InRepo         bool
	UnderCanonical bool
	NestedUnder    string // registered path if dest is nested, else empty
	ProvenanceOK   bool   // trusted branch-point on the adopt candidate
}

// ProvisionAction is the PlanProvision outcome.
type ProvisionAction string

const (
	ProvisionRefuse        ProvisionAction = "refuse"
	ProvisionAdopt         ProvisionAction = "adopt"
	ProvisionAlreadyAtDest ProvisionAction = "already_at_dest"
	ProvisionFresh         ProvisionAction = "fresh"
)

// ProvisionPlan is the decision for a proposed destination, produced
// before any git or filesystem mutation.
type ProvisionPlan struct {
	Action       ProvisionAction
	AdoptFrom    string
	RefuseReason string
}

// PlanProvision decides refuse, adopt, already-at-destination, or
// provision-fresh from inventory and destination facts. Evaluation order
// is nested dest, in-repo dest, then bound-inventory cardinality.
func PlanProvision(in ProvisionInput) (ProvisionPlan, error) {
	if in.IssueID == "" || in.Dest == "" || in.ExpectedBranch == "" {
		return ProvisionPlan{}, fmt.Errorf("provision: IssueID, Dest, and ExpectedBranch are required")
	}
	if in.NestedUnder != "" {
		return ProvisionPlan{
			Action: ProvisionRefuse,
			RefuseReason: fmt.Sprintf(
				"custom worktree destination %s is nested inside registered worktree %s",
				in.Dest, in.NestedUnder),
		}, nil
	}
	if in.InRepo && !in.UnderCanonical {
		return ProvisionPlan{
			Action: ProvisionRefuse,
			RefuseReason: fmt.Sprintf(
				"custom worktree destination %s is inside the repository; explicit destinations must be outside the repository or under canonical .worktrees",
				in.Dest),
		}, nil
	}

	var bound []InventoryRow
	for _, row := range in.Inventory {
		if row.Binding == in.IssueID {
			bound = append(bound, row)
		}
	}
	if len(bound) > 1 {
		paths := make([]string, 0, len(bound))
		for _, row := range bound {
			paths = append(paths, row.Path)
		}
		slices.Sort(paths)
		return ProvisionPlan{
			Action: ProvisionRefuse,
			RefuseReason: fmt.Sprintf(
				"issue %s is bound to %d worktrees (%s); remove the armature-issue-id binding from the ones you do not want before claiming",
				in.IssueID, len(bound), strings.Join(paths, ", ")),
		}, nil
	}
	if len(bound) == 1 {
		row := bound[0]
		if row.Path == in.Dest {
			return ProvisionPlan{Action: ProvisionAlreadyAtDest}, nil
		}
		want := "refs/heads/" + in.ExpectedBranch
		if row.Branch != want {
			head := row.Branch
			if head == "" {
				head = "detached HEAD"
			}
			return ProvisionPlan{
				Action: ProvisionRefuse,
				RefuseReason: fmt.Sprintf(
					"worktree at %s is bound to %s but is on %s, not %s; finish or abandon the in-progress git operation there and check out %s before claiming",
					row.Path, in.IssueID, head, in.ExpectedBranch, in.ExpectedBranch),
			}, nil
		}
		if !in.ProvenanceOK {
			return ProvisionPlan{
				Action: ProvisionRefuse,
				RefuseReason: "adopted worktree has no recorded branch-point provenance; " +
					"re-claim it from a managed worktree or use --skip-delivery-gate only with an explicit override",
			}, nil
		}
		return ProvisionPlan{Action: ProvisionAdopt, AdoptFrom: row.Path}, nil
	}
	return ProvisionPlan{Action: ProvisionFresh}, nil
}
