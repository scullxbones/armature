package review

import (
	"fmt"
)

// GitAdapter is an interface for git operations required by the Prepare function.
type GitAdapter interface {
	// ResolveRevision resolves a git revision to its full commit SHA.
	ResolveRevision(rev string) (string, error)
	// DiffRange returns the unified diff between two revisions.
	DiffRange(base, head string) (string, error)
	// DiffNameOnlyRange returns the list of changed file names between two revisions.
	DiffNameOnlyRange(base, head string) ([]string, error)
}

// Prepare builds a ReviewBundle for an issue given its contract metadata and a git range.
// It resolves the git revisions, computes the diff and changed files, and constructs
// a complete ReviewBundle with fingerprints and bundle ID.
func Prepare(git GitAdapter, issueID, title string, scope []string, criteria []string, base, head string) (*ReviewBundle, error) {
	// Resolve both revisions to full SHAs
	baseSHA, err := git.ResolveRevision(base)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base revision %s: %w", base, err)
	}

	headSHA, err := git.ResolveRevision(head)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve head revision %s: %w", head, err)
	}

	// Get the unified diff
	diff, err := git.DiffRange(baseSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to compute diff: %w", err)
	}

	// Get the list of changed files
	changedFiles, err := git.DiffNameOnlyRange(baseSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	// Build the contract
	contract := Contract{
		DefinitionOfDone: "",
		Acceptance:       criteria,
	}

	// Build the delivery
	delivery := Delivery{
		BaseSHA:      baseSHA,
		HeadSHA:      headSHA,
		ChangedFiles: changedFiles,
		Diff:         diff,
	}

	// Compute fingerprints
	contractFP := FingerprintContract(contract)
	deliveryFP := FingerprintDelivery(delivery)

	// Build the bundle (without bundleID initially)
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue: IssueInfo{
			ID:    issueID,
			Type:  "task",
			Title: title,
		},
		Contract: contract,
		Delivery: delivery,
		Fingerprints: Fingerprints{
			Contract: contractFP,
			Delivery: deliveryFP,
		},
	}

	// Compute and set the bundle ID
	bundle.BundleID = ComputeBundleID(*bundle)

	// Validate the bundle
	if err := bundle.Valid(); err != nil {
		return nil, fmt.Errorf("invalid review bundle: %w", err)
	}

	return bundle, nil
}
