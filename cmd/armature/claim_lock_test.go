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
