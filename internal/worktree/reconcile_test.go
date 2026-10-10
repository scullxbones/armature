package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testReconcile(worktrees []Meta, issues map[string]IssueFacts, managedRoots ...string) ReconcileResult {
	return Reconcile(worktrees, issues, managedRoots...)
}

func TestReconcile_EmptyInputs(t *testing.T) {
	t.Parallel()
	result := testReconcile([]Meta{}, map[string]IssueFacts{})

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.GCRemovalSet)
}

func TestReconcile_BoundWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-01", Branch: "task/task-01", Binding: "task-01"},
	}
	issues := map[string]IssueFacts{
		"task-01": {
			ID:           "task-01",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-01",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Len(t, result.BoundWorktrees, 1)
	assert.Contains(t, result.BoundWorktrees, "task-01")
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
}

func TestReconcile_ForeignLiveClaimIsOrphan_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{{Path: "/local/.worktrees/task-foreign", Branch: "refs/heads/task/task-foreign", Binding: "task-foreign"}}
	issues := map[string]IssueFacts{
		"task-foreign": {
			ID:           "task-foreign",
			Status:       "in-progress",
			ClaimedBy:    "worker-remote",
			WorktreePath: "/remote/.worktrees/task-foreign",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Empty(t, result.BoundWorktrees)
	assert.Equal(t, []string{"task-foreign"}, result.Orphans)
}

func TestReconcile_Orphan_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-02", Branch: "task/task-02", Binding: "task-02"},
	}
	issues := map[string]IssueFacts{
		"task-02": {
			ID:           "task-02",
			Status:       "open",
			ClaimedBy:    "",
			WorktreePath: "",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Empty(t, result.BoundWorktrees)
	assert.Len(t, result.Orphans, 1)
	assert.Contains(t, result.Orphans, "task-02")
	assert.Empty(t, result.Ghosts)
}

func TestReconcile_Ghost_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{}
	issues := map[string]IssueFacts{
		"task-03": {
			ID:           "task-03",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-03",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Len(t, result.Ghosts, 1)
	assert.Contains(t, result.Ghosts, "task-03")
}

func TestReconcile_GCRemovalMerged_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-04", Branch: "task/task-04", Binding: "task-04"},
	}
	issues := map[string]IssueFacts{
		"task-04": {
			ID:           "task-04",
			Status:       "merged",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-04",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-04")
	assert.Empty(t, result.Orphans)
	assert.Empty(t, result.Ghosts)
}

func TestReconcile_GCRemovalCancelled_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-05", Branch: "task/task-05", Binding: "task-05"},
	}
	issues := map[string]IssueFacts{
		"task-05": {
			ID:           "task-05",
			Status:       "cancelled",
			WorktreePath: "/repo/.worktrees/task-05",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-05")
}

func TestReconcile_NoGCRemovalDone_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-06", Branch: "task/task-06", Binding: "task-06"},
	}
	issues := map[string]IssueFacts{
		"task-06": {
			ID:           "task-06",
			Status:       "done",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-06",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Empty(t, result.GCRemovalSet)
	assert.Len(t, result.BoundWorktrees, 1)
}

func TestReconcile_MixedScenario_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-01", Branch: "task/task-01", Binding: "task-01"},
		{Path: "/repo/.worktrees/task-02", Branch: "task/task-02", Binding: "task-02"},
		{Path: "/repo/.worktrees/task-04", Branch: "task/task-04", Binding: "task-04"},
	}
	issues := map[string]IssueFacts{
		"task-01": {
			ID:           "task-01",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-01",
		},
		"task-02": {
			ID:           "task-02",
			Status:       "open",
			ClaimedBy:    "",
			WorktreePath: "",
		},
		"task-03": {
			ID:           "task-03",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-03",
		},
		"task-04": {
			ID:           "task-04",
			Status:       "merged",
			WorktreePath: "/repo/.worktrees/task-04",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Len(t, result.BoundWorktrees, 1)
	assert.Contains(t, result.BoundWorktrees, "task-01")
	assert.Len(t, result.Orphans, 1)
	assert.Contains(t, result.Orphans, "task-02")
	assert.Len(t, result.Ghosts, 1)
	assert.Contains(t, result.Ghosts, "task-03")
	assert.Len(t, result.GCRemovalSet, 1)
	assert.Contains(t, result.GCRemovalSet, "task-04")
}

func TestReconcile_UnboundCanonicalWorktreeIsUnrecognized_REQ_LNGHZN_S5_T6(t *testing.T) {
	t.Parallel()

	worktrees := []Meta{{Path: "/repo/.worktrees/task-unbound", Branch: "task/task-unbound"}}
	issues := map[string]IssueFacts{
		"task-unbound": {
			ID:           "task-unbound",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-unbound",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Equal(t, []string{"/repo/.worktrees/task-unbound"}, result.Unrecognized)
	assert.Empty(t, result.BoundWorktrees, "an unbound worktree must never be reported as bound")
	assert.Empty(t, result.Orphans)
}

func TestReconcile_WorktreeWithoutIssue_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/unknown", Branch: "task/unknown"},
	}
	issues := map[string]IssueFacts{}

	result := testReconcile(worktrees, issues)

	assert.Empty(t, result.BoundWorktrees)
	assert.Empty(t, result.Orphans)
	assert.Equal(t, []string{"/repo/.worktrees/unknown"}, result.Unrecognized)
}

func TestReconcile_MergedWithoutWorktree_NotGhostNotRemoval_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{}
	issues := map[string]IssueFacts{
		"task-07": {
			ID:           "task-07",
			Status:       "merged",
			WorktreePath: "/repo/.worktrees/task-07",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.GCRemovalSet)
}

func TestReconcile_NonLiveClaimMissingWorktree_NotGhost_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{}
	issues := map[string]IssueFacts{
		"task-08": {
			ID:           "task-08",
			Status:       "open",
			ClaimedBy:    "",
			WorktreePath: "/repo/.worktrees/task-08",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Empty(t, result.Ghosts)
}

func TestReconcile_SortedOutput_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-c", Branch: "task/task-c", Binding: "task-c"},
		{Path: "/repo/.worktrees/task-a", Branch: "task/task-a", Binding: "task-a"},
		{Path: "/repo/.worktrees/task-b", Branch: "task/task-b", Binding: "task-b"},
	}
	issues := map[string]IssueFacts{
		"task-a": {ID: "task-a", Status: "in-progress", ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-a"},
		"task-b": {ID: "task-b", Status: "in-progress", ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-b"},
		"task-c": {ID: "task-c", Status: "in-progress", ClaimedBy: "w", WorktreePath: "/repo/.worktrees/task-c"},
	}

	result := testReconcile(worktrees, issues)

	assert.Equal(t, []string{"task-a", "task-b", "task-c"}, result.BoundWorktrees)
}

func TestReconcile_SortsGCRemovals_REQ_LNGHZN_S5_T10(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-c", Branch: "task/task-c", Binding: "task-c"},
		{Path: "/repo/.worktrees/task-a", Branch: "task/task-a", Binding: "task-a"},
		{Path: "/repo/.worktrees/task-b", Branch: "task/task-b", Binding: "task-b"},
	}
	issues := map[string]IssueFacts{
		"task-a": {ID: "task-a", Status: "merged", WorktreePath: "/repo/.worktrees/task-a"},
		"task-b": {ID: "task-b", Status: "merged", WorktreePath: "/repo/.worktrees/task-b"},
		"task-c": {ID: "task-c", Status: "merged", WorktreePath: "/repo/.worktrees/task-c"},
	}

	result := testReconcile(worktrees, issues)

	assert.Equal(t, []string{"task-a", "task-b", "task-c"}, result.GCRemovalSet)
	require.Len(t, result.GCRemovals, 3)
	assert.Equal(t, []string{"task-a", "task-b", "task-c"}, []string{
		result.GCRemovals[0].Binding,
		result.GCRemovals[1].Binding,
		result.GCRemovals[2].Binding,
	})
}

func TestReconcile_SymlinkNormalization_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	realDir := t.TempDir()
	realWorktree := filepath.Join(realDir, "task-09")
	require.NoError(t, os.MkdirAll(realWorktree, 0o755))

	linkDir := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realDir, linkDir))
	symWorktree := filepath.Join(linkDir, "task-09")

	worktrees := []Meta{{Path: realWorktree, Branch: "task/task-09", Binding: "task-09"}}
	issues := map[string]IssueFacts{
		"task-09": {ID: "task-09", Status: "in-progress", ClaimedBy: "w", WorktreePath: symWorktree},
	}

	result := testReconcile(worktrees, issues)

	assert.Equal(t, []string{"task-09"}, result.BoundWorktrees)
	assert.Empty(t, result.Ghosts)
	assert.Empty(t, result.Unrecognized)
}

