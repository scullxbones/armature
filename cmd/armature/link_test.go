package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkCmd_RejectsUnsupportedRel(t *testing.T) {
	repo := setupRepoWithTwoTasks(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "link", "--source", "task-01", "--dep", "task-02", "--rel", "blocks")
	require.Error(t, err, "link --rel blocks should be rejected before the op is written")
	assert.Contains(t, err.Error(), "blocked_by")

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
