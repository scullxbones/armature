package adapters_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
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
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "branch", "--list", "_armature")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "_armature")

	// Verify we are still on the original branch (not _armature)
	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
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
	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
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

func TestCreateBranchFrom_DoesNotUseTags(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// init repo and make an initial commit
	run := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	run("commit", "--allow-empty", "-m", "init")

	// Create a tag with the same name as the would-be branch
	run("tag", "task/fix-123")

	c := adapters.New(dir)
	err := c.CreateBranchFrom("task/fix-123", "HEAD")
	require.NoError(t, err, "CreateBranchFrom should succeed even when a tag of the same name exists")

	// Verify the branch was actually created (not just the tag)
	checkCmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--verify", "refs/heads/task/fix-123")
	assert.NoError(t, checkCmd.Run(), "refs/heads/task/fix-123 branch must exist after CreateBranchFrom")
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
	cmd := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "log", "--oneline", "-1")
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
	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(branchOut))

	// Create and merge a feature branch
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
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

	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(branchOut))

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
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
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Write two files and commit
	require.NoError(t, os.WriteFile(filepath.Join(repo, "alpha.txt"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "beta.txt"), []byte("b"), 0644))
	gitRun("add", "alpha.txt", "beta.txt")
	gitRun("commit", "-m", "add files")

	// Get the HEAD SHA
	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
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
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	content := []byte("hello world\n")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "hello.txt"), content, 0644))
	gitRun("add", "hello.txt")
	gitRun("commit", "-m", "add hello")

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
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

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
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
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
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

	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
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

func TestEnhanceGitLockfileError_AddsSandboxHint(t *testing.T) {
	t.Parallel()
	base := "git add foo: exit status 128"
	out := "fatal: Unable to create '/repo/.git/worktrees/-arm/index.lock': Read-only file system"
	got := adapters.EnhanceGitLockfileErrorForTest(base, out)
	assert.Contains(t, got, "sandbox blocked git lockfile writes")
}

func TestEnhanceGitLockfileError_NoHintForOtherErrors(t *testing.T) {
	t.Parallel()
	base := "git add foo: exit status 1"
	out := "fatal: pathspec 'foo' did not match any files"
	got := adapters.EnhanceGitLockfileErrorForTest(base, out)
	assert.Equal(t, base, got)
}

func TestIsGitContentionError(t *testing.T) {
	t.Parallel()
	assert.True(t, adapters.IsGitContentionErrorForTest("fatal: Unable to create '/repo/.git/index.lock': File exists"))
	assert.True(t, adapters.IsGitContentionErrorForTest("fatal: cannot lock ref 'HEAD': is at abc but expected def"))
	assert.False(t, adapters.IsGitContentionErrorForTest("fatal: pathspec 'foo' did not match any files"))
}

func TestCommitWorktreeOp_RetriesOnIndexLock(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_armature"))
	worktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, c.AddWorktree("_armature", worktreePath))

	opsDir := filepath.Join(worktreePath, ".armature", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, os.WriteFile(logFile, []byte("test op\n"), 0644))

	gitDirCmd := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "rev-parse", "--git-dir")
	gitDirOut, err := gitDirCmd.Output()
	require.NoError(t, err)
	gitDir := strings.TrimSpace(string(gitDirOut))
	lockPath := filepath.Join(gitDir, "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("lock"), 0644))

	var wg sync.WaitGroup
	wg.Go(func() {
		time.Sleep(120 * time.Millisecond)
		_ = os.Remove(lockPath) //nolint:errcheck // os.Remove in goroutine; t.Fatal not callable from goroutine
	})

	wc := adapters.New(worktreePath)
	err = wc.CommitWorktreeOp(".armature/ops/worker-abc.log", "ops: append claim for E2-001")
	wg.Wait()
	require.NoError(t, err)
}

func TestCommitWorktreeOp_ExhaustsContentionRetries(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_armature"))
	worktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, c.AddWorktree("_armature", worktreePath))

	opsDir := filepath.Join(worktreePath, ".armature", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, os.WriteFile(logFile, []byte("test op\n"), 0644))

	gitDirCmd := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "rev-parse", "--git-dir")
	gitDirOut, err := gitDirCmd.Output()
	require.NoError(t, err)
	gitDir := strings.TrimSpace(string(gitDirOut))
	lockPath := filepath.Join(gitDir, "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("lock"), 0644))
	t.Cleanup(func() {
		require.NoError(t, os.Remove(lockPath))
	})

	wc := adapters.New(worktreePath)
	err = wc.CommitWorktreeOp(".armature/ops/worker-abc.log", "ops: append claim for E2-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 contention retries")
}

func TestLogBranch_InvalidBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.LogBranch("no-such-branch", 10)
	assert.Error(t, err)
}

func TestListFilesAtCommit_EmptyTree(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaOut))

	files, err := c.ListFilesAtCommit(sha)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestFetchAndRebase_ReportsFetchError(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	err := c.FetchAndRebase("main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git fetch origin")
}

