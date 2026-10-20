package worktree

import (
	"fmt"
	"slices"
	"strings"
)

// ProvisionInput is the borrowed, read-only fact set for a proposed
// Managed Worktree destination. Empty IssueID or Dest is an input error.
// Empty ExpectedBranch is an input error when Inventory is non-empty;
// dest-only nested/in-repo checks may omit it.
type ProvisionInput struct {
	IssueID        string
	Dest           string
	ExpectedBranch string
	Inventory      []Meta
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

// A nil inventory is dest-only and also records nested registration.
func PrepareProvisionInput(repoPath, dest, issueID, expectedBranch string, inventory []Meta, provenanceOK bool) (ProvisionInput, error) {
	inRepo, underCanonical := destLocationFacts(repoPath, dest)
	in := ProvisionInput{
		IssueID:        issueID,
		Dest:           dest,
		ExpectedBranch: expectedBranch,
		InRepo:         inRepo,
		UnderCanonical: underCanonical,
		ProvenanceOK:   provenanceOK,
	}
	if inventory == nil {
		nestedUnder, err := nestedRegisteredWorktree(repoPath, dest)
		if err != nil {
			return ProvisionInput{}, err
		}
		in.NestedUnder = nestedUnder
		return in, nil
	}
	in.Dest = NormalizePathAllowingMissing(dest)
	rows := make([]Meta, 0, len(inventory))
	for _, item := range inventory {
		item.Path = NormalizePath(item.Path)
		rows = append(rows, item)
	}
	in.Inventory = rows
	return in, nil
}

func destLocationFacts(repoPath, destination string) (inRepo, underCanonical bool) {
	repoRoot := NormalizePath(repoPath)
	normalizedDestination := NormalizePathAllowingMissing(destination)
	inRepo = IsUnderRoot(normalizedDestination, repoRoot)
	underCanonical = IsUnderRoot(normalizedDestination, CanonicalRoot(repoPath))
	return inRepo, underCanonical
}

func nestedRegisteredWorktree(repoPath, destination string) (string, error) {
	registeredPaths, err := RegisteredPaths(repoPath)
	if err != nil {
		return "", fmt.Errorf("inspect registered worktree destinations: %w", err)
	}
	normalizedDestination := NormalizePathAllowingMissing(destination)
	for _, registeredPath := range registeredPaths {
		normalizedRegisteredPath := NormalizePathAllowingMissing(registeredPath)
		if normalizedDestination == normalizedRegisteredPath {
			prunable, err := HasPrunableRegistration(repoPath, registeredPath)
			if err != nil {
				return "", fmt.Errorf("inspect registered worktree %s: %w", registeredPath, err)
			}
			if prunable {
				continue
			}
		}
		if IsUnderRoot(normalizedDestination, normalizedRegisteredPath) {
			return registeredPath, nil
		}
	}
	return "", nil
}

// PlanProvision decides refuse, adopt, already-at-destination, or
// provision-fresh from inventory and destination facts. Evaluation order
// is nested dest, in-repo dest, then bound-inventory cardinality.
func PlanProvision(in ProvisionInput) (ProvisionPlan, error) {
	if in.IssueID == "" || in.Dest == "" {
		return ProvisionPlan{}, fmt.Errorf("provision: IssueID and Dest are required")
	}
	if in.ExpectedBranch == "" && len(in.Inventory) > 0 {
		return ProvisionPlan{}, fmt.Errorf("provision: ExpectedBranch is required when Inventory is non-empty")
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

	var bound []Meta
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
		if NormalizePathAllowingMissing(row.Path) == NormalizePathAllowingMissing(in.Dest) {
			return ProvisionPlan{Action: ProvisionAlreadyAtDest}, nil
		}
		want := "refs/heads/" + in.ExpectedBranch
		if row.Branch != want {
			head := row.Branch
			if head == "" || head == "detached" {
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
