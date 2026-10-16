package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveContextKeepsInvokingWorktree_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	parent := initTestRepo(t)

	opsWorktree := filepath.Join(parent, ".arm")
	opsIssuesDir := filepath.Join(opsWorktree, ".armature")
	require.NoError(t, os.MkdirAll(opsIssuesDir, 0o755))
	require.NoError(t, WriteConfig(filepath.Join(opsIssuesDir, "config.json"), DefaultConfig("go")))
	gittest.Git(t, parent, "config", "armature.ops-worktree-path", opsWorktree)
	gittest.Git(t, parent, "config", "extensions.worktreeConfig", "true")
	gittest.Git(t, parent, "config", "--worktree", "armature.worker-id", "wt-main")

	first := filepath.Join(t.TempDir(), "wt-one")
	second := filepath.Join(t.TempDir(), "wt-two")
	gittest.Git(t, parent, "worktree", "add", first, "HEAD")
	gittest.Git(t, parent, "worktree", "add", second, "HEAD")
	gittest.Git(t, first, "config", "--worktree", "armature.worker-id", "wt-one")
	gittest.Git(t, second, "config", "--worktree", "armature.worker-id", "wt-two")

	ctx, err := ResolveContext(first)
	require.NoError(t, err)

	assert.Equal(t, absClean(t, first), absClean(t, ctx.invokingWorktree().String()))
	assert.Equal(t, absClean(t, parent), absClean(t, ctx.gitDir().String()))
	assert.NotEqual(t, ctx.invokingWorktree(), invokingWorktreePath(ctx.gitDir().String()),
		"ResolveContext must not substitute the main checkout for the invoking worktree")
	assert.Equal(t, opsIssuesDir, ctx.IssuesDir)
	assert.Equal(t, opsWorktree, ctx.WorktreePath)

	assert.Equal(t, "wt-one", worktreeWorkerID(t, ctx.invokingWorktree().String()))
	assert.Equal(t, "wt-main", worktreeWorkerID(t, ctx.gitDir().String()))

	secondCtx, err := ResolveContext(second)
	require.NoError(t, err)
	assert.Equal(t, absClean(t, second), absClean(t, secondCtx.invokingWorktree().String()))
	assert.Equal(t, "wt-two", worktreeWorkerID(t, secondCtx.invokingWorktree().String()))
	assert.NotEqual(t, worktreeWorkerID(t, first), worktreeWorkerID(t, second),
		"a second worktree must not inherit the first worktree's worker-id")
}

func worktreeWorkerID(t *testing.T, checkout string) string {
	t.Helper()
	id, err := adapters.New(checkout).ReadGitConfigWorktree("armature.worker-id")
	require.NoError(t, err, "read armature.worker-id from %s", checkout)
	return id
}

func absClean(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(resolved)
}
