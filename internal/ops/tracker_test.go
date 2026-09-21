package ops_test

import (
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoTracker_AlwaysReturnsZero(t *testing.T) {
	t.Parallel()
	tr := ops.NoTracker{}
	n, err := tr.Increment()
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	n, err = tr.Count()
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	require.NoError(t, tr.Reset())
}

func TestFilePushTracker_IncrementAndReset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tr := ops.NewFilePushTracker(dir)

	n, err := tr.Count()
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	n, err = tr.Increment()
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	n, err = tr.Increment()
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	n, err = tr.Increment()
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	n, err = tr.Count()
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	require.NoError(t, tr.Reset())
	n, err = tr.Count()
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestFilePushTracker_PersistenceAcrossInstances(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tr1 := ops.NewFilePushTracker(dir)
	_, err := tr1.Increment()
	require.NoError(t, err)
	_, err = tr1.Increment()
	require.NoError(t, err)
	_, err = tr1.Increment()
	require.NoError(t, err)

	tr2 := ops.NewFilePushTracker(dir)
	n, err := tr2.Count()
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestFilePushTracker_DefaultThreshold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tr := ops.NewFilePushTracker(dir)

	threshold := 5
	for i := 0; i < threshold-1; i++ {
		n, err := tr.Increment()
		require.NoError(t, err)
		assert.Less(t, n, threshold)
	}
	n, err := tr.Increment()
	require.NoError(t, err)
	assert.Equal(t, threshold, n)
}

func TestTrackerUsesStateDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tr := ops.NewFilePushTracker(dir)
	assert.Equal(t, filepath.Join(dir, "pending-push-count"), tr.Path)
}
