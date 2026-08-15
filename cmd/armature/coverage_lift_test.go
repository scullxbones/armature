package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoteDeleteCommand_PositionalArgs_REQ_LNGHZN_S10 exercises `arm note delete
// <issue-id> <note-id>` — the len(args) >= 3 positional-recovery branch in
// newNoteCmd's RunE (note.go), where args[0]=="delete", args[1]=issue ID, and
// args[2]=note ID are recovered without --issue/--note-id flags.
func TestNoteDeleteCommand_PositionalArgs_REQ_LNGHZN_S10(t *testing.T) {
	repo := setupRepoWithTask(t)

	addOut, err := runTrls(t, repo, "--format", "human", "note", "task-01", "hello world")
	require.NoError(t, err)
	assert.Contains(t, addOut, "added to task-01")

	// Extract the note ID from JSON output for a deterministic delete target.
	jsonOut, err := runTrls(t, repo, "--format", "json", "note", "task-01", "another note")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, `"note":"added"`)

	// Pull the note ID out of the JSON payload.
	const marker = `"note_id":"`
	idx := bytes.Index([]byte(jsonOut), []byte(marker))
	require.Greater(t, idx, -1, "expected note_id field in JSON output: %s", jsonOut)
	rest := jsonOut[idx+len(marker):]
	end := bytes.IndexByte([]byte(rest), '"')
	require.Greater(t, end, -1)
	noteID := rest[:end]

	// Delete using the positional form: "delete" "<issue-id>" "<note-id>".
	delOut, err := runTrls(t, repo, "--format", "human", "note", "delete", "task-01", noteID)
	require.NoError(t, err)
	assert.Contains(t, delOut, "deleted from task-01")
}

// TestWorkersCommand_NoWorkers_REQ_LNGHZN_S10 exercises the empty-state branch
// in newWorkersCmd's RunE (workers.go): `arm workers` on a repo with no worker
// activity in the op log must print "No workers found." rather than a table.
func TestWorkersCommand_NoWorkers_REQ_LNGHZN_S10(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)

	out, err := runTrls(t, repo, "--format", "human", "workers")
	require.NoError(t, err)
	assert.Contains(t, out, "No workers found.")
}

// TestReparentCommand_ToRoot_REQ_LNGHZN_S10 exercises the newParent == ""
// human-format branch in newReparentCmd's RunE (reparent.go): reparenting an
// issue with an empty --parent must report "Reparented <id> to root" rather
// than naming a parent.
func TestReparentCommand_ToRoot_REQ_LNGHZN_S10(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "--format", "human", "reparent", "--issue", "task-01", "--parent", "")
	require.NoError(t, err)
	assert.Contains(t, out, "Reparented task-01 to root")
}

// TestSyncCommand_NoMergedBranches_REQ_LNGHZN_S10 exercises the
// len(mergedIDs) == 0 branch in newSyncCmd's RunE (sync.go): on a repo where
// no feature branch has been merged, `arm sync` must report "No merged
// branches detected." instead of transitioning anything.
func TestSyncCommand_NoMergedBranches_REQ_LNGHZN_S10(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "sync")
	require.NoError(t, err)
	assert.Contains(t, out, "No merged branches detected.")
}
