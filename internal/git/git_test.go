package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/scullxbones/trellis/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("init")
	gitRun("config", "user.email", "test@test.com")
	gitRun("config", "user.name", "Test")
	gitRun("config", "commit.gpgsign", "false")
	gitRun("commit", "--allow-empty", "-m", "init")
	return dir
}

func TestCreateOrphanBranch(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	err := c.CreateOrphanBranch("_trellis")
	require.NoError(t, err)

	// Verify branch exists
	cmd := exec.Command("git", "-C", repo, "branch", "--list", "_trellis")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "_trellis")

	// Verify we are still on the original branch (not _trellis)
	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	assert.NotEqual(t, "_trellis\n", string(branchOut))
}

func TestCreateOrphanBranch_Idempotent(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_trellis"))
	// Second call should not error; branch already exists so it returns nil immediately
	err := c.CreateOrphanBranch("_trellis")
	assert.NoError(t, err)

	// Still on original branch
	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	assert.NotEqual(t, "_trellis\n", string(branchOut))
}

func TestAddWorktree(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_trellis"))

	worktreePath := filepath.Join(repo, ".trellis")
	err := c.AddWorktree("_trellis", worktreePath)
	require.NoError(t, err)

	// Verify worktree directory exists
	info, err := os.Stat(worktreePath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSetAndReadGitConfig(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	err := c.SetGitConfig("trellis.ops-worktree-path", "/some/path")
	require.NoError(t, err)

	val, err := c.ReadGitConfig("trellis.ops-worktree-path")
	require.NoError(t, err)
	assert.Equal(t, "/some/path", val)
}

func TestReadGitConfig_Unset(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	_, err := c.ReadGitConfig("trellis.nonexistent")
	assert.Error(t, err)
}
