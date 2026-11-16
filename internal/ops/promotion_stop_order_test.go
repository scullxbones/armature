package ops

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAllPromotionStops_SeqByBasenameThenLine_REQ_482(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Lexically later file gets higher Seq even when written "first" into the slice.
	zPath := filepath.Join(dir, "z-worker.log")
	aPath := filepath.Join(dir, "a-worker.log")
	require.NoError(t, AppendPromotionStop(zPath, "z", PromotionStop{
		Action: PromotionStopTrip, TreeRoot: "t1", IssueID: "t1", Kind: "k", At: 100,
	}))
	require.NoError(t, AppendPromotionStop(aPath, "a", PromotionStop{
		Action: PromotionStopClear, TreeRoot: "t1", NoteID: "n1", At: 200,
	}))

	got, err := ReadAllPromotionStops(dir)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, PromotionStopClear, got[0].Action, "a-worker.log sorts before z-worker.log")
	assert.Equal(t, 0, got[0].Seq)
	assert.Equal(t, int64(200), got[0].At)
	assert.Equal(t, PromotionStopTrip, got[1].Action)
	assert.Equal(t, 1, got[1].Seq)
	assert.Equal(t, int64(100), got[1].At)
}
