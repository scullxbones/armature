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
	t.Cleanup(func() { _ = f.Close() }) //nolint:errcheck // best-effort close of a test lock file in cleanup
	return f
}

// reopenTestLockFile opens a second, independent handle to the same lock
// file so tests can exercise cross-handle contention the way two separate
// processes would.
func reopenTestLockFile(t *testing.T, f *os.File) *os.File {
	t.Helper()
	f2, err := os.OpenFile(f.Name(), os.O_CREATE|os.O_RDWR, 0o600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f2.Close() }) //nolint:errcheck // best-effort close of a test lock file in cleanup
	return f2
}

// TestTryLockSucceedsWhenFree_REQ_LNGHZN_S5_T9 verifies TryLock acquires the
// lock immediately when nothing else holds it.
func TestTryLockSucceedsWhenFree_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	f := openTestLockFile(t)

	ok, err := TryLock(f)
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, Unlock(f))
}

// TestTryLockReportsHeldWithoutBlockingOrError_REQ_LNGHZN_S5_T9 verifies
// that when another handle already holds the lock, TryLock reports it as
// held via (false, nil) — never silently reporting success, and never
// returning an error for the ordinary "already locked" case.
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

// TestTryLockSucceedsAgainAfterUnlock_REQ_LNGHZN_S5_T9 verifies the lock is
// genuinely released by Unlock: a fresh TryLock from another handle must
// succeed once the original holder unlocks.
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

// TestLockUnlockRoundTrip_REQ_LNGHZN_S5_T9 verifies the blocking Lock/Unlock
// pair works end to end, and that once unlocked, another handle can take the
// lock (non-blocking, since nothing else holds it).
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
