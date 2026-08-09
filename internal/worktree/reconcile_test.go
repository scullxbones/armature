package worktree

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testNow is a fixed reference clock for reconcile tests. Cases that don't set a
// ClaimTTL are never stale (IsClaimStale returns false for ttl<=0), so this value
// only matters for the staleness-specific tests below, which set it explicitly.
var testNow = time.Unix(1_000_000, 0)

func TestReconcile_EmptyInputs(t *testing.T) {
	t.Parallel()
	result := Reconcile([]Meta{}, map[string]*materialize.Issue{}, testNow)

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.GCRemovalSet)
}

func TestReconcile_BoundWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Worktree exists, issue has claim and worktree_path
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-01", Branch: "task/task-01", Binding: "task-01"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:           "task-01",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-01",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Len(t, result.BoundWorktrees, 1)
	assert.Contains(t, result.BoundWorktrees, "task-01")
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
}

func TestReconcile_ForeignLiveClaimIsOrphan_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{{Path: "/local/.worktrees/task-foreign", Branch: "refs/heads/task/task-foreign", Binding: "task-foreign"}}
	issues := map[string]*materialize.Issue{
		"task-foreign": {
			ID:           "task-foreign",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-remote",
			WorktreePath: "/remote/.worktrees/task-foreign",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Empty(t, result.BoundWorktrees)
	assert.Equal(t, []string{"task-foreign"}, result.Orphans)
}

func TestReconcile_Orphan_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Worktree exists but issue has no claim (or issue doesn't exist)
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-02", Branch: "task/task-02", Binding: "task-02"},
	}
	issues := map[string]*materialize.Issue{
		"task-02": {
			ID:           "task-02",
			Status:       ops.StatusOpen,
			ClaimedBy:    "",
			WorktreePath: "",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Empty(t, result.BoundWorktrees)
	assert.Len(t, result.Orphans, 1)
	assert.Contains(t, result.Orphans, "task-02")
	assert.Empty(t, result.Ghosts)
}

func TestReconcile_Ghost_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue has worktree_path recorded but worktree doesn't exist on disk
	worktrees := []Meta{}
	issues := map[string]*materialize.Issue{
		"task-03": {
			ID:           "task-03",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-03",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Len(t, result.Ghosts, 1)
	assert.Contains(t, result.Ghosts, "task-03")
}

func TestReconcile_GCRemovalMerged_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue is merged with an existing worktree: should be in GCRemovalSet
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-04", Branch: "task/task-04", Binding: "task-04"},
	}
	issues := map[string]*materialize.Issue{
		"task-04": {
			ID:           "task-04",
			Status:       ops.StatusMerged,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-04",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-04")
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
}

func TestReconcile_GCRemovalCancelled_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue is cancelled with an existing worktree: should be in GCRemovalSet
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-05", Branch: "task/task-05", Binding: "task-05"},
	}
	issues := map[string]*materialize.Issue{
		"task-05": {
			ID:           "task-05",
			Status:       ops.StatusCancelled,
			WorktreePath: "/repo/.worktrees/task-05",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-05")
}

func TestReconcile_NoGCRemovalDone_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue is done (not merged) with an existing worktree: should NOT be in GCRemovalSet
	// (done means it hasn't been confirmed as merged yet)
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-06", Branch: "task/task-06", Binding: "task-06"},
	}
	issues := map[string]*materialize.Issue{
		"task-06": {
			ID:           "task-06",
			Status:       ops.StatusDone,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-06",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Empty(t, result.GCRemovalSet)
	assert.Len(t, result.BoundWorktrees, 1)
}

func TestReconcile_MixedScenario_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Complex scenario with multiple types
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-01", Branch: "task/task-01", Binding: "task-01"}, // bound
		{Path: "/repo/.worktrees/task-02", Branch: "task/task-02", Binding: "task-02"}, // orphan
		// task-03 is a ghost (recorded but not on disk)
		{Path: "/repo/.worktrees/task-04", Branch: "task/task-04", Binding: "task-04"}, // gc removal
	}
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:           "task-01",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-01",
		},
		"task-02": {
			ID:           "task-02",
			Status:       ops.StatusOpen,
			ClaimedBy:    "",
			WorktreePath: "",
		},
		"task-03": {
			ID:           "task-03",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-03", // doesn't exist on disk
		},
		"task-04": {
			ID:           "task-04",
			Status:       ops.StatusMerged,
			WorktreePath: "/repo/.worktrees/task-04",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Len(t, result.BoundWorktrees, 1)
	assert.Contains(t, result.BoundWorktrees, "task-01")
	assert.Len(t, result.Orphans, 1)
	assert.Contains(t, result.Orphans, "task-02")
	assert.Len(t, result.Ghosts, 1)
	assert.Contains(t, result.Ghosts, "task-03")
	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-04")
}

