package sync_test

import (
	"errors"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	armsync "github.com/scullxbones/armature/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectMerges_ReturnsMergedIssueIDs(t *testing.T) {
	t.Parallel()
	issue1 := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "feature/merged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	issue2 := materialize.Issue{
		ID: "T-002", Status: "done", Branch: "feature/unmerged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	issue3 := materialize.Issue{
		ID: "T-003", Status: "in-progress", Branch: "feature/wip", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}

	mc := NewFakeMergeChecker(map[string]bool{
		"feature/merged-work": true,
	})

	ids, err := armsync.DetectMerges([]materialize.Issue{issue1, issue2, issue3}, "main", mc)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"T-001"}, ids)
}

func TestDetectMerges_NoBranch_Skipped(t *testing.T) {
	t.Parallel()
	issue := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}

	mc := NewFakeMergeChecker(map[string]bool{})

	ids, err := armsync.DetectMerges([]materialize.Issue{issue}, "main", mc)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestDetectMerges_EmptyDir(t *testing.T) {
	t.Parallel()
	mc := NewFakeMergeChecker(map[string]bool{})
	ids, err := armsync.DetectMerges([]materialize.Issue{}, "main", mc)
	assert.NoError(t, err)
	assert.Empty(t, ids)
}

func TestSyncDetectMergesChecksAllIssues(t *testing.T) {
	t.Parallel()
	issue := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "feature/merged", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}

	mc := NewFakeMergeChecker(map[string]bool{"feature/merged": true})

	ids, err := armsync.DetectMerges([]materialize.Issue{issue}, "main", mc)
	require.NoError(t, err)
	assert.Equal(t, []string{"T-001"}, ids)
}

func TestDetectMerges_PartialGitFailure_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	merged := materialize.Issue{
		ID: "T-MERGED", Status: "done", Branch: "feature/merged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	broken := materialize.Issue{
		ID: "T-BROKEN", Status: "done", Branch: "feature/broken-check", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}

	mc := NewFakeMergeCheckerWithErrors(
		map[string]bool{"feature/merged-work": true},
		map[string]error{"feature/broken-check": errors.New("git cat-file failed")},
	)

	ids, err := armsync.DetectMerges([]materialize.Issue{broken, merged}, "main", mc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "feature/broken-check")
	assert.Equal(t, []string{"T-MERGED"}, ids)
}
