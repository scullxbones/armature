package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveContextAlwaysUsesOpsWorktree_REQ_SB_T5(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/repo/.arm/.armature", issuesDirFor("/repo/.arm"))
	assert.Equal(t, "/repo/.armature", issuesDirFor("/repo/.armature"))
}

func TestResolveContextErrorsWhenOpsWorktreePathEmpty_REQ_SB_T5(t *testing.T) {
	t.Parallel()
	_, err := ResolveLayout(initTestRepo(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "armature.ops-worktree-path")
}
