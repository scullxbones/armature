package ops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestLegacyClaimOpWithoutWorktreePathReplays_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()
	line := []byte(`["claim","task-01",100,"worker-1",{"ttl":60}]`)

	op, err := ParseLine(line)
	require.NoError(t, err)

	require.Equal(t, OpClaim, op.Type)
	require.Equal(t, 60, op.Payload.TTL)
	require.Equal(t, "", op.Payload.WorktreePath)
}

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
