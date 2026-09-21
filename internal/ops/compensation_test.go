package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompensationEncode_REQ_MATENC_S1_T3(t *testing.T) {
	t.Parallel()

	t.Run("unchanged encodes neither worktree flag", func(t *testing.T) {
		t.Parallel()
		p := Compensation{
			To:           StatusOpen,
			RestoreClaim: true,
			IfClaimToken: "tok",
			Worktree:     WorktreeRestore{Action: WorktreeUnchanged},
		}.Encode()
		assert.False(t, p.ClearWorktreePath)
		assert.Empty(t, p.WorktreePath)
		assert.Equal(t, WorktreeUnchanged, DecodeWorktreeRestore(p).Action)
	})

	t.Run("clear encodes clear_worktree_path only", func(t *testing.T) {
		t.Parallel()
		p := Compensation{Worktree: WorktreeRestore{Action: WorktreeClear}}.Encode()
		assert.True(t, p.ClearWorktreePath)
		assert.Empty(t, p.WorktreePath)
		assert.Equal(t, WorktreeClear, DecodeWorktreeRestore(p).Action)
	})

	t.Run("set encodes worktree_path only", func(t *testing.T) {
		t.Parallel()
		p := Compensation{Worktree: WorktreeRestore{Action: WorktreeSet, Path: "/wt"}}.Encode()
		assert.False(t, p.ClearWorktreePath)
		assert.Equal(t, "/wt", p.WorktreePath)
		got := DecodeWorktreeRestore(p)
		assert.Equal(t, WorktreeSet, got.Action)
		assert.Equal(t, "/wt", got.Path)
	})

	t.Run("empty set encodes as clear not leave", func(t *testing.T) {
		t.Parallel()
		path, clear := EncodeWorktree(WorktreeRestore{Action: WorktreeSet, Path: ""})
		assert.True(t, clear)
		assert.Empty(t, path)
		decoded := DecodeWorktreeRestore(Payload{WorktreePath: path, ClearWorktreePath: clear})
		assert.Equal(t, WorktreeClear, decoded.Action)
	})

	t.Run("clear wins when both wire flags are set", func(t *testing.T) {
		t.Parallel()
		got := DecodeWorktreeRestore(Payload{WorktreePath: "/stale", ClearWorktreePath: true})
		assert.Equal(t, WorktreeClear, got.Action)
		assert.Empty(t, got.Path)
	})

	t.Run("round-trip restore lease fields", func(t *testing.T) {
		t.Parallel()
		c := Compensation{
			To:                                StatusInProgress,
			RestoreClaim:                      true,
			RestoreClaimedBy:                  "w1",
			RestoreClaimedAt:                  10,
			RestoreClaimTTL:                   1,
			RestoreLastHeartbeat:              20,
			RestoreLastClaimingWorkerActivity: 30,
			RestoreClaimToken:                 "prior",
			IfClaimToken:                      "won",
			Worktree:                          WorktreeRestore{Action: WorktreeSet, Path: "/p"},
		}
		p := c.Encode()
		assert.Equal(t, c.To, p.To)
		assert.Equal(t, c.RestoreClaim, p.RestoreClaim)
		assert.Equal(t, c.RestoreClaimedBy, p.RestoreClaimedBy)
		assert.Equal(t, c.RestoreClaimedAt, p.RestoreClaimedAt)
		assert.Equal(t, c.RestoreClaimTTL, p.RestoreClaimTTL)
		assert.Equal(t, c.RestoreLastHeartbeat, p.RestoreLastHeartbeat)
		assert.Equal(t, c.RestoreLastClaimingWorkerActivity, p.RestoreLastClaimingWorkerActivity)
		assert.Equal(t, c.RestoreClaimToken, p.RestoreClaimToken)
		assert.Equal(t, c.IfClaimToken, p.IfClaimToken)
		assert.Equal(t, c.Worktree, DecodeWorktreeRestore(p))
	})
}

func TestWorktreeRestoreApply_REQ_MATENC_S1_T3(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", WorktreeRestore{Action: WorktreeClear}.Apply("/old"))
	assert.Equal(t, "/new", WorktreeRestore{Action: WorktreeSet, Path: "/new"}.Apply("/old"))
	assert.Equal(t, "/old", WorktreeRestore{Action: WorktreeUnchanged}.Apply("/old"))
	assert.Equal(t, "", WorktreeRestore{Action: WorktreeUnchanged}.Apply(""))
}

func TestCompensationEncode_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	c := Compensation{Worktree: WorktreeRestore{Action: WorktreeSet, Path: "/p"}, IfClaimToken: "t"}
	before := c
	_ = c.Encode()
	require.Equal(t, before, c)
}
