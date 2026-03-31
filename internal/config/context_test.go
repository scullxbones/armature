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
	t.Parallel()
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

func TestResolveContext_DualBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	// Simulate a dual-branch setup: create the worktree dir with .issues/ inside
	worktreePath := filepath.Join(repo, ".trellis")
	issuesDir := filepath.Join(worktreePath, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	cfg.Mode = "dual-branch"
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	// Set git config keys
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	runGit("config", "trellis.mode", "dual-branch")
	runGit("config", "trellis.ops-worktree-path", worktreePath)

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, "dual-branch", ctx.Mode)
	assert.Equal(t, issuesDir, ctx.IssuesDir)
	assert.Equal(t, repo, ctx.RepoPath)
}

func TestResolveContext_DualBranch_MissingWorktreePath(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	// Set dual-branch mode but do NOT set ops-worktree-path
	cmd := exec.Command("git", "-C", repo, "config", "trellis.mode", "dual-branch")
	require.NoError(t, cmd.Run())

	_, err := ResolveContext(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trellis.ops-worktree-path")
}

func TestResolveContext_SingleBranch_Explicit(t *testing.T) {
	t.Parallel()
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

func TestResolveContext_DualBranch_WorktreePath(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	worktreePath := filepath.Join(repo, ".trellis")
	issuesDir := filepath.Join(worktreePath, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	cfg.Mode = "dual-branch"
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	runGit("config", "trellis.mode", "dual-branch")
	runGit("config", "trellis.ops-worktree-path", worktreePath)

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, worktreePath, ctx.WorktreePath)
}

func TestResolveContext_SingleBranch_WorktreePath_Empty(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	issuesDir := filepath.Join(repo, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), DefaultConfig("go")))

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, "", ctx.WorktreePath)
}

func TestContextStateDir(t *testing.T) {
	t.Parallel()
	ctx := &Context{
		StateDir: "/tmp/trellis-state",
	}
	assert.Equal(t, "/tmp/trellis-state", ctx.StateDir)
}
