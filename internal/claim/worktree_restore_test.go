package claim_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestWorktreeRestore_REQ_MATENC_S1_T3(t *testing.T) {
	t.Parallel()

	t.Run("clear empty prior", func(t *testing.T) {
		t.Parallel()
		w := claim.WorktreeRestore("")
		assert.Equal(t, ops.WorktreeClear, w.Action)
		assert.Empty(t, w.Path)
		p := ops.Compensation{Worktree: w}.Encode()
		assert.True(t, p.ClearWorktreePath)
		assert.Empty(t, p.WorktreePath)
		assert.Equal(t, "", w.Apply("/failed-claim"))
	})

	t.Run("set nonempty prior", func(t *testing.T) {
		t.Parallel()
		prior := "/repo/.worktrees/TASK-1"
		w := claim.WorktreeRestore(prior)
		assert.Equal(t, ops.WorktreeSet, w.Action)
		assert.Equal(t, prior, w.Path)
		p := ops.Compensation{Worktree: w}.Encode()
		assert.False(t, p.ClearWorktreePath)
		assert.Equal(t, prior, p.WorktreePath)
		assert.Equal(t, prior, w.Apply("/failed-claim"))
	})

	t.Run("leave encodes neither flag", func(t *testing.T) {
		t.Parallel()
		w := ops.WorktreeRestore{Action: ops.WorktreeUnchanged}
		assert.Equal(t, ops.WorktreeUnchanged, w.Action)
		assert.Empty(t, w.Path)
		p := ops.Compensation{Worktree: w}.Encode()
		assert.False(t, p.ClearWorktreePath)
		assert.Empty(t, p.WorktreePath)
		assert.Equal(t, "/live", w.Apply("/live"))
		assert.Equal(t, ops.WorktreeUnchanged, ops.DecodeWorktreeRestore(p).Action)
	})
}
