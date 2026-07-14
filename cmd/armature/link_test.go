package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLinkCmd_RejectsUnsupportedRel verifies that --rel is validated before the
// op is written to the log. Without this, an op with an unsupported rel (e.g.
// "blocks", which is derived automatically and was previously advertised as a
// valid input) would be appended successfully, then every later command would
// fail at materialize time with no way to recover from the append-only log.
func TestLinkCmd_RejectsUnsupportedRel(t *testing.T) {
	repo := setupRepoWithTwoTasks(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "link", "--source", "task-01", "--dep", "task-02", "--rel", "blocks")
	require.Error(t, err, "link --rel blocks should be rejected before the op is written")
	assert.Contains(t, err.Error(), "blocked_by")

	// The rejected op must not have been written — a later command must still work.
	out, err := runTrls(t, repo, "ready")
	require.NoError(t, err, "repo must remain usable after a rejected link")
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
}

func TestLinkCmd_AcceptsBlockedBy(t *testing.T) {
	repo := setupRepoWithTwoTasks(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "link", "--source", "task-01", "--dep", "task-02", "--rel", "blocked_by")
	require.NoError(t, err)
}
