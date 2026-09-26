package filelock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestLockFile(t *testing.T) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	return f
}

func reopenTestLockFile(t *testing.T, f *os.File) *os.File {
	t.Helper()
	f2, err := os.OpenFile(f.Name(), os.O_CREATE|os.O_RDWR, 0o600)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f2.Close()) })
	return f2
}

func TestTryLockSucceedsWhenFree_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	f := openTestLockFile(t)

	ok, err := TryLock(f)
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, Unlock(f))
}

func TestTryLockReportsHeldWithoutBlockingOrError_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	f := openTestLockFile(t)
	other := reopenTestLockFile(t, f)

	ok, err := TryLock(f)
	require.NoError(t, err)
	require.True(t, ok)

	held, err := TryLock(other)
	require.NoError(t, err)
	assert.False(t, held, "a second handle must not be able to acquire a lock already held elsewhere")

	require.NoError(t, Unlock(f))
}

func TestTryLockSucceedsAgainAfterUnlock_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	f := openTestLockFile(t)
	other := reopenTestLockFile(t, f)

	ok, err := TryLock(f)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, Unlock(f))

	ok, err = TryLock(other)
	require.NoError(t, err)
	assert.True(t, ok, "lock must be acquirable again after a genuine unlock")

	require.NoError(t, Unlock(other))
}

func TestLockUnlockRoundTrip_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	f := openTestLockFile(t)
	other := reopenTestLockFile(t, f)

	require.NoError(t, Lock(f))
	require.NoError(t, Unlock(f))

	ok, err := TryLock(other)
	require.NoError(t, err)
	assert.True(t, ok)
	require.NoError(t, Unlock(other))
}
