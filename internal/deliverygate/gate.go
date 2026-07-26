// Package deliverygate evaluates a worktree against an issue's delivery
// requirements (clean tree, scope containment, commit reference) without
// mutating any state.
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

	// arm's own materialized state under .armature/ is expected to be
	// gitignored in any repo using armature; treat it as noise here rather
	// than depending on every caller's .gitignore being correctly set up.
	// A rename's OldPath must also be under .armature/ before the entry is
	// ignored: DirtyEntries only reports OldPath for renames (empty
	// otherwise), so a rename from a tracked file outside .armature/ into
	// .armature/ (e.g. `git mv outside.go .armature/outside.go`) would
	// otherwise be discarded here by checking Path alone, even though it
	// effectively deletes a tracked non-armature-state file.
	const armatureStateDir = ".armature/"
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, armatureStateDir) &&
			(entry.OldPath == "" || strings.HasPrefix(entry.OldPath, armatureStateDir)) {
			continue
		}
		paths = append(paths, entry.Path)
	}

	// If there are any dirty entries, the tree is not clean
	if len(paths) > 0 {
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

	// Get the list of file changes since base commit, with rename detection
	// enabled so both the source and destination paths of any rename are
	// checked against scope. `git diff --name-only` alone would report only
	// the destination path of a rename, masking an out-of-scope original
	// location (e.g. a rename that moves a file from outside scope into
	// scope, silently deleting the out-of-scope original).
	entries, err := git.DiffNameStatus(baseCommit)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get diff: %v", err),
		}
	}

	files := make([]string, 0, len(entries)*2)
	for _, entry := range entries {
		if entry.OldPath != "" {
			files = append(files, entry.OldPath)
		}
		files = append(files, entry.Path)
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
// where type is one of feat, fix, refactor, test, docs, style, polish
// (see docs/conventions.md).
func CommitReferenceCheck(worktreePath, baseCommit, issueID string) CheckResult {
	git := adapters.New(worktreePath)

	// Get only commits strictly after baseCommit (exclusive) up to HEAD.
	entries, err := git.LogRange(baseCommit, "HEAD")
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get commits: %v", err),
		}
	}

	// Build regex pattern: ^(feat|fix|refactor|test|docs|style|polish)\(ISSUE-ID\)!?:[ \t]+\S
	// Matches: type(ISSUE-ID): <description> or type(ISSUE-ID)!: <description>
	// where type is one of the commit types enumerated by
	// docs/conventions.md. Restricting the type alternation (rather than
	// accepting any lowercase word) prevents a bogus type like
	// "oops(ISSUE-ID): ..." from satisfying this check. Requiring whitespace
	// followed by a non-whitespace character after the colon prevents a
	// bare "fix(ISSUE-ID):" with no actual description from satisfying this
	// check.
	pattern := regexp.MustCompile(`^(feat|fix|refactor|test|docs|style|polish)\(` + regexp.QuoteMeta(issueID) + `\)!?:[ \t]+\S`)

	// A commit whose subject matches the conventional-commit format but has no
	// actual diff (e.g. an empty commit created with `git commit
	// --allow-empty -m "fix(ISSUE-ID): busywork"`) delivers no real content
	// and must not satisfy this check on its own. Keep scanning past it in
	// case a later matching commit does have real content.
	// A matching commit having non-empty CommitChangedFiles is not enough on
	// its own: a later commit in the range (e.g. a revert) can cancel out
	// exactly the change the matching commit made, leaving nothing actually
	// delivered even though the matching commit "touched files" at the time
	// it was made. It is also not enough to require that the net base-to-HEAD
	// diff is merely nonempty for any reason: a fully-reverted matching commit
	// can be padded with one unrelated trivial in-scope commit, making the net
	// diff nonempty while the matching commit's own substance survives
	// nowhere in it. Require instead that the net diff contains at least one
	// path that the matching commit itself touched, so a padded self-
	// cancelling revert can't slip a zero-net-change delivery past this check.
	netDiff, err := git.DiffNameStatus(baseCommit)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get diff: %v", err),
		}
	}
	netDiffFiles := make(map[string]bool, len(netDiff)*2)
	for _, e := range netDiff {
		if e.Path != "" {
			netDiffFiles[e.Path] = true
		}
		if e.OldPath != "" {
			netDiffFiles[e.OldPath] = true
		}
	}

	foundMatchingCommit := false
	for _, entry := range entries {
		if !pattern.MatchString(entry.Subject) {
			continue
		}
		files, err := git.CommitChangedFiles(entry.SHA)
		if err != nil || len(files) == 0 {
			continue
		}
		overlaps := false
		for _, f := range files {
			if netDiffFiles[f] {
				overlaps = true
				break
			}
		}
		if !overlaps {
			continue
		}
		foundMatchingCommit = true
		break
	}

	if !foundMatchingCommit {
		return CheckResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"No commits with non-trivial, undone content found matching conventional-commit format %s(<ISSUE-ID>): ... since %s",
				"[type]", baseCommit,
			),
		}
	}

	return CheckResult{Pass: true, Remediation: ""}
}