func TestReconcile_UnclaimedWorktreeIsOrphan_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-08", Branch: "task/task-08", Binding: "task-08"},
	}
	issues := map[string]IssueFacts{
		"task-08": {
			ID:           "task-08",
			Status:       "in-progress",
			ClaimedBy:    "",
			WorktreePath: "/repo/.worktrees/task-08",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Empty(t, result.BoundWorktrees)
	assert.Len(t, result.Orphans, 1)
	assert.Contains(t, result.Orphans, "task-08")
	assert.Empty(t, result.Ghosts)
}

func TestWorktreeListFlagsOrphans_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-09", Branch: "task/task-09", Binding: "task-09"},
		{Path: "/repo/.worktrees/task-10", Branch: "task/task-10", Binding: "task-10"},
	}
	issues := map[string]IssueFacts{
		"task-09": {ID: "task-09", Status: "in-progress", ClaimedBy: "worker-1", WorktreePath: "/repo/.worktrees/task-09"},
		"task-10": {ID: "task-10", Status: "in-progress", ClaimedBy: "", WorktreePath: "/repo/.worktrees/task-10"},
	}

	result := testReconcile(worktrees, issues)

	assert.Contains(t, result.BoundWorktrees, "task-09")
	assert.Contains(t, result.Orphans, "task-10")
}

