package review

import (
	"fmt"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/commitref"
)

// ReviewCommits discovers and returns all delivery commits for an issue
// across all conventional-commit type prefixes (feat, fix, refactor, test,
// docs, chore, etc.), plus the documented merge-commit reference form.
//
// Commits are matched by either:
//   - the typed prefix `TYPE(ISSUE-ID):` (or `TYPE(ISSUE-ID)!:`), where TYPE
//     is restricted to the commit types enumerated by docs/conventions.md, or
//   - the merge form `merge: ISSUE-ID description` (see docs/conventions.md),
//     so an issue delivered solely via a merge commit is still discoverable
//     here — the delivery gate's CommitReferenceCheck already accepts this
//     form (internal/deliverygate/gate.go).
//
// Both patterns are built by internal/commitref (TypedCommitPattern /
// MergeCommitPattern), the same shared source the delivery gate itself uses,
// so this discovery check and the gate's pass/fail check can't
// independently drift apart on which types/forms they recognize — the exact
// bug class that left the merge form unrecognized here.
func ReviewCommits(git *adapters.Client, issueID string, branch string) ([]adapters.LogEntry, error) {
	if branch == "" {
		branch = "HEAD"
	}

	// Get all commits on the requested branch (e.g. a task or story branch,
	// not necessarily whatever happens to be checked out).
	entries, err := git.LogBranch(branch, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	// Initialize as an empty (non-nil) slice so the no-match case marshals to
	// JSON "[]" rather than "null" for agent consumers.
	results := []adapters.LogEntry{}
	for _, entry := range entries {
		// LogBranch's format string produces a single-line subject, so no
		// further newline-splitting is needed here.
		//
		// commitref.IsValidReference is the single shared decision point for
		// "does this commit satisfy issueID" (typed form, or merge form on a
		// genuine 2+-parent commit), shared with
		// internal/deliverygate.CommitReferenceCheck so this discovery check
		// and the gate's pass/fail check can't independently drift apart —
		// see its doc comment for the bug class this structurally prevents.
		if commitref.IsValidReference(entry.Subject, entry.ParentCount(), issueID) {
			results = append(results, entry)
		}
	}

	// Don't reverse; return in git log order (newest first)
	return results, nil
}
