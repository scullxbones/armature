package ops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaimOpRecordsWorktreePath_REQ_LNGHZN_S5_T1 verifies that a claim op with a
// WorktreePath field is properly recorded and accessible on the payload, which
// recovery tooling can use to find the worktree deterministically.
func TestClaimOpRecordsWorktreePath_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()
	op := Op{
		Type:      OpClaim,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  "worker-1",
		Payload: Payload{
			TTL:          60,
			WorktreePath: "/home/user/repo/.worktrees/task-01",
		},
	}

	require.Equal(t, "/home/user/repo/.worktrees/task-01", op.Payload.WorktreePath)
}

// TestLegacyClaimOpWithoutWorktreePathReplays_REQ_LNGHZN_S5_T1 verifies backward
// compatibility at the JSON level: a claim op serialized WITHOUT a worktree_path
// key (a legacy op) parses cleanly and yields an empty WorktreePath, which is the
// expected behavior for ops recorded before this field was added.
func TestLegacyClaimOpWithoutWorktreePathReplays_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()
	// A legacy claim line, no worktree_path key in the payload object.
	line := []byte(`["claim","task-01",100,"worker-1",{"ttl":60}]`)

	op, err := ParseLine(line)
	require.NoError(t, err)

	require.Equal(t, OpClaim, op.Type)
	require.Equal(t, 60, op.Payload.TTL)
	require.Equal(t, "", op.Payload.WorktreePath)
}

// TestClaimOpJSONRoundtripWithWorktreePath_REQ_LNGHZN_S5_T1 verifies that a claim
// op with WorktreePath survives a real JSONL marshal/parse round-trip, since ops
// are persisted in JSON form in the op log.
func TestClaimOpJSONRoundtripWithWorktreePath_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()
	original := Op{
		Type:      OpClaim,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  "worker-1",
		Payload: Payload{
			TTL:          60,
			WorktreePath: "/home/user/repo/.worktrees/task-01",
		},
	}

	data, err := MarshalOp(original)
	require.NoError(t, err)

	restored, err := ParseLine(data)
	require.NoError(t, err)

	require.Equal(t, original.Payload.WorktreePath, restored.Payload.WorktreePath)
	require.Equal(t, original.Payload.TTL, restored.Payload.TTL)
}
