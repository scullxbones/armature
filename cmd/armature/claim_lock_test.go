package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPessimisticCloneClaimFlockSecondAcquisitionFailsWhileHeld_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	flock, err := tryAcquirePessimisticCloneClaimFlock(repo, "task-01")
	require.NoError(t, err)
	t.Cleanup(flock.Release)

	_, err = tryAcquirePessimisticCloneClaimFlock(repo, "task-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task-01")
	assert.Contains(t, err.Error(), "in progress")
}

func TestPessimisticCloneClaimFlockSucceedsAfterRelease_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	flock, err := tryAcquirePessimisticCloneClaimFlock(repo, "task-01")
	require.NoError(t, err)
	flock.Release()

	flock2, err := tryAcquirePessimisticCloneClaimFlock(repo, "task-01")
	require.NoError(t, err)
	flock2.Release()
}

func TestPessimisticCloneClaimFlockIsPerIssue_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	flockA, err := tryAcquirePessimisticCloneClaimFlock(repo, "task-01")
	require.NoError(t, err)
	t.Cleanup(flockA.Release)

	flockB, err := tryAcquirePessimisticCloneClaimFlock(repo, "task-02")
	require.NoError(t, err)
	flockB.Release()
}

func TestPessimisticCloneClaimFlockContractHoldsOnBuildPlatform_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	flock1, err := tryAcquirePessimisticCloneClaimFlock(repo, "contract-task")
	require.NoError(t, err, "first acquisition on this build platform must succeed")

	_, err = tryAcquirePessimisticCloneClaimFlock(repo, "contract-task")
	require.Error(t, err, "a concurrent acquisition must be refused, never silently granted")

	flock1.Release()

	flock2, err := tryAcquirePessimisticCloneClaimFlock(repo, "contract-task")
	require.NoError(t, err, "acquisition must succeed again after a genuine release")
	flock2.Release()
}
