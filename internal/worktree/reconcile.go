// Package worktree provides managed worktree reconciliation and listing.
package worktree

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// Meta describes a worktree on disk.
type Meta struct {
	Path   string
	Branch string
	// IssueID is the authoritative worktree→issue binding, read from the
	// worktree's own armature-issue-id marker file (the same binding the
	// removal layer verifies via harnesshook.ReadIssueBindingFileErr). When
	// set, it — not the path basename — determines the worktree's issue
	// identity during reconciliation, so a slash-bearing issue ID (or any ID
	// that does not round-trip through filepath.Base) is classified correctly.
	// An empty IssueID falls back to the path basename, preserving behavior for
	// callers/tests that do not populate the binding.
	IssueID string
}

// ReconcileResult holds the classification of all worktrees and detected anomalies.
type ReconcileResult struct {
	// BoundWorktrees: local worktrees whose issue holds a live, non-stale claim
	BoundWorktrees []string
	// Orphans: local worktrees whose issue has no live claim (unclaimed or stale)
	Orphans []string
	// Ghosts: issues holding a LIVE claim whose recorded WorktreePath is missing on disk
	Ghosts []string
	// GCRemovalSet: issues in merged/cancelled status with a local worktree on disk
	GCRemovalSet []string
	// Unrecognized: worktree paths on disk that map to no known issue (reported by PATH)
	Unrecognized []string
}

// Reconcile classifies managed worktrees against the set of issues and their claim state.
//
// Classification is driven from THIS clone's on-disk worktrees (the []Meta), not
// from the git-replicated absolute issue.WorktreePath: the removal layer
// (removeWorktreeForIssueTracked) locates a worktree by branch in THIS clone, so
// selection must key on the same clone-local signal to agree with it. Each local
// worktree's issue is derived from its path (the base name is the issue ID), then
// classified by the issue's status AND claim staleness:
//   - terminal issue (merged/cancelled) -> GCRemovalSet (a clone-local terminal
//     worktree is gc-ready even when the recorded WorktreePath points at a foreign
//     or reused clone)
//   - live, non-stale claim (ClaimedBy set, TTL not expired) -> BoundWorktrees
//   - anything else (unclaimed, or a claim past its TTL) -> Orphans
//   - a worktree mapping to no known issue -> Unrecognized (by PATH)
//
// A GHOST is an issue holding a live claim (ClaimedBy set, non-terminal) whose
// recorded worktree_path has no matching local worktree. A gc'd/merged worktree
// that is simply gone is the EXPECTED end state, so terminal-status issues are
// excluded. Staleness reuses claim.IsClaimStale against now so a claim past its
// TTL is treated as no-longer-live.
//
// managedRoots optionally scopes ghost detection to worktrees this clone owns.
// A live claim's recorded WorktreePath is an absolute path captured in the
// claiming clone and git-replicated to every clone; a claim owned by a remote
// clone can never match this clone's local worktrees, so treating it as a ghost
// here would be a false positive. When one or more managedRoots are supplied
// (normalized, trailing-separator prefixes of this clone's managed worktree
// directory), a missing worktree is only a ghost when its recorded path falls
// under one of them. When none are supplied, ghost scoping is disabled and all
// live claims are eligible — preserving behavior for callers/tests that don't scope.
func Reconcile(worktrees []Meta, issues map[string]*materialize.Issue, now time.Time, managedRoots ...string) ReconcileResult {
	result := ReconcileResult{
		BoundWorktrees: []string{},
		Orphans:        []string{},
		Ghosts:         []string{},
		GCRemovalSet:   []string{},
		Unrecognized:   []string{},
	}

	// accountedFor tracks issues that have a local worktree, so the ghost pass
	// (which looks for live claims with NO local worktree) can skip them.
	accountedFor := make(map[string]bool)

	// First pass: drive classification from THIS clone's on-disk worktrees.
	// Identity comes from the authoritative armature-issue-id binding (wt.IssueID)
	// the caller reads off each worktree; only when that is absent do we fall back
	// to the path basename (legacy callers/tests). The basename is a weak signal —
	// it truncates slash-bearing IDs — so the binding is preferred whenever present.
	for _, wt := range worktrees {
		issueID := wt.IssueID
		if issueID == "" {
			issueID = extractIssueIDFromWorktreePath(wt.Path)
		}
		issue := issues[issueID]
		if issueID == "" || issue == nil {
			result.Unrecognized = append(result.Unrecognized, wt.Path)
			continue
		}
		accountedFor[issueID] = true

		switch {
		case isTerminalStatus(issue.Status):
			result.GCRemovalSet = append(result.GCRemovalSet, issueID)
		case issue.ClaimedBy != "" && !issue.ClaimStale(now.Unix()):
			result.BoundWorktrees = append(result.BoundWorktrees, issueID)
		default:
			// Unclaimed, or a claim past its TTL: worktree with no live claim.
			result.Orphans = append(result.Orphans, issueID)
		}
	}

	// Second pass: issues holding a live claim whose worktree is missing on disk
	// are ghosts. A terminal issue whose worktree is gone is the expected end
	// state, not an anomaly, so terminal issues are excluded.
	for _, issue := range issues {
		if issue == nil || issue.WorktreePath == "" {
			continue
		}
		if accountedFor[issue.ID] {
			continue
		}
		// The worktree is missing on disk (that's the ghost condition), so the
		// recorded path's leaf cannot be symlink-resolved directly. Resolve its
		// existing parent instead so the managed-root prefix test stays symmetric
		// with the EvalSymlinks-resolved roots even when the repo root is reached
		// through a symlink (WSL /mnt/c, macOS /tmp→/private/tmp, symlinked $HOME).
		normPath := NormalizePathAllowingMissing(issue.WorktreePath)
		if !isTerminalStatus(issue.Status) && issue.ClaimedBy != "" &&
			!issue.ClaimStale(now.Unix()) &&
			isUnderManagedRoot(normPath, managedRoots) {
			result.Ghosts = append(result.Ghosts, issue.ID)
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

// isUnderManagedRoot reports whether normPath falls under one of the supplied
// managed roots. With no roots supplied, scoping is disabled and it returns true
// (legacy behavior). Roots are expected to be NormalizePath'd, trailing-separator
// prefixes; normPath is expected to already be normalized by the caller.
func isUnderManagedRoot(normPath string, managedRoots []string) bool {
	if len(managedRoots) == 0 {
		return true
	}
	for _, root := range managedRoots {
		if root != "" && strings.HasPrefix(normPath, root) {
			return true
		}
	}
	return false
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