// TestReconcile_UnboundCanonicalWorktreeIsUnrecognized_REQ_LNGHZN_S5_T6 is the
// regression guard for the basename-inference defect. A worktree sitting at the
// canonical .worktrees/<issue-id> path, for an issue holding a LIVE claim, but
// carrying no armature-issue-id binding, must reconcile as Unrecognized — never
// BOUND. Reporting BOUND here would tell an agent the worktree is healthy while
// doctor and the delivery gate, which both require the binding, reject it.
func TestReconcile_UnboundCanonicalWorktreeIsUnrecognized_REQ_LNGHZN_S5_T6(t *testing.T) {
	t.Parallel()

	// Path basename matches the issue ID exactly; only the binding is missing.
	worktrees := []Meta{{Path: "/repo/.worktrees/task-unbound", Branch: "task/task-unbound"}}
	issues := map[string]*materialize.Issue{
		"task-unbound": {
			ID:           "task-unbound",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-unbound",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Equal(t, []string{"/repo/.worktrees/task-unbound"}, result.Unrecognized)
	assert.Empty(t, result.BoundWorktrees, "an unbound worktree must never be reported as bound")
	assert.Empty(t, result.Orphans)
}

func TestReconcile_WorktreeWithoutIssue_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Worktree exists but there's no corresponding issue at all: it must NOT be
	// reported as an orphan under an invented issue ID. It belongs in the
	// distinct Unrecognized bucket, reported by PATH.
	worktrees := []Meta{
		{Path: "/repo/.worktrees/unknown", Branch: "task/unknown"},
	}
	issues := map[string]*materialize.Issue{}

	result := Reconcile(worktrees, issues, testNow)

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Equal(t, []string{"/repo/.worktrees/unknown"}, result.Unrecognized)
}

func TestReconcile_MergedWithoutWorktree_NotGhostNotRemoval_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue is merged and its worktree is already gone: this is the EXPECTED end
	// state after gc/merge teardown, not a ghost and not a gc-removal candidate.
	worktrees := []Meta{}
	issues := map[string]*materialize.Issue{
		"task-07": {
			ID:           "task-07",
			Status:       ops.StatusMerged,
			WorktreePath: "/repo/.worktrees/task-07",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.GCRemovalSet)
}

// TestReconcile_NonLiveClaimMissingWorktree_NotGhost_REQ_LNGHZN_S5_T2 verifies a
// recorded worktree_path with no live claim (ClaimedBy empty) whose worktree is
// missing is not a ghost — ghost means a LIVE claim lost its worktree.
func TestReconcile_NonLiveClaimMissingWorktree_NotGhost_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{}
	issues := map[string]*materialize.Issue{
		"task-08": {
			ID:           "task-08",
			Status:       ops.StatusOpen,
			ClaimedBy:    "",
			WorktreePath: "/repo/.worktrees/task-08",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Empty(t, result.Ghosts)
}

// TestReconcile_SortedOutput_REQ_LNGHZN_S5_T2 asserts the result slices are
// sorted deterministically regardless of map iteration order.
func TestReconcile_SortedOutput_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-c", Branch: "task/task-c", Binding: "task-c"},
		{Path: "/repo/.worktrees/task-a", Branch: "task/task-a", Binding: "task-a"},
		{Path: "/repo/.worktrees/task-b", Branch: "task/task-b", Binding: "task-b"},
	}
	issues := map[string]*materialize.Issue{
		"task-a": {ID: "task-a", Status: ops.StatusInProgress, ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-a"},
		"task-b": {ID: "task-b", Status: ops.StatusInProgress, ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-b"},
		"task-c": {ID: "task-c", Status: ops.StatusInProgress, ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-c"},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Equal(t, []string{"task-a", "task-b", "task-c"}, result.BoundWorktrees)
}

// TestReconcile_SymlinkNormalization_REQ_LNGHZN_S5_T2 verifies that a worktree
// recorded via a symlinked path still matches the resolved path git reports.
func TestReconcile_SymlinkNormalization_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	realDir := t.TempDir()
	realWorktree := filepath.Join(realDir, "task-09")
	require.NoError(t, os.MkdirAll(realWorktree, 0o755))

	linkDir := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realDir, linkDir))
	symWorktree := filepath.Join(linkDir, "task-09")

	// git reports the resolved path; the issue recorded the symlinked path.
	worktrees := []Meta{{Path: realWorktree, Branch: "task/task-09", Binding: "task-09"}}
	issues := map[string]*materialize.Issue{
		"task-09": {ID: "task-09", Status: ops.StatusInProgress, ClaimedBy: "w", WorktreePath: symWorktree},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Equal(t, []string{"task-09"}, result.BoundWorktrees)
	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.Unrecognized)
}

func TestReconcile_UnclaimedWorktreeIsOrphan_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue references an existing worktree but holds no live claim (ClaimedBy empty):
	// per the contract this is an ORPHAN (worktree with no live claim), not bound.
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-08", Branch: "task/task-08", Binding: "task-08"},
	}
	issues := map[string]*materialize.Issue{
		"task-08": {
			ID:           "task-08",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "",
			WorktreePath: "/repo/.worktrees/task-08",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Empty(t, result.BoundWorktrees)
	assert.Len(t, result.Orphans, 1)
	assert.Contains(t, result.Orphans, "task-08")
	assert.Empty(t, result.Ghosts)
}

// TestWorktreeListFlagsOrphans_REQ_LNGHZN_S5_T2 is the contract-named acceptance test:
// worktree list must flag orphans (worktree on disk with no live claim).
func TestWorktreeListFlagsOrphans_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-09", Branch: "task/task-09", Binding: "task-09"}, // bound (claimed)
		{Path: "/repo/.worktrees/task-10", Branch: "task/task-10", Binding: "task-10"}, // orphan (unclaimed)
	}
	issues := map[string]*materialize.Issue{
		"task-09": {ID: "task-09", Status: ops.StatusInProgress, ClaimedBy: "worker-1", WorktreePath: "/repo/.worktrees/task-09"},
		"task-10": {ID: "task-10", Status: ops.StatusInProgress, ClaimedBy: "", WorktreePath: "/repo/.worktrees/task-10"},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Contains(t, result.BoundWorktrees, "task-09")
	assert.Contains(t, result.Orphans, "task-10")
}

