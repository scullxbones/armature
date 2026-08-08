package worktree

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestReconcile_EmptyInputs(t *testing.T) {
	t.Parallel()
	result := Reconcile([]WorktreeMeta{}, map[string]*materialize.Issue{})

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.GCRemovalSet)
}

func TestReconcile_BoundWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Worktree exists, issue has claim and worktree_path
	worktrees := []WorktreeMeta{
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
	worktrees := []WorktreeMeta{
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
	worktrees := []WorktreeMeta{}
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
	worktrees := []WorktreeMeta{
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
	worktrees := []WorktreeMeta{
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
	worktrees := []WorktreeMeta{
		{Path: "/repo/.worktrees/task-06", Branch: "task/task-06"},
	}
	issues := map[string]*materialize.Issue{
		"task-06": {
			ID:           "task-06",
			Status:       ops.StatusDone,
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
	worktrees := []WorktreeMeta{
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
	// Worktree exists but there's no corresponding issue at all (truly orphaned)
	worktrees := []WorktreeMeta{
		{Path: "/repo/.worktrees/unknown", Branch: "task/unknown"},
	}
	issues := map[string]*materialize.Issue{}

	result := Reconcile(worktrees, issues)

	// The worktree path doesn't match any pattern we can extract, but it should still be classified
	// For safety, we should handle this gracefully
	assert.Empty(t, result.BoundWorktrees)
	assert.Len(t, result.Orphans, 1)
}

func TestReconcile_GhostWithoutWorktree_NotInRemovalSet_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// Issue is merged but worktree doesn't exist: ghost should exist but NOT be in gc removal set
	// (can't remove a worktree that doesn't exist)
	worktrees := []WorktreeMeta{}
	issues := map[string]*materialize.Issue{
		"task-07": {
			ID:           "task-07",
			Status:       ops.StatusMerged,
			WorktreePath: "/repo/.worktrees/task-07",
		},
	}

	result := Reconcile(worktrees, issues)

	assert.Len(t, result.Ghosts, 1)
	assert.Contains(t, result.Ghosts, "task-07")
	assert.Empty(t, result.GCRemovalSet)
}
