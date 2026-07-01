package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepoProbe struct {
	worktreePath string
}

func (f fakeRepoProbe) Probe(repoPath string) (RepoProbeResult, error) {
	return RepoProbeResult{
		RepoPath:     repoPath,
		WorktreePath: f.worktreePath,
	}, nil
}

func TestResolveContextAlwaysUsesOpsWorktree_REQ_SB_T5(t *testing.T) {
	t.Parallel()
	probe := fakeRepoProbe{
		worktreePath: "/repo/.arm",
	}

	ctx, err := ResolveContextWithProbe("/repo", probe, Config{})

	require.NoError(t, err)
	assert.Equal(t, "/repo", ctx.RepoPath)
	assert.Equal(t, "/repo/.arm/.armature", ctx.IssuesDir)
	assert.Equal(t, "/repo/.arm", ctx.WorktreePath)
}

func TestResolveContextErrorsWhenOpsWorktreePathEmpty_REQ_SB_T5(t *testing.T) {
	t.Parallel()
	probe := fakeRepoProbe{
		worktreePath: "",
	}

	_, err := ResolveContextWithProbe("/repo", probe, Config{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "armature.ops-worktree-path")
}