// TestReconcile_RemoteClaimNotGhost_REQ_LNGHZN_S5_T3 verifies that a live claim
// owned by a REMOTE clone (its recorded WorktreePath is under a different clone's
// absolute .worktrees root, and so never appears in this clone's git worktree
// list) is NOT classified as a ghost when ghost detection is scoped to this
// clone's managed root.
func TestReconcile_RemoteClaimNotGhost_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	localRoot := "/local/clone/.worktrees/"
	worktrees := []Meta{} // nothing on disk locally
	issues := map[string]*materialize.Issue{
		"task-remote": {
			ID:           "task-remote",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-remote",
			WorktreePath: "/other/clone/.worktrees/task-remote",
		},
	}

	result := Reconcile(worktrees, issues, testNow, localRoot)

	assert.Empty(t, result.Ghosts, "a remote clone's live claim must not be a local ghost")
}

// TestReconcile_SymlinkedRootLocalGhost_REQ_LNGHZN_S5_T3 pins the symlink-
// asymmetric ghost-scoping fix: a genuine LOCAL ghost whose recorded path is
// reached through a symlinked repo root must still be detected. The recorded
// WorktreePath is stored symlink-unresolved (filepath.Abs) and its leaf worktree
// is missing on disk, so a naive EvalSymlinks fails and leaves the path symlinky,
// while the managed root (its parent .worktrees/ exists) resolves to the real
// path — defeating the HasPrefix scope test and silently dropping the ghost.
// Unlike the fake-absolute-path ghost tests, this exercises a REAL symlink so the
// fix is genuinely pinned rather than tautological.
func TestReconcile_SymlinkedRootLocalGhost_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	realRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(realRoot, ".worktrees"), 0o755))

	linkRoot := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realRoot, linkRoot))

	// Managed root computed from the symlinked path, exactly as production does:
	// its existing .worktrees/ dir resolves through EvalSymlinks to the real path.
	managedRoot := NormalizePath(filepath.Join(linkRoot, ".worktrees")) + string(os.PathSeparator)

	// Recorded path goes through the symlink; the leaf worktree is missing on disk
	// (the ghost condition), so the full path cannot be symlink-resolved directly.
	recorded := filepath.Join(linkRoot, ".worktrees", "task-sym")

	worktrees := []Meta{}
	issues := map[string]*materialize.Issue{
		"task-sym": {
			ID:           "task-sym",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			WorktreePath: recorded,
		},
	}

	result := Reconcile(worktrees, issues, testNow, managedRoot)

	assert.Equal(t, []string{"task-sym"}, result.Ghosts,
		"a genuine local ghost reached through a symlinked repo root must not be dropped")
}

