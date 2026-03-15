package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

func TestResolveContext_SingleBranch_Default(t *testing.T) {
	repo := initTestRepo(t)

	// Create .issues/config.json so LoadConfig works
	issuesDir := filepath.Join(repo, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, "single-branch", ctx.Mode)
	assert.Equal(t, filepath.Join(repo, ".issues"), ctx.IssuesDir)
	assert.Equal(t, repo, ctx.RepoPath)
	assert.Equal(t, "go", ctx.Config.ProjectType)
}

func TestResolveContext_DualBranch_ReturnsError(t *testing.T) {
	repo := initTestRepo(t)

	// Set dual-branch mode in git config
	cmd := exec.Command("git", "-C", repo, "config", "trellis.mode", "dual-branch")
	require.NoError(t, cmd.Run())

	_, err := ResolveContext(repo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestResolveContext_SingleBranch_Explicit(t *testing.T) {
	repo := initTestRepo(t)

	cmd := exec.Command("git", "-C", repo, "config", "trellis.mode", "single-branch")
	require.NoError(t, cmd.Run())

	issuesDir := filepath.Join(repo, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), DefaultConfig("go")))

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, "single-branch", ctx.Mode)
}
