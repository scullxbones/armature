package materialize

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyClaimOpWithoutWorktreePathReplays_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()

	lines := [][]byte{
		[]byte(`["create","task-01",100,"worker-1",{"type":"task","title":"t","definition_of_done":"d"}]`),
		[]byte(`["claim","task-01",101,"worker-1",{"ttl":60}]`),
	}

	state := NewState()
	for _, line := range lines {
		op, err := ops.ParseLine(line)
		require.NoError(t, err)
		require.NoError(t, state.ApplyOp(op))
	}

	issue := state.Issues["task-01"]
	require.NotNil(t, issue)
	assert.Equal(t, "", issue.WorktreePath, "legacy claim replays with empty WorktreePath")
	assert.Equal(t, "worker-1", issue.ClaimedBy)
}

func TestClaimOpWorktreePathReplays_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()

	lines := [][]byte{
		[]byte(`["create","task-02",100,"worker-1",{"type":"task","title":"t","definition_of_done":"d"}]`),
		[]byte(`["claim","task-02",101,"worker-1",{"ttl":60,"worktree_path":"/repo/.worktrees/task-02"}]`),
	}

	state := NewState()
	for _, line := range lines {
		op, err := ops.ParseLine(line)
		require.NoError(t, err)
		require.NoError(t, state.ApplyOp(op))
	}

	issue := state.Issues["task-02"]
	require.NotNil(t, issue)
	assert.Equal(t, "/repo/.worktrees/task-02", issue.WorktreePath)
}
