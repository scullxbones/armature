package sync_test

import (
	"testing"

	armsync "github.com/scullxbones/armature/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func RunMergeCheckerContract(t *testing.T, mc armsync.MergeChecker) {
	t.Run("BranchMergedInto_MergedBranch_ReturnsTrue", func(t *testing.T) {
		t.Parallel()
		result, err := mc.BranchMergedInto("feature/done", "main")
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("BranchMergedInto_UnmergedBranch_ReturnsFalse", func(t *testing.T) {
		t.Parallel()
		result, err := mc.BranchMergedInto("feature/wip", "main")
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("BranchMergedInto_SameBranch_ReturnsTrue", func(t *testing.T) {
		t.Parallel()
		result, err := mc.BranchMergedInto("main", "main")
		require.NoError(t, err)
		assert.True(t, result)
	})
}

type FakeMergeChecker struct {
	merged map[string]bool
	errs   map[string]error
	err    error
}

func NewFakeMergeChecker(merged map[string]bool) *FakeMergeChecker {
	return &FakeMergeChecker{merged: merged}
}

func NewFakeMergeCheckerWithErrors(merged map[string]bool, errs map[string]error) *FakeMergeChecker {
	return &FakeMergeChecker{merged: merged, errs: errs}
}

func (f *FakeMergeChecker) BranchMergedInto(branch, target string) (bool, error) {
	if f.errs != nil {
		if err, ok := f.errs[branch]; ok {
			return false, err
		}
	}
	if f.err != nil {
		return false, f.err
	}
	if branch == target {
		return true, nil
	}
	return f.merged[branch], nil
}

func TestFakeMergeChecker_SatisfiesContract(t *testing.T) {
	t.Parallel()
	mc := NewFakeMergeChecker(map[string]bool{
		"feature/done": true,
		"feature/wip":  false,
		"main":         true,
	})

	RunMergeCheckerContract(t, mc)
}
