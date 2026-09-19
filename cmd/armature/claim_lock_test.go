package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