func TestFetchAndRebase_ReportsRebaseError(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	origin := filepath.Join(t.TempDir(), "origin.git")

	run := func(dir string, args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	run(t.TempDir(), "init", "--bare", origin)
	run(repo, "remote", "add", "origin", origin)
	run(repo, "push", "-u", "origin", "HEAD:main")

	c := adapters.New(repo)
	err := c.FetchAndRebase("feature/missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git rebase origin/feature/missing")
}

func TestHeadSHA_InvalidRepo(t *testing.T) {
	t.Parallel()
	c := adapters.New(filepath.Join(t.TempDir(), "no-such-repo"))

	_, err := c.HeadSHA()
	require.Error(t, err)
}

func TestCurrentBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	branch, err := c.CurrentBranch()
	require.NoError(t, err)
	assert.NotEmpty(t, branch)
}

func TestCommitMessage(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaOut))

	msg, err := c.CommitMessage(sha)
	require.NoError(t, err)
	assert.Contains(t, msg, "init")
}

func TestCommitMessage_InvalidSHA(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.CommitMessage("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	assert.Error(t, err)
}

func TestPush_ErrorOnNoRemote(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	branch := strings.TrimSpace(string(branchOut))

	err = c.Push(branch)
	assert.Error(t, err)
}

func TestRemoveWorktree(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_armature"))
	worktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, c.AddWorktree("_armature", worktreePath))

	// Verify the worktree exists
	_, err := os.Stat(worktreePath)
	require.NoError(t, err)

	// Remove the worktree
	err = c.RemoveWorktree(worktreePath)
	require.NoError(t, err)

	// Verify the worktree directory is gone
	_, err = os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(err))
}

func TestRemoveWorktree_NonExistent(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// Removing a non-existent worktree path should return an error
	err := c.RemoveWorktree(filepath.Join(repo, "no-such-worktree"))
	assert.Error(t, err)
}

func TestDiffFrom(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Commit a file at a known point
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("original\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "add file")

	// Get that SHA as the base
	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	// Make another commit
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("modified\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "modify file")

	diff, err := c.DiffFrom(baseSHA)
	require.NoError(t, err)
	assert.Contains(t, diff, "modified")
	assert.Contains(t, diff, "original")
}

func TestDiffFrom_InvalidSHA(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.DiffFrom("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	assert.Error(t, err)
}

func TestDiffNameOnly(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Commit two files at the base
	require.NoError(t, os.WriteFile(filepath.Join(repo, "alpha.txt"), []byte("a\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "beta.txt"), []byte("b\n"), 0644))
	gitRun("add", "alpha.txt", "beta.txt")
	gitRun("commit", "-m", "add files")

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	// Modify only alpha
	require.NoError(t, os.WriteFile(filepath.Join(repo, "alpha.txt"), []byte("changed\n"), 0644))
	gitRun("add", "alpha.txt")
	gitRun("commit", "-m", "change alpha")

	names, err := c.DiffNameOnly(baseSHA)
	require.NoError(t, err)
	assert.Contains(t, names, "alpha.txt")
	assert.NotContains(t, names, "beta.txt")
}

func TestDiffNameOnly_InvalidSHA(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.DiffNameOnly("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	assert.Error(t, err)
}

func TestResetHard(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Commit a file
	require.NoError(t, os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0644))
	gitRun("add", "base.txt")
	gitRun("commit", "-m", "base commit")

	// Get base SHA
	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	// Make a second commit
	require.NoError(t, os.WriteFile(filepath.Join(repo, "extra.txt"), []byte("extra\n"), 0644))
	gitRun("add", "extra.txt")
	gitRun("commit", "-m", "extra commit")

	// Reset back to base
	err = c.ResetHard(baseSHA)
	require.NoError(t, err)

	// extra.txt should no longer be tracked
	headCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, baseSHA, strings.TrimSpace(string(headOut)))
}

func TestResetHard_InvalidRef(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	err := c.ResetHard("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	assert.Error(t, err)
}

func TestApplyPatch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Commit a base file
	require.NoError(t, os.WriteFile(filepath.Join(repo, "patch_target.txt"), []byte("line1\nline2\n"), 0644))
	gitRun("add", "patch_target.txt")
	gitRun("commit", "-m", "base")

	// Build a valid unified diff patch
	patch := `diff --git a/patch_target.txt b/patch_target.txt
index 0000000..1111111 100644
--- a/patch_target.txt
+++ b/patch_target.txt
@@ -1,2 +1,3 @@
 line1
 line2
+line3
`

	err := c.ApplyPatch([]byte(patch))
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(repo, "patch_target.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "line3")
}

func TestApplyPatch_InvalidPatch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	err := c.ApplyPatch([]byte("this is not a valid patch\n"))
	assert.Error(t, err)
}

func TestAddAll(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// Write a file but don't stage it
	require.NoError(t, os.WriteFile(filepath.Join(repo, "new_file.txt"), []byte("content\n"), 0644))

	err := c.AddAll()
	require.NoError(t, err)

	// Verify the file is staged
	statusCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "diff", "--cached", "--name-only")
	out, err := statusCmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "new_file.txt")
}

func TestAddPathsStagesOnlySelectedPaths(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("selected\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "artifact.txt"), []byte("artifact\n"), 0644))

	err := c.AddPaths([]string{"selected.txt"})
	require.NoError(t, err)

	statusCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "diff", "--cached", "--name-only")
	out, err := statusCmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "selected.txt")
	assert.NotContains(t, string(out), "artifact.txt")
}

func TestCommitWithMessage(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Stage a file
	require.NoError(t, os.WriteFile(filepath.Join(repo, "commit_me.txt"), []byte("data\n"), 0644))
	gitRun("add", "commit_me.txt")

	err := c.CommitWithMessage("test: my commit message")
	require.NoError(t, err)

	// Verify the commit message
	logCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "log", "-1", "--pretty=%s")
	logOut, err := logCmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(logOut), "my commit message")
}