func TestReconcile_RemoteClaimNotGhost_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	localRoot := "/local/clone/.worktrees/"
	worktrees := []Meta{}
	issues := map[string]IssueFacts{
		"task-remote": {
			ID:           "task-remote",
			Status:       "in-progress",
			ClaimedBy:    "worker-remote",
			WorktreePath: "/other/clone/.worktrees/task-remote",
		},
	}

	result := testReconcile(worktrees, issues, localRoot)

	assert.Empty(t, result.Ghosts, "a remote clone's live claim must not be a local ghost")
}

func TestReconcile_SymlinkedRootLocalGhost_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	realRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(realRoot, ".worktrees"), 0o755))

	linkRoot := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realRoot, linkRoot))

	managedRoot := NormalizePath(filepath.Join(linkRoot, ".worktrees")) + string(os.PathSeparator)

	recorded := filepath.Join(linkRoot, ".worktrees", "task-sym")

	worktrees := []Meta{}
	issues := map[string]IssueFacts{
		"task-sym": {
			ID:           "task-sym",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: recorded,
		},
	}

	result := testReconcile(worktrees, issues, managedRoot)

	assert.Equal(t, []string{"task-sym"}, result.Ghosts,
		"a genuine local ghost reached through a symlinked repo root must not be dropped")
}

func TestReconcile_LocalClaimStillGhost_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	localRoot := "/local/clone/.worktrees/"
	worktrees := []Meta{}
	issues := map[string]IssueFacts{
		"task-local": {
			ID:           "task-local",
			Status:       "in-progress",
			ClaimedBy:    "worker-local",
			WorktreePath: "/local/clone/.worktrees/task-local",
		},
	}

	result := testReconcile(worktrees, issues, localRoot)

	assert.Equal(t, []string{"task-local"}, result.Ghosts)
}

func TestReconcile_CustomInsideRepositoryGhostUsesLocalEvidence_REQ_LNGHZN_S9_T1(t *testing.T) {
	t.Parallel()
	issues := map[string]IssueFacts{
		"task-custom": {
			ID:           "task-custom",
			Status:       "in-progress",
			ClaimedBy:    "worker-local",
			WorktreePath: "/repo/story/task-custom",
		},
	}

	result := ReconcileWithLocalEvidence(nil, issues, []string{"/repo"}, nil)
	assert.Equal(t, []string{"task-custom"}, result.Ghosts,
		"a missing live claim at an explicit in-repository destination must be a local ghost")
}

func TestReconcile_CustomOutsideRepositoryGhostRequiresLocalRegistration_REQ_LNGHZN_S9_T1(t *testing.T) {
	t.Parallel()
	issues := map[string]IssueFacts{
		"task-custom": {
			ID:           "task-custom",
			Status:       "in-progress",
			ClaimedBy:    "worker-local",
			WorktreePath: "/tmp/armature-custom/task-custom",
		},
	}

	registered := []string{"/tmp/armature-custom/task-custom"}
	result := ReconcileWithLocalEvidence(nil, issues, []string{"/repo/.worktrees"}, registered)
	assert.Equal(t, []string{"task-custom"}, result.Ghosts,
		"a missing outside destination is local evidence when Git still has its prunable registration")

	foreign := ReconcileWithLocalEvidence(nil, issues, []string{"/repo/.worktrees"}, nil)
	assert.Empty(t, foreign.Ghosts,
		"a foreign absolute path with no local registration must not become a ghost")
}

