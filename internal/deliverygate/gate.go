package deliverygate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
)

// CheckResult represents the outcome of a single gate check.
type CheckResult struct {
	Pass        bool   // Whether the check passed
	Remediation string // Remediation message if check failed (empty if passed)
}

// GateResult represents the combined results of all three delivery gate checks.
type GateResult struct {
	CleanTree        CheckResult // Check 1: git status --porcelain is empty
	ScopeContainment CheckResult // Check 2: delivery diff is subset of declared scope
	CommitReference  CheckResult // Check 3: at least one commit matches conventional format
}

// DeliveryGate evaluates a worktree against an issue via three checks:
// 1. Clean tree: git status --porcelain is empty
// 2. Scope containment: delivery diff vs base is subset of declared scope
// 3. Commit reference: at least one commit matches conventional-commit format
//
// Returns a structured GateResult with per-check results and remediations.
// Performs no state mutation — only reads and reports.
func DeliveryGate(worktreePath, issueID, baseCommit string, scope []string) *GateResult {
	return &GateResult{
		CleanTree:        CleanTreeCheck(worktreePath),
		ScopeContainment: ScopeContainmentCheck(worktreePath, baseCommit, scope),
		CommitReference:  CommitReferenceCheck(worktreePath, baseCommit, issueID),
	}
}

// CleanTreeCheck verifies that git status --porcelain is empty.
// Returns (Pass: true, Remediation: "") if the tree is clean,
// or (Pass: false, Remediation: "...message...") if there are uncommitted changes.
func CleanTreeCheck(worktreePath string) CheckResult {
	git := adapters.New(worktreePath)

	// Get all dirty entries (both tracked and untracked)
	entries, err := git.DirtyEntries()
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to check tree status: %v", err),
		}
	}

	// If there are any dirty entries, the tree is not clean
	if len(entries) > 0 {
		paths := make([]string, 0, len(entries))
		for _, entry := range entries {
			paths = append(paths, entry.Path)
		}
		return CheckResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"Working tree is not clean. Commit or discard changes to: %s",
				strings.Join(paths, ", "),
			),
		}
	}

	return CheckResult{Pass: true, Remediation: ""}
}

// ScopeContainmentCheck verifies that all files changed since baseCommit
// are within the declared scope globs.
// Returns (Pass: true, Remediation: "") if all files are in scope,
// or (Pass: false, Remediation: "...message...") if any file is outside scope.
func ScopeContainmentCheck(worktreePath, baseCommit string, scope []string) CheckResult {
	git := adapters.New(worktreePath)

	// Get the list of files changed since base commit
	files, err := git.DiffNameOnly(baseCommit)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get diff: %v", err),
		}
	}

	// Use IsWithinScope to check if all files are within scope
	isInScope, outOfScopeFile := claim.IsWithinScope(files, scope)
	if !isInScope {
		return CheckResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"Delivery diff contains file outside declared scope: %s (scope: %v)",
				outOfScopeFile, scope,
			),
		}
	}

	return CheckResult{Pass: true, Remediation: ""}
}

// CommitReferenceCheck verifies that at least one commit since baseCommit
// matches the conventional-commit format with the issue ID in the scope.
// Format: <type>(<ISSUE-ID>): ... or <type>(<ISSUE-ID>)!: ...
// where type is lowercase alphabetic (feat, fix, refactor, test, docs, etc).
func CommitReferenceCheck(worktreePath, baseCommit, issueID string) CheckResult {
	git := adapters.New(worktreePath)

	// Get all commits from base to HEAD
	entries, err := git.LogBranch("HEAD", 0)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get commits: %v", err),
		}
	}

	// Build regex pattern: ^[a-z]+\(ISSUE-ID\)!?:
	// Matches: type(ISSUE-ID): or type(ISSUE-ID)!:
	pattern := regexp.MustCompile(`^[a-z]+\(` + regexp.QuoteMeta(issueID) + `\)!?:`)

	// Check if we have at least one commit from baseCommit onwards
	// that matches the conventional-commit pattern
	foundMatchingCommit := false
	for _, entry := range entries {
		// Entry.Subject is the first line of the commit message
		if pattern.MatchString(entry.Subject) {
			foundMatchingCommit = true
			break
		}

		// Stop when we reach the base commit (don't check commits before it)
		if entry.SHA == baseCommit {
			break
		}
	}

	if !foundMatchingCommit {
		return CheckResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"No commits found matching conventional-commit format %s(<ISSUE-ID>): ... since %s",
				"[type]", baseCommit,
			),
		}
	}

	return CheckResult{Pass: true, Remediation: ""}
}