// TestReconcile_LocalClaimStillGhost_REQ_LNGHZN_S5_T3 verifies that a live claim
// owned by THIS clone (recorded path under the local managed root) whose worktree
// is missing on disk is still classified as a ghost even with scoping enabled.
func TestReconcile_LocalClaimStillGhost_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	localRoot := "/local/clone/.worktrees/"
	worktrees := []Meta{} // missing on disk
	issues := map[string]*materialize.Issue{
		"task-local": {
			ID:           "task-local",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-local",
			WorktreePath: "/local/clone/.worktrees/task-local",
		},
	}

	result := Reconcile(worktrees, issues, testNow, localRoot)

	assert.Equal(t, []string{"task-local"}, result.Ghosts)
}

// TestReconcile_TerminalForeignPathLocalWorktree_IsGCRemoval_REQ_LNGHZN_S5_T2
// (thread 2) covers a merged issue whose git-replicated WorktreePath names a
// FOREIGN clone, while a real worktree for it exists on THIS clone's branch.
// Classification must key on the local worktree (via its path) and land the
// issue in GCRemovalSet — not Orphans — so gc, which removes by branch in this
// clone, agrees with selection. Pre-fix, the foreign path failed to match and
// the local worktree fell through to Orphans.
func TestReconcile_TerminalForeignPathLocalWorktree_IsGCRemoval_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/local/clone/.worktrees/task-foreign", Branch: "task/task-foreign", Binding: "task-foreign"},
	}
	issues := map[string]*materialize.Issue{
		"task-foreign": {
			ID:     "task-foreign",
			Status: ops.StatusMerged,
			// Recorded path points at a DIFFERENT clone (git-replicated absolute path).
			WorktreePath: "/other/clone/.worktrees/task-foreign",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Contains(t, result.GCRemovalSet, "task-foreign",
		"a terminal issue with a local worktree must be gc-ready regardless of the foreign recorded path")
	assert.NotContains(t, result.Orphans, "task-foreign",
		"a terminal local worktree must not be misclassified as an orphan")
}

// TestReconcile_StaleClaimIsOrphanNotBound_REQ_LNGHZN_S5_T2 (thread 4) covers a
// claimed issue whose claim is past its TTL: it holds ClaimedBy but is no longer
// live, so its worktree is an Orphan, not Bound. Pre-fix, any non-empty ClaimedBy
// was treated as healthy with no TTL check.
func TestReconcile_StaleClaimIsOrphanNotBound_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-stale", Branch: "task/task-stale", Binding: "task-stale"},
	}
	// Claimed at t=100 with a 1-minute TTL; now is well past 100+60=160.
	now := time.Unix(100_000, 0)
	issues := map[string]*materialize.Issue{
		"task-stale": {
			ID:           "task-stale",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			ClaimedAt:    100,
			ClaimTTL:     1,
			WorktreePath: "/repo/.worktrees/task-stale",
		},
	}

	result := Reconcile(worktrees, issues, now)

	assert.Contains(t, result.Orphans, "task-stale", "a claim past its TTL is an orphan")
	assert.NotContains(t, result.BoundWorktrees, "task-stale", "a stale claim must not be bound")
}

