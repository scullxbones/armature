// Package deliverygate evaluates a worktree against an issue's delivery
// requirements (clean tree, scope containment, commit reference) without
// mutating any state.
package deliverygate

import (
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/commitref"
	"github.com/scullxbones/armature/internal/scopematch"
)

type checkResult struct {
	Pass        bool
	Remediation string
}

type gateResult struct {
	CleanTree        checkResult
	ScopeContainment checkResult
	CommitReference  checkResult
}

// DeliveryGate evaluates clean tree, scope containment, and commit reference.
// Each check exposes Pass and Remediation.
func DeliveryGate(worktreePath, issueID, baseCommit string, scope []string) *gateResult {
	rng, commitRef := commitReferenceCheck(worktreePath, baseCommit, issueID)
	return &gateResult{
		CleanTree:        cleanTreeCheck(worktreePath),
		ScopeContainment: scopeContainmentCheck(worktreePath, rng.base, rng.head, scope),
		CommitReference:  commitRef,
	}
}

func cleanTreeCheck(worktreePath string) checkResult {
	git := adapters.New(worktreePath)

	entries, err := git.DirtyEntries()
	if err != nil {
		return checkResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to check tree status: %v", err),
		}
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if armatureStateNoise(entry) {
			continue
		}
		paths = append(paths, entry.Path)
	}

	if len(paths) > 0 {
		return checkResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"Working tree is not clean. Commit or discard changes to: %s",
				strings.Join(paths, ", "),
			),
		}
	}

	return checkResult{Pass: true, Remediation: ""}
}

func armatureStateNoise(entry adapters.DirtyEntry) bool {
	const armatureStateDir = ".armature/"
	return strings.HasPrefix(entry.Path, armatureStateDir) &&
		(entry.OldPath == "" || strings.HasPrefix(entry.OldPath, armatureStateDir))
}

func scopeContainmentCheck(worktreePath, baseCommit, head string, scope []string) checkResult {
	git := adapters.New(worktreePath)

	entries, err := git.DiffNameStatusRange(baseCommit, head)
	if err != nil {
		return checkResult{
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

	isInScope, outOfScopeFile := scopematch.IsWithinScope(files, scope)
	if !isInScope {
		return checkResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"Delivery diff contains file outside declared scope: %s (scope: %v)",
				outOfScopeFile, scope,
			),
		}
	}

	return checkResult{Pass: true, Remediation: ""}
}

func commitReferenceCheck(worktreePath, baseCommit, issueID string) (deliveryRange, checkResult) {
	git := adapters.New(worktreePath)
	return deliveryRef(git, baseCommit, issueID)
}

type deliveryRange struct {
	base string
	head string
}

func deliveryRef(git *adapters.Client, baseCommit, issueID string) (deliveryRange, checkResult) {
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
	return logContainsMatchingReference(entries, issueID)
}

func logContainsMatchingReference(entries []adapters.LogEntry, issueID string) bool {
	for _, entry := range entries {
		if commitref.IsValidReference(entry.Subject, entry.ParentCount(), issueID) {
			return true
		}
	}
	return false
}

func isolatedPrimaryEvidence(git *adapters.Client, baseCommit, primary, issueID string) (deliveryRange, checkResult, bool) {
	entries, err := git.LogRange(baseCommit, primary)
	if err != nil {
		return deliveryRange{}, checkResult{}, false
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
	return deliveryRange{}, checkResult{}, false
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

func commitReferenceAgainst(git *adapters.Client, baseCommit, head, issueID string) checkResult {
	entries, err := git.LogRange(baseCommit, head)
	if err != nil {
		return checkResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get commits: %v", err),
		}
	}

	if !logContainsMatchingReference(entries, issueID) {
		return checkResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"No commits found matching conventional-commit format %s(<ISSUE-ID>): ... since %s",
				"[type]", baseCommit,
			),
		}
	}

	empty, err := netDeliveryEmpty(git, baseCommit, head)
	if err != nil {
		return checkResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get diff: %v", err),
		}
	}
	if empty {
		return checkResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"A commit matches conventional-commit format but no net changes remain in the diff since %s "+
					"(the change was likely undone by a later commit)",
				baseCommit,
			),
		}
	}

	return checkResult{Pass: true, Remediation: ""}
}

func netDeliveryEmpty(git *adapters.Client, baseCommit, head string) (bool, error) {
	diffEntries, err := git.DiffNameStatusRange(baseCommit, head)
	if err != nil {
		return false, err
	}
	return len(diffEntries) == 0, nil
}