func TestReconcile_TerminalForeignPathLocalWorktree_IsGCRemoval_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/local/clone/.worktrees/task-foreign", Branch: "task/task-foreign", Binding: "task-foreign"},
	}
	issues := map[string]IssueFacts{
		"task-foreign": {
			ID:           "task-foreign",
			Status:       "merged",
			WorktreePath: "/other/clone/.worktrees/task-foreign",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Contains(t, result.GCRemovalSet, "task-foreign",
		"a terminal issue with a local worktree must be gc-ready regardless of the foreign recorded path")
	assert.NotContains(t, result.Orphans, "task-foreign",
		"a terminal local worktree must not be misclassified as an orphan")
}

func TestReconcile_StaleClaimIsOrphanNotBound_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-stale", Branch: "task/task-stale", Binding: "task-stale"},
	}
	issues := map[string]IssueFacts{
		"task-stale": {
			ID:           "task-stale",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-stale",
			ClaimStale:   true,
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Contains(t, result.Orphans, "task-stale", "a claim past its TTL is an orphan")
	assert.NotContains(t, result.BoundWorktrees, "task-stale", "a stale claim must not be bound")
}

func TestReconcile_FreshClaimStillBound_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-fresh", Branch: "task/task-fresh", Binding: "task-fresh"},
	}
	issues := map[string]IssueFacts{
		"task-fresh": {
			ID:           "task-fresh",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/task-fresh",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Contains(t, result.BoundWorktrees, "task-fresh")
	assert.NotContains(t, result.Orphans, "task-fresh")
}

func TestReconcile_BindingKeysIdentityOverBasename_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/T2", Branch: "task/LNGHZN-S5/T2", Binding: "LNGHZN-S5/T2"},
	}
	issues := map[string]IssueFacts{
		"LNGHZN-S5/T2": {
			ID:           "LNGHZN-S5/T2",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/repo/.worktrees/T2",
		},
	}

	result := testReconcile(worktrees, issues)

	assert.Equal(t, []string{"LNGHZN-S5/T2"}, result.BoundWorktrees,
		"the armature-issue-id binding must key identity, not the truncated basename")
	assert.Empty(t, result.Unrecognized,
		"a bound worktree must not be misreported as unrecognized due to a truncated basename")
}

func TestReconcile_StaleClaimMissingWorktree_NotGhost_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	localRoot := "/local/clone/.worktrees/"
	worktrees := []Meta{}
	issues := map[string]IssueFacts{
		"task-stale": {
			ID:           "task-stale",
			Status:       "in-progress",
			ClaimedBy:    "worker-1",
			WorktreePath: "/local/clone/.worktrees/task-stale",
			ClaimStale:   true,
		},
	}

	result := testReconcile(worktrees, issues, localRoot)

	assert.Empty(t, result.Ghosts, "a stale (no-longer-live) claim with a missing worktree is not a ghost")
}

func TestWorktreeGCRemovesMergedWorktrees_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	worktrees := []Meta{
		{Path: "/repo/.worktrees/task-11", Branch: "task/task-11", Binding: "task-11"},
	}
	issues := map[string]IssueFacts{
		"task-11": {ID: "task-11", Status: "merged", ClaimedBy: "worker-1", WorktreePath: "/repo/.worktrees/task-11"},
	}

	result := testReconcile(worktrees, issues)

	assert.Contains(t, result.GCRemovalSet, "task-11")
	assert.NotContains(t, result.BoundWorktrees, "task-11")
	assert.NotContains(t, result.Orphans, "task-11")
}

func TestReconcile_GCSelectsRecordedPathAmongDuplicateMarkers_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	legacyPath := "/tmp/legacy-task-12"
	canonicalPath := "/repo/.worktrees/task-12"
	result := testReconcile([]Meta{
		{Path: legacyPath, Binding: "task-12"},
		{Path: canonicalPath, Binding: "task-12"},
	}, map[string]IssueFacts{
		"task-12": {ID: "task-12", Status: "merged", WorktreePath: canonicalPath},
	})

	assert.Equal(t, []string{"task-12"}, result.GCRemovalSet)
	require.Len(t, result.GCRemovals, 1)
	assert.Equal(t, canonicalPath, result.GCRemovals[0].Path,
		"GC must carry the exact recorded canonical path instead of looking up the first binding")
}

func TestReconcile_GCAmbiguousDuplicateMarkersRemovesNothing_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	result := testReconcile([]Meta{
		{Path: "/repo/.worktrees/task-13-a", Binding: "task-13"},
		{Path: "/repo/.worktrees/task-13-b", Binding: "task-13"},
	}, map[string]IssueFacts{
		"task-13": {ID: "task-13", Status: "cancelled", WorktreePath: "/other/clone/task-13"},
	})

	assert.Empty(t, result.GCRemovalSet)
	assert.Empty(t, result.GCRemovals)
	assert.Equal(t, []string{"task-13"}, result.GCAmbiguous)
}

func TestReconcile_WrongPathMarkerStillLeavesRecordedPathGhost_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	recordedPath := "/repo/.worktrees/task-14"
	wrongPath := "/repo/.worktrees/legacy-task-14"
	result := testReconcile([]Meta{{Path: wrongPath, Binding: "task-14"}}, map[string]IssueFacts{
		"task-14": {ID: "task-14", Status: "in-progress", ClaimedBy: "worker-1", WorktreePath: recordedPath},
	})

	assert.Equal(t, []string{"task-14"}, result.Orphans,
		"a binding at the wrong path is not the live recorded worktree")
	assert.Equal(t, []string{"task-14"}, result.Ghosts,
		"a wrong-path binding must not hide the missing recorded path")
}
