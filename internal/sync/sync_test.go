package sync_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	armsync "github.com/scullxbones/armature/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectMerges_ReturnsMergedIssueIDs(t *testing.T) {
	t.Parallel()
	// done + merged branch
	issue1 := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "feature/merged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	// done + unmerged branch
	issue2 := materialize.Issue{
		ID: "T-002", Status: "done", Branch: "feature/unmerged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	// in-progress — should be skipped regardless of branch status
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
	// No issues provided
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

	// DetectMerges should check all provided issues
	ids, err := armsync.DetectMerges([]materialize.Issue{issue}, "main", mc)
	require.NoError(t, err)
	assert.Equal(t, []string{"T-001"}, ids)
}