// TestReconcile_FreshClaimStillBound_REQ_LNGHZN_S5_T2 is the companion to the
// stale case: a claim within its TTL stays Bound.
func TestReconcile_FreshClaimStillBound_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-fresh", Branch: "task/task-fresh", Binding: "task-fresh"},
	}
	// Claimed at t=100 with a 60-minute TTL; now is well within 100+3600.
	now := time.Unix(200, 0)
	issues := map[string]*materialize.Issue{
		"task-fresh": {
			ID:           "task-fresh",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			ClaimedAt:    100,
			ClaimTTL:     60,
			WorktreePath: "/repo/.worktrees/task-fresh",
		},
	}

	result := Reconcile(worktrees, issues, now)

	assert.Contains(t, result.BoundWorktrees, "task-fresh")
	assert.NotContains(t, result.Orphans, "task-fresh")
}

// TestReconcile_BindingKeysIdentityOverBasename_REQ_LNGHZN_S5_T2 verifies that a
// worktree's authoritative armature-issue-id binding (Meta.Binding) — not the path
// basename — drives classification. The issue ID contains a slash, so the path
// basename would truncate it to the last segment and fail to match any issue,
// misreporting the worktree as Unrecognized. Keying on the binding classifies it
// correctly as bound.
func TestReconcile_BindingKeysIdentityOverBasename_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		// git flattens slash IDs into a single directory, so the on-disk basename
		// ("T2") differs from the real issue ID ("LNGHZN-S5/T2").
		{Path: "/repo/.worktrees/T2", Branch: "task/LNGHZN-S5/T2", Binding: "LNGHZN-S5/T2"},
	}
	issues := map[string]*materialize.Issue{
		"LNGHZN-S5/T2": {
			ID:           "LNGHZN-S5/T2",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/T2",
		},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Equal(t, []string{"LNGHZN-S5/T2"}, result.BoundWorktrees,
		"the armature-issue-id binding must key identity, not the truncated basename")
	assert.Empty(t, result.Unrecognized,
		"a bound worktree must not be misreported as unrecognized due to a truncated basename")
}

