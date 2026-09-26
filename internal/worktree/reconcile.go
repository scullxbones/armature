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
// here would be a false positive. Production supplies the canonical root and
// the repository root, while callers may supply any local roots appropriate to
// their inventory. A path outside those roots is local only when it appears in
// registeredPaths, which is clone-local Git worktree evidence. When no roots or
// registrations are supplied, ghost scoping is disabled for compatibility.
func Reconcile(worktrees []Meta, issues map[string]*materialize.Issue, now time.Time, managedRoots ...string) ReconcileResult {
	return ReconcileWithLocalEvidence(worktrees, issues, now, managedRoots, nil)
}

// ReconcileWithLocalEvidence is Reconcile with an additional set of paths
// returned by this clone's Git worktree registry. That evidence includes
// prunable custom worktrees, which List intentionally omits because their
// directories cannot provide a binding file.
func ReconcileWithLocalEvidence(
	worktrees []Meta,
	issues map[string]*materialize.Issue,
	now time.Time,
	managedRoots []string,
	registeredPaths []string,
) ReconcileResult {
	result := ReconcileResult{
		BoundWorktrees: []string{},
		Orphans:        []string{},
		Ghosts:         []string{},
		GCRemovalSet:   []string{},
		GCRemovals:     []Meta{},
		GCAmbiguous:    []string{},
		Unrecognized:   []string{},
	}

	matchedRecordedPaths := make(recordedPathSet)
	gcCandidates := make(map[string][]Meta)

	for _, wt := range worktrees {
		issueID := wt.Binding
		issue := issues[issueID]
		if issueID == "" || issue == nil {
			result.Unrecognized = append(result.Unrecognized, wt.Path)
			continue
		}
		if issue.WorktreePath != "" && NormalizePathAllowingMissing(wt.Path) == NormalizePathAllowingMissing(issue.WorktreePath) {
			matchedRecordedPaths.add(issueID)
		}

		switch {
		case isTerminalStatus(issue.Status):
			gcCandidates[issueID] = append(gcCandidates[issueID], wt)
		case issue.ClaimedBy != "" && !issue.ClaimStale(now.Unix()):
			if liveClaimBindsLocalPath(issue, wt.Path) {
				result.BoundWorktrees = append(result.BoundWorktrees, issueID)
			} else {
				result.Orphans = append(result.Orphans, issueID)
			}
		default:
			result.Orphans = append(result.Orphans, issueID)
		}
	}

	for issueID, candidates := range gcCandidates {
		issue := issues[issueID]
		selected, res := selectGCRemoval(issue, candidates)
		if res != Bound {
			result.GCAmbiguous = append(result.GCAmbiguous, issueID)
			continue
		}
		result.GCRemovalSet = append(result.GCRemovalSet, issueID)
		result.GCRemovals = append(result.GCRemovals, selected)
	}

	ghostScopeDisabled := len(managedRoots) == 0
	for _, issue := range issues {
		if issue == nil || issue.WorktreePath == "" {
			continue
		}
		if matchedRecordedPaths.has(issue.ID) {
			continue
		}
		normPath := NormalizePathAllowingMissing(issue.WorktreePath)
		if !isTerminalStatus(issue.Status) && issue.ClaimedBy != "" &&
			!issue.ClaimStale(now.Unix()) &&
			(ghostScopeDisabled || isUnderManagedRoot(normPath, managedRoots) || isRegisteredPath(normPath, registeredPaths)) {
			result.Ghosts = append(result.Ghosts, issue.ID)
		}
	}

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

func liveClaimBindsLocalPath(issue *materialize.Issue, wtPath string) bool {
	return issue.WorktreePath == "" || NormalizePathAllowingMissing(wtPath) == NormalizePathAllowingMissing(issue.WorktreePath)
}

type recordedPathSet map[string]struct{}

func (s recordedPathSet) add(id string) { s[id] = struct{}{} }

func (s recordedPathSet) has(id string) bool {
	_, ok := s[id]
	return ok
}

func isRegisteredPath(normPath string, registeredPaths []string) bool {
	for _, path := range registeredPaths {
		if NormalizePathAllowingMissing(path) == normPath {
			return true
		}
	}
	return false
}

func selectGCRemoval(issue *materialize.Issue, candidates []Meta) (Meta, Resolution) {
	if len(candidates) == 0 {
		return Meta{}, NotFound
	}
	if issue != nil && issue.WorktreePath != "" {
		var matches []Meta
		for _, candidate := range candidates {
			if NormalizePathAllowingMissing(candidate.Path) == NormalizePathAllowingMissing(issue.WorktreePath) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 {
			return matches[0], Bound
		}
		if len(matches) > 1 {
			return Meta{}, Ambiguous
		}
	}
	if len(candidates) == 1 {
		return candidates[0], Bound
	}
	return Meta{}, Ambiguous
}

func isUnderManagedRoot(normPath string, managedRoots []string) bool {
	for _, root := range managedRoots {
		if root != "" && IsUnderRoot(normPath, root) {
			return true
		}
	}
	return false
}

func isTerminalStatus(status string) bool {
	return status == ops.StatusMerged || status == ops.StatusCancelled
}
