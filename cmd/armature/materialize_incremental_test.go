package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/require"
)

func TestMaterializeCommand_UsesCommitIncremental_REQ_CLAIMORD_W14(t *testing.T) {
	_, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "create", "--type", "task", "--title", "inc", "--id", "task-inc")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	workerID := strings.TrimSpace(runOutput(t, repo, "config", "--get", "armature.worker-id"))
	require.NotEmpty(t, workerID)
	cp, err := materialize.LoadCheckpoint(filepath.Join(worktree, "state", workerID, "checkpoint.json"))
	require.NoError(t, err)
	require.NotEmpty(t, cp.LastCommitSHA,
		"arm materialize must pass OpsWorktree; file-concat replay wipes last_materialized_commit")
}
