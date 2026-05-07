package adapters_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
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
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	err := c.CreateOrphanBranch("_armature")
	require.NoError(t, err)

	// Verify branch exists
	cmd := exec.Command("git", "-C", repo, "branch", "--list", "_armature")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "_armature")

	// Verify we are still on the original branch (not _armature)
	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	assert.NotEqual(t, "_armature\n", string(branchOut))
}

func TestCreateOrphanBranch_Idempotent(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_armature"))
	// Second call should not error; branch already exists so it returns nil immediately
	err := c.CreateOrphanBranch("_armature")
	assert.NoError(t, err)

	// Still on original branch
	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	assert.NotEqual(t, "_armature\n", string(branchOut))
}

func TestAddWorktree(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_armature"))

	worktreePath := filepath.Join(repo, ".arm")
	err := c.AddWorktree("_armature", worktreePath)
	require.NoError(t, err)

	// Verify worktree directory exists
	info, err := os.Stat(worktreePath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSetAndReadGitConfig(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	err := c.SetGitConfig("armature.ops-worktree-path", "/some/path")
	require.NoError(t, err)

	val, err := c.ReadGitConfig("armature.ops-worktree-path")
	require.NoError(t, err)
	assert.Equal(t, "/some/path", val)
}

func TestReadGitConfig_Unset(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.ReadGitConfig("armature.nonexistent")
	assert.Error(t, err)
}

func TestCommitWorktreeOp(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// Create orphan branch and worktree (using E2-001 methods)
	require.NoError(t, c.CreateOrphanBranch("_armature"))
	worktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, c.AddWorktree("_armature", worktreePath))

	// Write a file in the worktree
	opsDir := filepath.Join(worktreePath, ".armature", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, os.WriteFile(logFile, []byte("test op\n"), 0644))

	// CommitWorktreeOp is called on a client rooted at the worktree
	wc := adapters.New(worktreePath)
	err := wc.CommitWorktreeOp(".armature/ops/worker-abc.log", "ops: append claim for E2-001")
	require.NoError(t, err)

	// Verify commit exists in the worktree branch
	cmd := exec.Command("git", "-C", worktreePath, "log", "--oneline", "-1")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "ops: append")
}

func TestCommitWorktreeOp_NoChanges_IsNoop(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_armature"))
	worktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, c.AddWorktree("_armature", worktreePath))

	// Write and commit file first
	opsDir := filepath.Join(worktreePath, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, os.WriteFile(logFile, []byte("op1\n"), 0644))
	wc := adapters.New(worktreePath)
	require.NoError(t, wc.CommitWorktreeOp("ops/worker-abc.log", "first commit"))

	// Call again without changes — should not error
	err := wc.CommitWorktreeOp("ops/worker-abc.log", "second commit")
	assert.NoError(t, err)
}

func TestBranchMergedInto_Merged(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// Detect what branch we're on
	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(branchOut))

	// Create and merge a feature branch
	gitRun := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("checkout", "-b", "feature/my-work")
	gitRun("commit", "--allow-empty", "-m", "feat: work")
	gitRun("checkout", mainBranch)
	gitRun("merge", "--no-ff", "feature/my-work", "-m", "Merge feature/my-work")

	merged, err := c.BranchMergedInto("feature/my-work", mainBranch)
	require.NoError(t, err)
	assert.True(t, merged)
}

func TestBranchMergedInto_NotMerged(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(branchOut))

	gitRun := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("checkout", "-b", "feature/unmerged")
	gitRun("commit", "--allow-empty", "-m", "wip")
	gitRun("checkout", mainBranch)

	merged, err := c.BranchMergedInto("feature/unmerged", mainBranch)
	require.NoError(t, err)
	assert.False(t, merged)
}

func TestBranchMergedInto_NonexistentBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// Non-existent branch should return (false, nil) not an error
	merged, err := c.BranchMergedInto("feature/ghost", "main")
	assert.NoError(t, err)
	assert.False(t, merged)
}

func TestListFilesAtCommit(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Write two files and commit
	require.NoError(t, os.WriteFile(filepath.Join(repo, "alpha.txt"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "beta.txt"), []byte("b"), 0644))
	gitRun("add", "alpha.txt", "beta.txt")
	gitRun("commit", "-m", "add files")

	// Get the HEAD SHA
	shaCmd := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaOut))

	files, err := c.ListFilesAtCommit(sha)
	require.NoError(t, err)
	assert.Contains(t, files, "alpha.txt")
	assert.Contains(t, files, "beta.txt")
}

func TestListFilesAtCommit_InvalidSHA(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.ListFilesAtCommit("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	assert.Error(t, err)
}

func TestShowFileAtCommit(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	content := []byte("hello world\n")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "hello.txt"), content, 0644))
	gitRun("add", "hello.txt")
	gitRun("commit", "-m", "add hello")

	shaCmd := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaOut))

	got, err := c.ShowFileAtCommit(sha, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestShowFileAtCommit_MissingFile(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	shaCmd := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaOut))

	_, err = c.ShowFileAtCommit(sha, "nonexistent.txt")
	assert.Error(t, err)
}

func TestLogBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Add two more commits
	require.NoError(t, os.WriteFile(filepath.Join(repo, "f1.txt"), []byte("1"), 0644))
	gitRun("add", "f1.txt")
	gitRun("commit", "-m", "second commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "f2.txt"), []byte("2"), 0644))
	gitRun("add", "f2.txt")
	gitRun("commit", "-m", "third commit")

	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	branch := strings.TrimSpace(string(branchOut))

	entries, err := c.LogBranch(branch, 2)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "third commit", entries[0].Subject)
	assert.Equal(t, "second commit", entries[1].Subject)
	assert.NotEmpty(t, entries[0].SHA)
	assert.NotEmpty(t, entries[0].Author)
	assert.NotEmpty(t, entries[0].Date)
}

func TestLogBranch_InvalidBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.LogBranch("no-such-branch", 10)
	assert.Error(t, err)
}
