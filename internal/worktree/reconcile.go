// Package worktree provides managed worktree reconciliation and listing.
package worktree

import (
	"path/filepath"
	"sort"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// Meta describes a worktree on disk.
type Meta struct {
	Path   string
	Branch string
}

// ReconcileResult holds the classification of all worktrees and detected anomalies.
type ReconcileResult struct {
	// BoundWorktrees: worktrees with live claims (issue has ClaimedBy and WorktreePath matches)
	BoundWorktrees []string
	// Orphans: worktrees on disk with no live claim that map to a known issue
	Orphans []string
	// Ghosts: issues holding a LIVE claim whose recorded WorktreePath is missing on disk
	Ghosts []string
	// GCRemovalSet: issues in merged/cancelled status with an existing worktree
	GCRemovalSet []string
	// Unrecognized: worktree paths on disk that map to no known issue (reported by PATH)
	Unrecognized []string
}

// Reconcile classifies managed worktrees against the set of issues and their claim state.
// A BOUND worktree is associated with an issue that references it (via WorktreePath), holds a live
// claim (issue.ClaimedBy is set), and the worktree exists on disk.
// An ORPHAN is a worktree on disk with no live claim whose derived issue ID is a known issue.
// A GHOST is an issue holding a live claim (ClaimedBy set, non-terminal) whose recorded
// worktree_path doesn't exist on disk. A gc'd/merged worktree that is simply gone is the
// EXPECTED end state, not a ghost, so terminal-status issues are excluded.
// GCRemovalSet contains issues in merged/cancelled with an existing worktree.
// UNRECOGNIZED holds worktree paths that map to no known issue.
// All path comparisons normalize both sides through NormalizePath so a symlinked repo
// root does not make an identical worktree look like two different paths.
func Reconcile(worktrees []Meta, issues map[string]*materialize.Issue) ReconcileResult {
	result := ReconcileResult{
		BoundWorktrees: []string{},
		Orphans:        []string{},
		Ghosts:         []string{},
		GCRemovalSet:   []string{},
		Unrecognized:   []string{},
	}

	worktreesByPath := make(map[string]bool)
	for _, wt := range worktrees {
		worktreesByPath[NormalizePath(wt.Path)] = true
	}

	worktreeClaimed := make(map[string]bool)

	// First pass: classify each issue that records a worktree path.
	for _, issue := range issues {
		if issue == nil || issue.WorktreePath == "" {
			continue
		}

		normPath := NormalizePath(issue.WorktreePath)
		isTerminal := isTerminalStatus(issue.Status)

		if worktreesByPath[normPath] {
			switch {
			case isTerminal:
				result.GCRemovalSet = append(result.GCRemovalSet, issue.ID)
			case issue.ClaimedBy != "":
				result.BoundWorktrees = append(result.BoundWorktrees, issue.ID)
			default:
				result.Orphans = append(result.Orphans, issue.ID)
			}
			worktreeClaimed[normPath] = true
			continue
		}

		// Recorded path is missing on disk. A ghost is only a LIVE claim whose
		// worktree vanished; a terminal (merged/cancelled) issue whose worktree
		// is gone is the expected end state, not an anomaly.
		if !isTerminal && issue.ClaimedBy != "" {
			result.Ghosts = append(result.Ghosts, issue.ID)
		}
	}

	// Second pass: worktrees on disk not tied to any issue's recorded path.
	for _, wt := range worktrees {
		if worktreeClaimed[NormalizePath(wt.Path)] {
			continue
		}
		issueID := extractIssueIDFromWorktreePath(wt.Path)
		if issueID != "" && issues[issueID] != nil {
			result.Orphans = append(result.Orphans, issueID)
		} else {
			result.Unrecognized = append(result.Unrecognized, wt.Path)
		}
	}

	// Deterministic output: map iteration order is nondeterministic.
	sort.Strings(result.BoundWorktrees)
	sort.Strings(result.Orphans)
	sort.Strings(result.Ghosts)
	sort.Strings(result.GCRemovalSet)
	sort.Strings(result.Unrecognized)

	return result
}

// isTerminalStatus returns true if the issue status is one where worktrees should be removed.
func isTerminalStatus(status string) bool {
	return status == ops.StatusMerged || status == ops.StatusCancelled
}

// extractIssueIDFromWorktreePath extracts the issue ID from a worktree path.
// Expected format: <repo-root>/.worktrees/<issue-id>
// Returns the base name if extractable, empty string otherwise. The caller
// validates the derived ID against the known issue set.
func extractIssueIDFromWorktreePath(path string) string {
	base := filepath.Base(path)
	if base != "" && base != "." && base != ".." {
		return base
	}
	return ""
}
