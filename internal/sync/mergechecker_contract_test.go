package sync_test

import (
	"testing"

	armsync "github.com/scullxbones/armature/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RunMergeCheckerContract tests that a MergeChecker implementation correctly
// reports whether a branch is merged into a target branch.
// This is a shared contract test that any MergeChecker impl must satisfy.
func RunMergeCheckerContract(t *testing.T, mc armsync.MergeChecker) {
	t.Run("BranchMergedInto_MergedBranch_ReturnsTrue", func(t *testing.T) {
		t.Parallel()
		// Set up expectation: branch "feature/done" is merged into "main"
		result, err := mc.BranchMergedInto("feature/done", "main")
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("BranchMergedInto_UnmergedBranch_ReturnsFalse", func(t *testing.T) {
		t.Parallel()
		// Set up expectation: branch "feature/wip" is not merged into "main"
		result, err := mc.BranchMergedInto("feature/wip", "main")
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("BranchMergedInto_SameBranch_ReturnsTrue", func(t *testing.T) {
		t.Parallel()
		// A branch is always "merged" into itself
		result, err := mc.BranchMergedInto("main", "main")
		require.NoError(t, err)
		assert.True(t, result)
	})
}

// FakeMergeChecker is a test double for MergeChecker.
// It allows configuring which branches are merged via a map.
type FakeMergeChecker struct {
	merged map[string]bool
	err    error
}

// NewFakeMergeChecker creates a FakeMergeChecker with the given merged branch states.
func NewFakeMergeChecker(merged map[string]bool) *FakeMergeChecker {
	return &FakeMergeChecker{merged: merged}
}

// BranchMergedInto returns whether the given branch is merged into target.
// The merged map is keyed by branch name; if the branch exists in the map, its value is returned.
func (f *FakeMergeChecker) BranchMergedInto(branch, target string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if branch == target {
		return true, nil
	}
	return f.merged[branch], nil
}

// TestFakeMergeChecker_SatisfiesContract verifies that FakeMergeChecker satisfies the MergeChecker contract.
func TestFakeMergeChecker_SatisfiesContract(t *testing.T) {
	t.Parallel()
	// Set up a fake that knows about specific merged/unmerged branches
	mc := NewFakeMergeChecker(map[string]bool{
		"feature/done": true,  // merged
		"feature/wip":  false, // not merged
		"main":         true,  // self-reference
	})

	RunMergeCheckerContract(t, mc)
}
