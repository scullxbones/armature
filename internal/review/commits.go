package review

import (
	"fmt"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/commitref"
)

// ReviewCommits discovers delivery commits for an issue.
// Commits are matched by either:
//   - the typed prefix `TYPE(ISSUE-ID):` (or `TYPE(ISSUE-ID)!:`), where TYPE
//     is restricted to the commit types enumerated by docs/conventions.md, or
//   - the merge form `merge: ISSUE-ID description` (see docs/conventions.md).
//
// Both patterns are built by internal/commitref (TypedCommitPattern /
// MergeCommitPattern), the same shared source the delivery gate uses.
func ReviewCommits(git *adapters.Client, issueID string, branch string) ([]adapters.LogEntry, error) {
	if branch == "" {
		branch = "HEAD"
	}

	entries, err := git.LogBranch(branch, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	// Initialize as an empty (non-nil) slice so the no-match case marshals to
	// JSON "[]" rather than "null" for agent consumers.
	results := []adapters.LogEntry{}
	for _, entry := range entries {
		if commitref.IsValidReference(entry.Subject, entry.ParentCount(), issueID) {
			results = append(results, entry)
		}
	}

	return results, nil
}