// TestReconcile_StaleClaimMissingWorktree_NotGhost_REQ_LNGHZN_S5_T2 verifies that
// an issue whose claim is past its TTL and whose worktree is missing on disk is NOT
// a ghost: a ghost is a LIVE claim that lost its worktree, and a stale claim is no
// longer live. Pre-fix the ghost pass omitted the staleness check (finding #2).
func TestReconcile_StaleClaimMissingWorktree_NotGhost_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	localRoot := "/local/clone/.worktrees/"
	worktrees := []Meta{} // missing on disk
	// Claimed at t=100 with a 1-minute TTL; now is well past 100+60.
	now := time.Unix(100_000, 0)
	issues := map[string]*materialize.Issue{
		"task-stale": {
			ID:           "task-stale",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			ClaimedAt:    100,
			ClaimTTL:     1,
			WorktreePath: "/local/clone/.worktrees/task-stale",
		},
	}

	result := Reconcile(worktrees, issues, now, localRoot)

	assert.Empty(t, result.Ghosts, "a stale (no-longer-live) claim with a missing worktree is not a ghost")
}

// TestWorktreeGCRemovesMergedWorktrees_REQ_LNGHZN_S5_T2 is the contract-named acceptance test:
// gc's removal set must contain issues in merged status with an existing worktree.
func TestWorktreeGCRemovesMergedWorktrees_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-11", Branch: "task/task-11", Binding: "task-11"},
	}
	issues := map[string]*materialize.Issue{
		"task-11": {ID: "task-11", Status: ops.StatusMerged, ClaimedBy: "worker-1", WorktreePath: "/repo/.worktrees/task-11"},
	}

	result := Reconcile(worktrees, issues, testNow)

	assert.Contains(t, result.GCRemovalSet, "task-11")
	assert.NotContains(t, result.BoundWorktrees, "task-11")
	assert.NotContains(t, result.Orphans, "task-11")
}

func TestReconcile_GCSelectsRecordedPathAmongDuplicateMarkers_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	legacyPath := "/tmp/legacy-task-12"
	canonicalPath := "/repo/.worktrees/task-12"
	result := Reconcile([]Meta{
		{Path: legacyPath, Binding: "task-12"},
		{Path: canonicalPath, Binding: "task-12"},
	}, map[string]*materialize.Issue{
		"task-12": {ID: "task-12", Status: ops.StatusMerged, WorktreePath: canonicalPath},
	}, testNow)

	assert.Equal(t, []string{"task-12"}, result.GCRemovalSet)
	require.Len(t, result.GCRemovals, 1)
	assert.Equal(t, canonicalPath, result.GCRemovals[0].Path,
		"GC must carry the exact recorded canonical path instead of looking up the first binding")
}

func TestReconcile_GCAmbiguousDuplicateMarkersRemovesNothing_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	result := Reconcile([]Meta{
		{Path: "/repo/.worktrees/task-13-a", Binding: "task-13"},
		{Path: "/repo/.worktrees/task-13-b", Binding: "task-13"},
	}, map[string]*materialize.Issue{
		"task-13": {ID: "task-13", Status: ops.StatusCancelled, WorktreePath: "/other/clone/task-13"},
	}, testNow)

	assert.Empty(t, result.GCRemovalSet)
	assert.Empty(t, result.GCRemovals)
	assert.Equal(t, []string{"task-13"}, result.GCAmbiguous)
}

func TestReconcile_WrongPathMarkerStillLeavesRecordedPathGhost_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	recordedPath := "/repo/.worktrees/task-14"
	wrongPath := "/repo/.worktrees/legacy-task-14"
	result := Reconcile([]Meta{{Path: wrongPath, Binding: "task-14"}}, map[string]*materialize.Issue{
		"task-14": {ID: "task-14", Status: ops.StatusInProgress, ClaimedBy: "worker-1", WorktreePath: recordedPath},
	}, testNow)

	assert.Equal(t, []string{"task-14"}, result.Orphans,
		"a binding at the wrong path is not the live recorded worktree")
	assert.Equal(t, []string{"task-14"}, result.Ghosts,
		"a wrong-path binding must not hide the missing recorded path")
}
