package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoteDeleteCommand_PositionalArgs_REQ_LNGHZN_S10(t *testing.T) {
	repo := setupRepoWithTask(t)

	addOut, err := runTrls(t, repo, "--format", "human", "note", "task-01", "hello world")
	require.NoError(t, err)
	assert.Contains(t, addOut, "added to task-01")

	jsonOut, err := runTrls(t, repo, "--format", "json", "note", "task-01", "another note")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, `"note":"added"`)

	const marker = `"note_id":"`
	idx := bytes.Index([]byte(jsonOut), []byte(marker))
	require.Greater(t, idx, -1, "expected note_id field in JSON output: %s", jsonOut)
	rest := jsonOut[idx+len(marker):]
	end := bytes.IndexByte([]byte(rest), '"')
	require.Greater(t, end, -1)
	noteID := rest[:end]

	delOut, err := runTrls(t, repo, "--format", "human", "note", "delete", "task-01", noteID)
	require.NoError(t, err)
	assert.Contains(t, delOut, "deleted from task-01")
}

func TestWorkersCommand_NoWorkers_REQ_LNGHZN_S10(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)

	out, err := runTrls(t, repo, "--format", "human", "workers")
	require.NoError(t, err)
	assert.Contains(t, out, "No workers found.")
}

func TestReparentCommand_ToRoot_REQ_LNGHZN_S10(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "--format", "human", "reparent", "--issue", "task-01", "--parent", "")
	require.NoError(t, err)
	assert.Contains(t, out, "Reparented task-01 to root")
}

func TestSyncCommand_NoMergedBranches_REQ_LNGHZN_S10(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "sync")
	require.NoError(t, err)
	assert.Contains(t, out, "No merged branches detected.")
}
