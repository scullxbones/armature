// Package worktree provides managed worktree reconciliation and listing.
package worktree

import (
	"sort"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// Meta describes a worktree on disk.
type Meta struct {
	Path   string
	Branch string
	// Binding is the authoritative worktree→issue binding, read from the
	// worktree's own armature-issue-id binding file (the same binding the
	// removal layer verifies). When
	// set, it — not the path basename — determines the worktree's issue
	// identity during reconciliation, so any historical ID that does not
	// round-trip through filepath.Base is classified correctly.
	// An empty Binding is unrecognized; directory basenames are descriptions,
	// never identity.
	Binding string
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
	// GCRemovals carries the exact selected worktree metadata for each ID in
	// GCRemovalSet. Callers must remove these paths, not look them up again by
	// issue ID, because multiple binding-bound worktrees can exist for one issue.
	GCRemovals []Meta
	// GCAmbiguous lists terminal issues with more than one candidate and no
	// uniquely recorded path. Ambiguous candidates are never removed.
	GCAmbiguous []string
	// Unrecognized: worktree paths on disk that map to no known issue (reported by PATH)
	Unrecognized []string
}

// Reconcile classifies managed worktrees against the set of issues and their claim state.
//
// Classification is driven from THIS clone's on-disk worktrees (the []Meta), not
// from the git-replicated absolute issue.WorktreePath. Each local worktree's
// authoritative binding identity drives classification, then the issue's
// status and claim staleness:
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
		GCRemovals:     []Meta{},
		GCAmbiguous:    []string{},
		Unrecognized:   []string{},
	}

	// recordedPathMatches tracks the exact local path recorded by a live claim.
	// A wrong-path binding for the same issue is still an orphan and must not hide
	// the recorded-path ghost.
	recordedPathMatches := make(map[string]bool)
	gcCandidates := make(map[string][]Meta)

	// First pass: drive classification from THIS clone's on-disk worktrees.
	// Identity is the armature-issue-id binding (wt.Binding) and nothing else.
	// A worktree carrying no binding is Unrecognized — its directory basename is
	// NOT an identity and must never be promoted to one. Inferring an issue from
	// the basename would report a live claim as BOUND while doctor and the
	// delivery gate, which both require the binding, reject the very same
	// worktree: the anomaly would be suppressed exactly where an agent reads it.
	for _, wt := range worktrees {
		issueID := wt.Binding
		issue := issues[issueID]
		if issueID == "" || issue == nil {
			result.Unrecognized = append(result.Unrecognized, wt.Path)
			continue
		}
		if issue.WorktreePath != "" && NormalizePathAllowingMissing(wt.Path) == NormalizePathAllowingMissing(issue.WorktreePath) {
			recordedPathMatches[issueID] = true
		}

		switch {
		case isTerminalStatus(issue.Status):
			gcCandidates[issueID] = append(gcCandidates[issueID], wt)
		case issue.ClaimedBy != "" && !issue.ClaimStale(now.Unix()):
			// A claim's absolute WorktreePath identifies the clone that owns the
			// claim. A binding in this clone is not enough to call a local path
			// bound when the materialized claim points at another clone; classify
			// that local checkout as an orphan so it cannot be mistaken for the
			// claimant's live worktree.
			if issue.WorktreePath == "" || NormalizePathAllowingMissing(wt.Path) == NormalizePathAllowingMissing(issue.WorktreePath) {
				result.BoundWorktrees = append(result.BoundWorktrees, issueID)
			} else {
				result.Orphans = append(result.Orphans, issueID)
			}
		default:
			// Unclaimed, or a claim past its TTL: worktree with no live claim.
			result.Orphans = append(result.Orphans, issueID)
		}
	}

	// Select terminal worktrees by exact recorded path where available. If the
	// recorded path cannot disambiguate multiple binding-bound candidates, leave
	// the issue out of the removal set rather than guessing.
	for issueID, candidates := range gcCandidates {
		issue := issues[issueID]
		selected, ok := selectGCRemoval(issue, candidates)
		if !ok {
			result.GCAmbiguous = append(result.GCAmbiguous, issueID)
			continue
		}
		result.GCRemovalSet = append(result.GCRemovalSet, issueID)
		result.GCRemovals = append(result.GCRemovals, selected)
	}

	// Second pass: issues holding a live claim whose worktree is missing on disk
	// are ghosts. A terminal issue whose worktree is gone is the expected end
	// state, not an anomaly, so terminal issues are excluded.
	for _, issue := range issues {
		if issue == nil || issue.WorktreePath == "" {
			continue
		}
		if recordedPathMatches[issue.ID] {
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
	sort.Slice(result.GCRemovals, func(i, j int) bool {
		if result.GCRemovals[i].Binding == result.GCRemovals[j].Binding {
			return result.GCRemovals[i].Path < result.GCRemovals[j].Path
		}
		return result.GCRemovals[i].Binding < result.GCRemovals[j].Binding
	})
	sort.Strings(result.GCAmbiguous)
	sort.Strings(result.Unrecognized)

	return result
}

func selectGCRemoval(issue *materialize.Issue, candidates []Meta) (Meta, bool) {
	if len(candidates) == 0 {
		return Meta{}, false
	}
	if issue != nil && issue.WorktreePath != "" {
		var matches []Meta
		for _, candidate := range candidates {
			if NormalizePathAllowingMissing(candidate.Path) == NormalizePathAllowingMissing(issue.WorktreePath) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 {
			return matches[0], true
		}
		if len(matches) > 1 {
			return Meta{}, false
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return Meta{}, false
}

// isUnderManagedRoot reports whether normPath falls under one of the supplied
// managed roots. With no roots supplied, scoping is disabled and it returns true
// (legacy behavior). Both roots and paths are normalized, and a path-separator
// boundary is required so a sibling such as .worktrees-old cannot match.
func isUnderManagedRoot(normPath string, managedRoots []string) bool {
	if len(managedRoots) == 0 {
		return true
	}
	for _, root := range managedRoots {
		if root != "" && IsUnderRoot(normPath, root) {
			return true
		}
	}
	return false
}

// isTerminalStatus returns true if the issue status is one where worktrees should be removed.
func isTerminalStatus(status string) bool {
	return status == ops.StatusMerged || status == ops.StatusCancelled
}