func TestCommitWithMessage_NothingStaged(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// No staged changes — should return an error
	err := c.CommitWithMessage("test: empty commit")
	assert.Error(t, err)
}

func TestCreateBranchFrom(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Create a commit on main so we have something to branch from
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "initial commit")

	// Get current branch (main or master)
	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(branchOut))

	// Create a branch from main
	newBranch := "feature/test-branch"
	err = c.CreateBranchFrom(newBranch, mainBranch)
	require.NoError(t, err)

	// Verify branch exists
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "branch", "--list", newBranch)
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), newBranch)

	// Verify the new branch contains the commit from main
	// by checking out the branch and verifying the file exists
	gitRun("checkout", newBranch)
	_, err = os.Stat(filepath.Join(repo, "file.txt"))
	require.NoError(t, err, "file should exist in the new branch (inherited from main)")
}

func TestCreateBranchFrom_Idempotent(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Create a commit on main
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "initial commit")

	// Get current branch
	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(branchOut))

	newBranch := "feature/test-branch"

	// First call creates the branch
	err = c.CreateBranchFrom(newBranch, mainBranch)
	require.NoError(t, err)

	// Second call should not error (idempotent)
	err = c.CreateBranchFrom(newBranch, mainBranch)
	assert.NoError(t, err)

	// Verify branch still exists and is correct
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "branch", "--list", newBranch)
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), newBranch)
}

func TestResolveRevision(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Get HEAD SHA for comparison
	headCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	expectedSHA := strings.TrimSpace(string(headOut))

	// Resolve HEAD
	sha, err := c.ResolveRevision("HEAD")
	require.NoError(t, err)
	assert.Equal(t, expectedSHA, sha)

	// Create a tag and resolve it
	gitRun("tag", "v1.0")
	tagSHA, err := c.ResolveRevision("v1.0")
	require.NoError(t, err)
	assert.Equal(t, expectedSHA, tagSHA)
}

func TestResolveRevision_InvalidRevision(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.ResolveRevision("nonexistent-ref")
	assert.Error(t, err)
}

func TestDiffRange(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Get the initial commit SHA
	initCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	initOut, err := initCmd.Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(initOut))

	// Create a new commit
	require.NoError(t, os.WriteFile(filepath.Join(repo, "test.txt"), []byte("hello\n"), 0644))
	gitRun("add", "test.txt")
	gitRun("commit", "-m", "add test file")

	// Get the new commit SHA
	newCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	newOut, err := newCmd.Output()
	require.NoError(t, err)
	headSHA := strings.TrimSpace(string(newOut))

	// Diff the range
	diff, err := c.DiffRange(baseSHA, headSHA)
	require.NoError(t, err)
	assert.Contains(t, diff, "test.txt")
	assert.Contains(t, diff, "hello")
}

func TestDiffNameOnlyRange(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Get the initial commit SHA
	initCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	initOut, err := initCmd.Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(initOut))

	// Create new commits with multiple files
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file1.txt"), []byte("a\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file2.txt"), []byte("b\n"), 0644))
	gitRun("add", "file1.txt", "file2.txt")
	gitRun("commit", "-m", "add files")

	// Get the new commit SHA
	newCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	newOut, err := newCmd.Output()
	require.NoError(t, err)
	headSHA := strings.TrimSpace(string(newOut))

	// Get the name-only diff
	files, err := c.DiffNameOnlyRange(baseSHA, headSHA)
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Contains(t, files, "file1.txt")
	assert.Contains(t, files, "file2.txt")
}

func TestDiffNameOnlyRange_NoChanges(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// Get HEAD SHA
	headCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(headOut))

	// Diff the same commit (no changes)
	files, err := c.DiffNameOnlyRange(sha, sha)
	require.NoError(t, err)
	assert.Empty(t, files)
}
