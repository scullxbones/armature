package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcile_EmptyInputs(t *testing.T) {
	t.Parallel()
	result := Reconcile([]Meta{}, map[string]*materialize.Issue{})

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.GCRemovalSet)
}

func TestReconcile_BoundWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Worktree exists, issue has claim and worktree_path
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-01", Branch: "task/task-01"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:           "task-01",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-01",
		},
	}

	result := Reconcile(worktrees, issues)

	assert.Len(t, result.BoundWorktrees, 1)
	assert.Contains(t, result.BoundWorktrees, "task-01")
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
}

func TestReconcile_Orphan_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Worktree exists but issue has no claim (or issue doesn't exist)
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-02", Branch: "task/task-02"},
	}
	issues := map[string]*materialize.Issue{
		"task-02": {
			ID:           "task-02",
			Status:       ops.StatusOpen,
			ClaimedBy:    "",
			WorktreePath: "",
		},
	}

	result := Reconcile(worktrees, issues)

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

	result := Reconcile(worktrees, issues)

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Len(t, result.Ghosts, 1)
	assert.Contains(t, result.Ghosts, "task-03")
}

func TestReconcile_GCRemovalMerged_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue is merged with an existing worktree: should be in GCRemovalSet
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-04", Branch: "task/task-04"},
	}
	issues := map[string]*materialize.Issue{
		"task-04": {
			ID:           "task-04",
			Status:       ops.StatusMerged,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-04",
		},
	}

	result := Reconcile(worktrees, issues)

	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-04")
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
}

func TestReconcile_GCRemovalCancelled_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue is cancelled with an existing worktree: should be in GCRemovalSet
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-05", Branch: "task/task-05"},
	}
	issues := map[string]*materialize.Issue{
		"task-05": {
			ID:           "task-05",
			Status:       ops.StatusCancelled,
			WorktreePath: "/repo/.worktrees/task-05",
		},
	}

	result := Reconcile(worktrees, issues)

	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-05")
}

func TestReconcile_NoGCRemovalDone_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue is done (not merged) with an existing worktree: should NOT be in GCRemovalSet
	// (done means it hasn't been confirmed as merged yet)
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-06", Branch: "task/task-06"},
	}
	issues := map[string]*materialize.Issue{
		"task-06": {
			ID:           "task-06",
			Status:       ops.StatusDone,
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-06",
		},
	}

	result := Reconcile(worktrees, issues)

	assert.Empty(t, result.GCRemovalSet)
	assert.Len(t, result.BoundWorktrees, 1)
}

func TestReconcile_MixedScenario_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Complex scenario with multiple types
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-01", Branch: "task/task-01"}, // bound
		{Path: "/repo/.worktrees/task-02", Branch: "task/task-02"}, // orphan
		// task-03 is a ghost (recorded but not on disk)
		{Path: "/repo/.worktrees/task-04", Branch: "task/task-04"}, // gc removal
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

	result := Reconcile(worktrees, issues)

	assert.Len(t, result.BoundWorktrees, 1)
	assert.Contains(t, result.BoundWorktrees, "task-01")
	assert.Len(t, result.Orphans, 1)
	assert.Contains(t, result.Orphans, "task-02")
	assert.Len(t, result.Ghosts, 1)
	assert.Contains(t, result.Ghosts, "task-03")
	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-04")
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

	result := Reconcile(worktrees, issues)

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

	result := Reconcile(worktrees, issues)

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

	result := Reconcile(worktrees, issues)

	assert.Empty(t, result.Ghosts)
}

// TestReconcile_SortedOutput_REQ_LNGHZN_S5_T2 asserts the result slices are
// sorted deterministically regardless of map iteration order.
func TestReconcile_SortedOutput_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-c", Branch: "task/task-c"},
		{Path: "/repo/.worktrees/task-a", Branch: "task/task-a"},
		{Path: "/repo/.worktrees/task-b", Branch: "task/task-b"},
	}
	issues := map[string]*materialize.Issue{
		"task-a": {ID: "task-a", Status: ops.StatusInProgress, ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-a"},
		"task-b": {ID: "task-b", Status: ops.StatusInProgress, ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-b"},
		"task-c": {ID: "task-c", Status: ops.StatusInProgress, ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-c"},
	}

	result := Reconcile(worktrees, issues)

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
	worktrees := []Meta{{Path: realWorktree, Branch: "task/task-09"}}
	issues := map[string]*materialize.Issue{
		"task-09": {ID: "task-09", Status: ops.StatusInProgress, ClaimedBy: "w", WorktreePath: symWorktree},
	}

	result := Reconcile(worktrees, issues)

	assert.Equal(t, []string{"task-09"}, result.BoundWorktrees)
	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.Unrecognized)
}

func TestReconcile_UnclaimedWorktreeIsOrphan_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue references an existing worktree but holds no live claim (ClaimedBy empty):
	// per the contract this is an ORPHAN (worktree with no live claim), not bound.
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-08", Branch: "task/task-08"},
	}
	issues := map[string]*materialize.Issue{
		"task-08": {
			ID:           "task-08",
			Status:       ops.StatusInProgress,
			ClaimedBy:    "",
			WorktreePath: "/repo/.worktrees/task-08",
		},
	}

	result := Reconcile(worktrees, issues)

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
		{Path: "/repo/.worktrees/task-09", Branch: "task/task-09"}, // bound (claimed)
		{Path: "/repo/.worktrees/task-10", Branch: "task/task-10"}, // orphan (unclaimed)
	}
	issues := map[string]*materialize.Issue{
		"task-09": {ID: "task-09", Status: ops.StatusInProgress, ClaimedBy: "worker-1", WorktreePath: "/repo/.worktrees/task-09"},
		"task-10": {ID: "task-10", Status: ops.StatusInProgress, ClaimedBy: "", WorktreePath: "/repo/.worktrees/task-10"},
	}

	result := Reconcile(worktrees, issues)

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

	result := Reconcile(worktrees, issues, localRoot)

	assert.Empty(t, result.Ghosts, "a remote clone's live claim must not be a local ghost")
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

	result := Reconcile(worktrees, issues, localRoot)

	assert.Equal(t, []string{"task-local"}, result.Ghosts)
}

// TestWorktreeGCRemovesMergedWorktrees_REQ_LNGHZN_S5_T2 is the contract-named acceptance test:
// gc's removal set must contain issues in merged status with an existing worktree.
func TestWorktreeGCRemovesMergedWorktrees_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-11", Branch: "task/task-11"},
	}
	issues := map[string]*materialize.Issue{
		"task-11": {ID: "task-11", Status: ops.StatusMerged, ClaimedBy: "worker-1", WorktreePath: "/repo/.worktrees/task-11"},
	}

	result := Reconcile(worktrees, issues)

	assert.Contains(t, result.GCRemovalSet, "task-11")
	assert.NotContains(t, result.BoundWorktrees, "task-11")
	assert.NotContains(t, result.Orphans, "task-11")
}
