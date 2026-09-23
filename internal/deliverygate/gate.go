// Package deliverygate evaluates a worktree against an issue's delivery
// requirements (clean tree, scope containment, commit reference) without
// mutating any state.
package deliverygate

import (
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/commitref"
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
// 2. Scope containment: selected delivery range is subset of declared scope
// 3. Commit reference: at least one commit matches conventional-commit format
//
// CommitReference and ScopeContainment share one (rangeBase, rangeHead)
// pair. Worktree-first that is claimBase..HEAD; primary-branch fallback
// isolates the complete landing that delivered the issue — an enclosing
// merge's first-parent..M when a matching commit arrived via a 2+-parent
// merge, otherwise first-parent..matching-SHA for a squash / single-parent
// landing — so later unrelated commits on main/master are not attributed
// to this issue.
//
// Returns a structured GateResult with per-check results and remediations.
// Performs no state mutation — only reads and reports.
func DeliveryGate(worktreePath, issueID, baseCommit string, scope []string) *GateResult {
	rng, commitRef := commitReferenceCheck(worktreePath, baseCommit, issueID)
	return &GateResult{
		CleanTree:        cleanTreeCheck(worktreePath),
		ScopeContainment: scopeContainmentCheck(worktreePath, rng.base, rng.head, scope),
		CommitReference:  commitRef,
	}
}

// CleanTreeCheck verifies that git status --porcelain is empty.
// Returns (Pass: true, Remediation: "") if the tree is clean,
// or (Pass: false, Remediation: "...message...") if there are uncommitted changes.
func cleanTreeCheck(worktreePath string) CheckResult {
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
// delivery head (as produced by GatedBaseCommit), not an arbitrary ref —
// the diff below uses two-dot (baseCommit..head) semantics, which silently
// includes commits reachable from baseCommit but not from head if baseCommit
// is a raw branch tip rather than a merge-base. (baseCommit, head) is the
// same pair CommitReference used: claimBase..HEAD on the worktree-first
// path, or the complete isolated landing (enclosing merge first-parent..M,
// or first-parent..matching-SHA) when primary-branch fallback selected
// evidence.
func scopeContainmentCheck(worktreePath, baseCommit, head string, scope []string) CheckResult {
	git := adapters.New(worktreePath)

	// Get the list of file changes since base commit, with rename detection
	// enabled so both the source and destination paths of any rename are
	// checked against scope. `git diff --name-only` alone would report only
	// the destination path of a rename, masking an out-of-scope original
	// location (e.g. a rename that moves a file from outside scope into
	// scope, silently deleting the out-of-scope original).
	entries, err := git.DiffNameStatusRange(baseCommit, head)
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
//     docs/conventions.md). The worktree HEAD is searched first; if it has
//     no matching evidence, each resolvable of refs/heads/main then
//     refs/heads/master is searched until a matching landing isolates and
//     passes, so a squash-land on the primary branch still counts even when
//     a stale local main exists.
//  2. Net delivery is non-empty: the tree diff of the selected range is
//     non-empty (reusing the same diff primitive as ScopeContainmentCheck).
//     DeliveryGate feeds ScopeContainment that same (rangeBase, rangeHead).
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
// current branch (as produced by GatedBaseCommit), not an arbitrary ref —
// LogRange and the net diff below use two-dot (baseCommit..HEAD) semantics,
// which is only correct when baseCommit is the real divergence point.
func commitReferenceCheck(worktreePath, baseCommit, issueID string) (deliveryRange, CheckResult) {
	git := adapters.New(worktreePath)
	return deliveryRef(git, baseCommit, issueID)
}

// deliveryRange is the exclusive-base, inclusive-head pair both
// CommitReference and ScopeContainment evaluate for one gate pass.
type deliveryRange struct {
	base string
	head string
}

// deliveryRef picks one delivery range for the gate pass: claimBase..HEAD
// when the worktree has CommitReference evidence, otherwise the complete
// isolated landing on the first primary-branch ref that supplies passing
// evidence. ScopeContainmentCheck must use this same pair so an empty stale
// worktree cannot pass while an out-of-scope landing on the primary branch
// is ignored, and so later unrelated primary-branch commits are not
// attributed to this issue.
func deliveryRef(git *adapters.Client, baseCommit, issueID string) (deliveryRange, CheckResult) {
	worktreeRange := deliveryRange{base: baseCommit, head: "HEAD"}
	worktreeResult := commitReferenceAgainst(git, worktreeRange.base, worktreeRange.head, issueID)
	if worktreeResult.Pass {
		return worktreeRange, worktreeResult
	}
	// Worktree-first stays claimBase..HEAD whenever a matching conventional
	// commit exists there — including empty-net reverts, which must fail
	// closed on that range. Primary isolation only runs when the claim
	// worktree has no matching subject (stale task branch after squash-land).
	if hasMatchingReference(git, worktreeRange.base, worktreeRange.head, issueID) {
		return worktreeRange, worktreeResult
	}
	// After a remote land, matching conventional commits live on
	// main/master while the claim worktree is still on the stale task
	// branch. Search every resolvable primary candidate until a complete
	// isolated landing passes so a stale local main cannot hide a valid
	// master landing.
	for _, primary := range primaryBranchRefs(git) {
		if rng, primaryResult, ok := isolatedPrimaryEvidence(git, baseCommit, primary, issueID); ok {
			return rng, primaryResult
		}
	}
	return worktreeRange, worktreeResult
}

func hasMatchingReference(git *adapters.Client, baseCommit, head, issueID string) bool {
	entries, err := git.LogRange(baseCommit, head)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if commitref.IsValidReference(entry.Subject, entry.ParentCount(), issueID) {
			return true
		}
	}
	return false
}

// isolatedPrimaryEvidence finds matching conventional commits on primary
// (LogRange is newest-first) and evaluates CommitReference against the
// complete landing that delivered the newest usable match: the nearest
// enclosing merge (first-parent..M) when that match was introduced by a
// 2+-parent merge, otherwise first-parent..matching-SHA. Commits with no
// parent are skipped (fail closed). Isolation is range selection only: no
// byte-level attribution of whether that commit's content survived to the
// primary tip.
func isolatedPrimaryEvidence(git *adapters.Client, baseCommit, primary, issueID string) (deliveryRange, CheckResult, bool) {
	entries, err := git.LogRange(baseCommit, primary)
	if err != nil {
		return deliveryRange{}, CheckResult{}, false
	}
	for _, entry := range entries {
		if !commitref.IsValidReference(entry.Subject, entry.ParentCount(), issueID) {
			continue
		}
		if len(entry.Parents) == 0 {
			continue
		}
		rng := completePrimaryLanding(git, entries, entry)
		result := commitReferenceAgainst(git, rng.base, rng.head, issueID)
		if result.Pass {
			return rng, result, true
		}
	}
	return deliveryRange{}, CheckResult{}, false
}

// completePrimaryLanding selects the delivery range for one matching
// commit: prefer the newest enclosing merge landing, else isolate that
// commit to first-parent..SHA (squash / single-parent).
func completePrimaryLanding(git *adapters.Client, rangeEntries []adapters.LogEntry, matching adapters.LogEntry) deliveryRange {
	if rng, ok := enclosingMergeLanding(git, rangeEntries, matching); ok {
		return rng
	}
	return deliveryRange{base: matching.Parents[0], head: matching.SHA}
}

// enclosingMergeLanding returns first-parent..M for the newest genuine
// merge M in rangeEntries such that matching is reachable from M but not
// from M.Parents[0] (the commits that merge brought onto the primary line).
func enclosingMergeLanding(git *adapters.Client, rangeEntries []adapters.LogEntry, matching adapters.LogEntry) (deliveryRange, bool) {
	for _, merge := range rangeEntries {
		if len(merge.Parents) < 2 {
			continue
		}
		if merge.SHA == matching.SHA {
			return deliveryRange{base: merge.Parents[0], head: merge.SHA}, true
		}
		introduced, err := git.LogRange(merge.Parents[0], merge.SHA)
		if err != nil {
			continue
		}
		for _, entry := range introduced {
			if entry.SHA == matching.SHA {
				return deliveryRange{base: merge.Parents[0], head: merge.SHA}, true
			}
		}
	}
	return deliveryRange{}, false
}

func primaryBranchRefs(git *adapters.Client) []string {
	var refs []string
	for _, ref := range []string{"refs/heads/main", "refs/heads/master"} {
		if _, err := git.ResolveRevision(ref); err == nil {
			refs = append(refs, ref)
		}
	}
	return refs
}

func commitReferenceAgainst(git *adapters.Client, baseCommit, head, issueID string) CheckResult {
	// Get only commits strictly after baseCommit (exclusive) up to head.
	entries, err := git.LogRange(baseCommit, head)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get commits: %v", err),
		}
	}

	// A matching commit is either the typed form
	// (type(ISSUE-ID): <description>, type restricted to the set
	// docs/conventions.md documents) or the merge form
	// (merge: ISSUE-ID <description>) on a genuine merge commit — see
	// commitref.IsValidReference's doc comment for the full rationale.
	foundMatchingCommit := false
	for _, entry := range entries {
		// commitref.IsValidReference is the single shared decision point for
		// "does this commit satisfy issueID" (typed form, or merge form on a
		// genuine 2+-parent commit) — see its doc comment for why this must
		// not be reimplemented independently here.
		if commitref.IsValidReference(entry.Subject, entry.ParentCount(), issueID) {
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
	// net base..head diff to be non-empty as independent evidence that
	// something was actually delivered.
	diffEntries, err := git.DiffNameStatusRange(baseCommit, head)
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
