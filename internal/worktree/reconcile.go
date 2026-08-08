// Package worktree provides managed worktree reconciliation and listing.
package worktree

import (
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// WorktreeMeta describes a worktree on disk.
type WorktreeMeta struct {
	Path   string
	Branch string
}

// ReconcileResult holds the classification of all worktrees and detected anomalies.
type ReconcileResult struct {
	// BoundWorktrees: worktrees with live claims (issue has ClaimedBy and WorktreePath matches)
	BoundWorktrees []string
	// Orphans: worktrees on disk with no live claim
	Orphans []string
	// Ghosts: issues with recorded WorktreePath that no longer exists on disk
	Ghosts []string
	// GCRemovalSet: issues in merged/cancelled status with an existing worktree
	GCRemovalSet []string
}

// Reconcile classifies managed worktrees against the set of issues and their claim state.
// A BOUND worktree is associated with an issue that references it (via WorktreePath), holds a live
// claim (issue.ClaimedBy is set), and the worktree exists on disk.
// An ORPHAN is a worktree on disk with no live claim: either no issue references it, or the
// referencing issue exists but its ClaimedBy is empty.
// A GHOST is an issue with a recorded worktree_path that doesn't exist on disk.
// GCRemovalSet contains issues in merged/cancelled with an existing worktree.
func Reconcile(worktrees []WorktreeMeta, issues map[string]*materialize.Issue) ReconcileResult {
	result := ReconcileResult{
		BoundWorktrees: []string{},
		Orphans:        []string{},
		Ghosts:         []string{},
		GCRemovalSet:   []string{},
	}

	// Create maps for fast lookup
	worktreesByPath := make(map[string]bool)
	for _, wt := range worktrees {
		worktreesByPath[wt.Path] = true
	}

	worktreeClaimed := make(map[string]bool)

	// First pass: classify each issue
	for _, issue := range issues {
		if issue == nil {
			continue
		}

		hasWorktreePath := issue.WorktreePath != ""
		isTerminal := isTerminalStatus(issue.Status)
		worktreeExists := worktreesByPath[issue.WorktreePath]

		if hasWorktreePath {
			if worktreeExists {
				switch {
				case isTerminal:
					// Terminal status: in removal set
					result.GCRemovalSet = append(result.GCRemovalSet, issue.ID)
				case issue.ClaimedBy != "":
					// Live claim held: bound worktree (actively in use)
					result.BoundWorktrees = append(result.BoundWorktrees, issue.ID)
				default:
					// Worktree exists but no live claim: orphan per contract
					result.Orphans = append(result.Orphans, issue.ID)
				}
				worktreeClaimed[issue.WorktreePath] = true
			} else {
				// Ghost: recorded path doesn't exist on disk
				result.Ghosts = append(result.Ghosts, issue.ID)
			}
		}
	}

	// Second pass: find orphans (worktrees on disk not claimed by any issue)
	for _, wt := range worktrees {
		if !worktreeClaimed[wt.Path] {
			// Extract issue ID from worktree path (.worktrees/<issue-id>)
			issueID := extractIssueIDFromWorktreePath(wt.Path)
			if issueID != "" {
				result.Orphans = append(result.Orphans, issueID)
			}
		}
	}

	return result
}

// isTerminalStatus returns true if the issue status is one where worktrees should be removed.
func isTerminalStatus(status string) bool {
	return status == ops.StatusMerged || status == ops.StatusCancelled
}

// extractIssueIDFromWorktreePath extracts the issue ID from a worktree path.
// Expected format: <repo-root>/.worktrees/<issue-id>
// Returns the issue ID if extractable, empty string otherwise.
func extractIssueIDFromWorktreePath(path string) string {
	// Get the base name of the path
	base := filepath.Base(path)
	// Check if it looks like a valid issue ID (contains at least one letter or digit)
	if base != "" && base != "." && base != ".." {
		return base
	}
	return ""
}
