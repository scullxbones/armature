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
//
// Precondition: baseCommit must already be an actual merge-base of the
// current branch (as produced by ResolveBaseCommit), not an arbitrary ref —
// the diff below uses two-dot (baseCommit..HEAD) semantics, which silently
// includes commits reachable from baseCommit but not from HEAD if baseCommit
// is a raw branch tip rather than a merge-base.
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

// CommitReferenceCheck verifies two independent things since baseCommit:
//
//  1. Conventional-commit reference exists: at least one commit's subject
//     matches <type>(<ISSUE-ID>): ... or <type>(<ISSUE-ID>)!: ..., where
//     type is one of feat, fix, refactor, test, docs, style, polish (see
//     docs/conventions.md).
//  2. Net delivery is non-empty: the tree diff baseCommit..HEAD is
//     non-empty (reusing the same diff primitive as ScopeContainmentCheck).
//
// This intentionally does NOT attempt to prove that the specific matching
// commit's own diff content survives byte-for-byte to HEAD. Earlier
// implementations tried exactly that (line-heuristic, then patch-id, then
// blob-OID comparison), and each rewrite reintroduced a new edge-case bug
// because every diff shape (delete/rename/binary/merge) needs its own
// content-reconstruction logic — there is no clean git primitive for "did
// this named commit's content survive". Requiring only "a matching commit
// exists" AND "something was net-delivered" still prevents the abuse case
// this check exists for (a `type(ID): busywork` commit immediately
// reverted with nothing else delivered — the net diff is empty and the
// gate correctly fails) without needing byte-level attribution back to a
// single commit.
//
// Precondition: baseCommit must already be an actual merge-base of the
// current branch (as produced by ResolveBaseCommit), not an arbitrary ref —
// LogRange and the net diff below use two-dot (baseCommit..HEAD) semantics,
// which is only correct when baseCommit is the real divergence point.
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

	// Build regex pattern matching either:
	//   type(ISSUE-ID): <description>  or  type(ISSUE-ID)!: <description>
	//   merge: ISSUE-ID <description>
	// where type is one of the commit types enumerated by
	// docs/conventions.md. Restricting the type alternation (rather than
	// accepting any lowercase word) prevents a bogus type like
	// "oops(ISSUE-ID): ..." from satisfying this check. Requiring whitespace
	// followed by a non-whitespace character after the colon (or after the
	// issue ID, for the merge form) prevents a bare "fix(ISSUE-ID):" or
	// "merge: ISSUE-ID" with no actual description from satisfying this
	// check. The merge form uses a distinct shape (no parens around the
	// issue ID) per the documented "merge: <ISSUE-ID> <description>" special
	// case for merge commits.
	typedPattern := regexp.MustCompile(
		`^(feat|fix|refactor|test|docs|style|polish)\(` + regexp.QuoteMeta(issueID) + `\)!?:[ \t]+\S`,
	)
	mergePattern := regexp.MustCompile(`^merge:[ \t]+` + regexp.QuoteMeta(issueID) + `[ \t]+\S`)

	foundMatchingCommit := false
	for _, entry := range entries {
		if typedPattern.MatchString(entry.Subject) {
			foundMatchingCommit = true
			break
		}
		// The merge: ID description subject form is only a valid reference
		// on a GENUINE merge commit (2+ parents). Regex alone can't tell a
		// real merge commit from an ordinary single-parent commit whose
		// author merely wrote a subject that looks like the merge form —
		// accepting it there would let a fabricated "merge:" subject satisfy
		// the gate without ever actually merging anything.
		if entry.ParentCount() >= 2 && mergePattern.MatchString(entry.Subject) {
			foundMatchingCommit = true
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

	// A matching commit subject alone is not enough: a later commit in the
	// range (e.g. a revert) can cancel out exactly the change the matching
	// commit made, leaving nothing actually delivered even though the
	// matching commit "touched files" at the time it was made. Require the
	// net base..HEAD diff to be non-empty as independent evidence that
	// something was actually delivered.
	diffEntries, err := git.DiffNameStatus(baseCommit)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get diff: %v", err),
		}
	}
	if len(diffEntries) == 0 {
		return CheckResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"A commit matches conventional-commit format but no net changes remain in the diff since %s "+
					"(the change was likely undone by a later commit)",
				baseCommit,
			),
		}
	}

	return CheckResult{Pass: true, Remediation: ""}
}
