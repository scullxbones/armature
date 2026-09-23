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
	Pass        bool
	Remediation string
}

// GateResult represents the combined results of all three delivery gate checks.
type GateResult struct {
	CleanTree        CheckResult
	ScopeContainment CheckResult
	CommitReference  CheckResult
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
func DeliveryGate(worktreePath, issueID, baseCommit string, scope []string) *GateResult {
	rng, commitRef := commitReferenceCheck(worktreePath, baseCommit, issueID)
	return &GateResult{
		CleanTree:        cleanTreeCheck(worktreePath),
		ScopeContainment: scopeContainmentCheck(worktreePath, rng.base, rng.head, scope),
		CommitReference:  commitRef,
	}
}

// CleanTreeCheck verifies that git status --porcelain is empty.
func cleanTreeCheck(worktreePath string) CheckResult {
	git := adapters.New(worktreePath)

	entries, err := git.DirtyEntries()
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to check tree status: %v", err),
		}
	}

	const armatureStateDir = ".armature/"
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, armatureStateDir) &&
			(entry.OldPath == "" || strings.HasPrefix(entry.OldPath, armatureStateDir)) {
			continue
		}
		paths = append(paths, entry.Path)
	}

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

	// `git diff --name-only` alone would report only the destination path of
	// a rename, masking an out-of-scope original location.
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
// Precondition: baseCommit must already be an actual merge-base of the
// current branch (as produced by GatedBaseCommit), not an arbitrary ref —
// LogRange and the net diff below use two-dot (baseCommit..HEAD) semantics,
// which is only correct when baseCommit is the real divergence point.
func commitReferenceCheck(worktreePath, baseCommit, issueID string) (deliveryRange, CheckResult) {
	git := adapters.New(worktreePath)
	return deliveryRef(git, baseCommit, issueID)
}

type deliveryRange struct {
	base string
	head string
}

func deliveryRef(git *adapters.Client, baseCommit, issueID string) (deliveryRange, CheckResult) {
	worktreeRange := deliveryRange{base: baseCommit, head: "HEAD"}
	worktreeResult := commitReferenceAgainst(git, worktreeRange.base, worktreeRange.head, issueID)
	if worktreeResult.Pass {
		return worktreeRange, worktreeResult
	}
	if hasMatchingReference(git, worktreeRange.base, worktreeRange.head, issueID) {
		return worktreeRange, worktreeResult
	}
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

func completePrimaryLanding(git *adapters.Client, rangeEntries []adapters.LogEntry, matching adapters.LogEntry) deliveryRange {
	if rng, ok := enclosingMergeLanding(git, rangeEntries, matching); ok {
		return rng
	}
	return deliveryRange{base: matching.Parents[0], head: matching.SHA}
}

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
	entries, err := git.LogRange(baseCommit, head)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get commits: %v", err),
		}
	}

	foundMatchingCommit := false
	for _, entry := range entries {
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
