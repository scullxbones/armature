package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcquireClaimLockSecondAcquisitionFailsWhileHeld_REQ_LNGHZN_S5_T9 covers
// finding 2 (the destructive-filesystem race in cleanupPartialWorktree): a
// second acquisition of the per-issue claim lock for the same issue in the
// same clone must fail immediately (non-blocking) while the first holder
// still has it, with a clear, actionable error.
func TestAcquireClaimLockSecondAcquisitionFailsWhileHeld_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	release, err := acquireClaimLock(repo, "task-01")
	require.NoError(t, err)
	t.Cleanup(release)

	_, err = acquireClaimLock(repo, "task-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task-01")
	assert.Contains(t, err.Error(), "in progress")
}

// TestAcquireClaimLockSucceedsAfterRelease_REQ_LNGHZN_S5_T9 verifies the lock
// is genuinely released (not merely closed without unlocking, and not
// leaking into a permanently-locked state): once release() runs, a fresh
// acquisition for the same issue in the same clone must succeed.
func TestAcquireClaimLockSucceedsAfterRelease_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	release, err := acquireClaimLock(repo, "task-01")
	require.NoError(t, err)
	release()

	release2, err := acquireClaimLock(repo, "task-01")
	require.NoError(t, err)
	release2()
}

// TestAcquireClaimLockIsPerIssue_REQ_LNGHZN_S5_T9 verifies the lock is scoped
// to a single issue: holding the lock for one issue must not block acquiring
// it for a different issue in the same clone, since claims for unrelated
// issues never contend for the same worktree path.
func TestAcquireClaimLockIsPerIssue_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	releaseA, err := acquireClaimLock(repo, "task-01")
	require.NoError(t, err)
	t.Cleanup(releaseA)

	releaseB, err := acquireClaimLock(repo, "task-02")
	require.NoError(t, err)
	releaseB()
}

// TestAcquireClaimLockContractHoldsOnBuildPlatform_REQ_LNGHZN_S5_T9 pins the
// contract acquireClaimLock relies on from internal/filelock's TryLock/Unlock
// on whichever platform this test binary was built for
// (internal/filelock/filelock_unix.go on unix,
// internal/filelock/filelock_windows.go on windows): a first acquisition
// must succeed, a concurrent second acquisition for the same issue must be
// reported as held (not silently granted), and release must be genuine so a
// later acquisition succeeds again. This is the same contract
// internal/filelock/filelock_other.go's fail-closed fallback deliberately
// refuses to provide on any platform that is neither unix nor windows — see
// that file's tryLock, which returns (false, non-nil error) unconditionally
// rather than (true, nil), so acquireClaimLock surfaces a hard error
// instead of silently reporting success with no real exclusion. The CI
// host running this test is unix, so the windows and "other" build
// variants cannot be exercised at runtime here; see the task report's
// TEST_EXCEPTION for the cross-compile verification of those variants.
func TestAcquireClaimLockContractHoldsOnBuildPlatform_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	release1, err := acquireClaimLock(repo, "contract-task")
	require.NoError(t, err, "first acquisition on this build platform must succeed")

	_, err = acquireClaimLock(repo, "contract-task")
	require.Error(t, err, "a concurrent acquisition must be refused, never silently granted")

	release1()

	release2, err := acquireClaimLock(repo, "contract-task")
	require.NoError(t, err, "acquisition must succeed again after a genuine release")
	release2()
}
