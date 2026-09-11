package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkersEmitsEnvelopeNotJSONL_REQ_AOC_S2_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	for _, extra := range [][]string{
		{"--format", "json"},
		{"--format", "agent"},
		{"--json"},
	} {
		args := append([]string{"workers"}, extra...)
		out, err := runTrls(t, repo, args...)
		require.NoError(t, err, "args=%v", extra)

		decoded := decodeContractEnvelope(t, out, "workers")
		var workers []WorkerStatus
		require.NoError(t, json.Unmarshal(decoded["workers"], &workers))
		require.NotEmpty(t, workers)
		assert.NotEmpty(t, workers[0].WorkerID)
		assert.NotEmpty(t, workers[0].Status)
	}
}
