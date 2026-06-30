package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepoProbe struct {
	mode         string
	worktreePath string
}

func (f fakeRepoProbe) Probe(repoPath string) (RepoProbeResult, error) {
	return RepoProbeResult{
		RepoPath:     repoPath,
		Mode:         f.mode,
		WorktreePath: f.worktreePath,
	}, nil
}

func TestResolveContextSeparatesRepoProbeFromContextDerivation(t *testing.T) {
	t.Parallel()
	probe := fakeRepoProbe{
		mode:         "dual-branch",
		worktreePath: "/repo/.arm",
	}

	ctx, err := ResolveContextWithProbe("/repo", probe, Config{})

	require.NoError(t, err)
	assert.Equal(t, "/repo", ctx.RepoPath)
	assert.Equal(t, "/repo/.arm/.armature", ctx.IssuesDir)
}
