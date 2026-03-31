package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	repo := initTestRepo(t)
	c := git.New(repo)

	err := c.SetGitConfig("trellis.ops-worktree-path", "/some/path")
	require.NoError(t, err)

	val, err := c.ReadGitConfig("trellis.ops-worktree-path")
	require.NoError(t, err)
	assert.Equal(t, "/some/path", val)
}

func TestReadGitConfig_Unset(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := git.New(repo)

	_, err := c.ReadGitConfig("trellis.nonexistent")
	assert.Error(t, err)
}

func TestCommitWorktreeOp(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := git.New(repo)

	// Create orphan branch and worktree (using E2-001 methods)
	require.NoError(t, c.CreateOrphanBranch("_trellis"))
	worktreePath := filepath.Join(repo, ".trellis")
	require.NoError(t, c.AddWorktree("_trellis", worktreePath))

	// Write a file in the worktree
	opsDir := filepath.Join(worktreePath, ".issues", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, os.WriteFile(logFile, []byte("test op\n"), 0644))

	// CommitWorktreeOp is called on a client rooted at the worktree
	wc := git.New(worktreePath)
	err := wc.CommitWorktreeOp(".issues/ops/worker-abc.log", "ops: append claim for E2-001")
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
	c := git.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_trellis"))
	worktreePath := filepath.Join(repo, ".trellis")
	require.NoError(t, c.AddWorktree("_trellis", worktreePath))

	// Write and commit file first
	opsDir := filepath.Join(worktreePath, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, os.WriteFile(logFile, []byte("op1\n"), 0644))
	wc := git.New(worktreePath)
	require.NoError(t, wc.CommitWorktreeOp("ops/worker-abc.log", "first commit"))

	// Call again without changes — should not error
	err := wc.CommitWorktreeOp("ops/worker-abc.log", "second commit")
	assert.NoError(t, err)
}

func TestBranchMergedInto_Merged(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := git.New(repo)

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
	c := git.New(repo)

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

// TestConcurrentWorkerPush demonstrates what happens when two workers commit
// to their own log files on _trellis and both try to push to a shared remote.
// Worker A pushes first (fast-forward, succeeds). Worker B's push is rejected
// because the remote tip has moved — it must fetch+rebase before pushing.
func TestConcurrentWorkerPush_SecondPushRejected(t *testing.T) {
	t.Parallel()
	// Set up a bare remote repo that acts as the shared git server
	remote := t.TempDir()
	gitRun := func(dir string, args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v in %s: %s", args, dir, out)
		return strings.TrimSpace(string(out))
	}
	gitRun(remote, "init", "--bare")

	// Clone into two worker repos simulating two independent agents
	workerA := t.TempDir()
	workerB := t.TempDir()
	gitRun(workerA, "clone", remote, ".")
	gitRun(workerA, "config", "user.email", "a@test.com")
	gitRun(workerA, "config", "user.name", "Worker A")
	gitRun(workerA, "config", "commit.gpgsign", "false")
	gitRun(workerB, "clone", remote, ".")
	gitRun(workerB, "config", "user.email", "b@test.com")
	gitRun(workerB, "config", "user.name", "Worker B")
	gitRun(workerB, "config", "commit.gpgsign", "false")

	// Both workers create the _trellis orphan branch locally (simulating trls init)
	// and push it to the remote so both start from the same tip
	gitRun(workerA, "checkout", "--orphan", "_trellis")
	gitRun(workerA, "commit", "--allow-empty", "-m", "init: _trellis")
	gitRun(workerA, "push", "-u", "origin", "_trellis")
	gitRun(workerB, "fetch", "origin")
	gitRun(workerB, "checkout", "-b", "_trellis", "origin/_trellis")
	gitRun(workerB, "branch", "--set-upstream-to=origin/_trellis", "_trellis")

	// Worker A writes an op to its own log file and commits
	opsA := filepath.Join(workerA, "ops")
	require.NoError(t, os.MkdirAll(opsA, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(opsA, "worker-a.log"), []byte(`{"type":"claim"}`+"\n"), 0644))
	gitRun(workerA, "add", "ops/worker-a.log")
	gitRun(workerA, "commit", "-m", "ops: claim E3-001 by worker-a")

	// Worker B writes an op to its own log file and commits (also from the old tip)
	opsB := filepath.Join(workerB, "ops")
	require.NoError(t, os.MkdirAll(opsB, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(opsB, "worker-b.log"), []byte(`{"type":"claim"}`+"\n"), 0644))
	gitRun(workerB, "add", "ops/worker-b.log")
	gitRun(workerB, "commit", "-m", "ops: claim E3-001 by worker-b")

	// Worker A pushes successfully (fast-forward)
	gitRun(workerA, "push", "origin", "_trellis")

	// Worker B's push is rejected — remote tip has moved
	pushB := exec.Command("git", "-C", workerB, "push", "origin", "_trellis")
	out, err := pushB.CombinedOutput()
	require.Error(t, err, "expected worker B's push to be rejected, but it succeeded: %s", out)
	assert.Contains(t, string(out), "rejected", "expected rejection message: %s", out)

	// Worker B must fetch + rebase to make its push fast-forward
	gitRun(workerB, "fetch", "origin")
	gitRun(workerB, "rebase", "origin/_trellis")

	// Now worker B's push succeeds
	gitRun(workerB, "push", "origin", "_trellis")

	// Verify the remote has both workers' log files
	verify := t.TempDir()
	gitRun(verify, "clone", remote, ".")
	gitRun(verify, "checkout", "_trellis")
	_, errA := os.Stat(filepath.Join(verify, "ops", "worker-a.log"))
	_, errB := os.Stat(filepath.Join(verify, "ops", "worker-b.log"))
	assert.NoError(t, errA, "worker-a.log should be present on remote")
	assert.NoError(t, errB, "worker-b.log should be present on remote")
}

// setupTwoWorkerRepos creates a bare remote and two worker clones, both on _trellis branch.
// Returns (remote, workerA, workerB paths).
func setupTwoWorkerRepos(t *testing.T) (string, string, string) {
	t.Helper()
	remote := t.TempDir()
	gitRun := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v in %s: %s", args, dir, out)
	}
	gitRun(remote, "init", "--bare")

	workerA := t.TempDir()
	workerB := t.TempDir()
	gitRun(workerA, "clone", remote, ".")
	gitRun(workerA, "config", "user.email", "a@test.com")
	gitRun(workerA, "config", "user.name", "Worker A")
	gitRun(workerA, "config", "commit.gpgsign", "false")
	gitRun(workerB, "clone", remote, ".")
	gitRun(workerB, "config", "user.email", "b@test.com")
	gitRun(workerB, "config", "user.name", "Worker B")
	gitRun(workerB, "config", "commit.gpgsign", "false")

	gitRun(workerA, "checkout", "--orphan", "_trellis")
	gitRun(workerA, "commit", "--allow-empty", "-m", "init: _trellis")
	gitRun(workerA, "push", "-u", "origin", "_trellis")
	gitRun(workerB, "fetch", "origin")
	gitRun(workerB, "checkout", "-b", "_trellis", "origin/_trellis")
	gitRun(workerB, "branch", "--set-upstream-to=origin/_trellis", "_trellis")

	return remote, workerA, workerB
}

func TestPush_FastForward_Succeeds(t *testing.T) {
	t.Parallel()
	_, workerA, _ := setupTwoWorkerRepos(t)
	cA := git.New(workerA)

	// Worker A makes a commit then pushes with the new Push method
	opsA := filepath.Join(workerA, "ops")
	require.NoError(t, os.MkdirAll(opsA, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(opsA, "worker-a.log"), []byte("op1\n"), 0644))
	cmd := exec.Command("git", "-C", workerA, "add", "ops/worker-a.log")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", workerA, "commit", "-m", "ops: add log")
	require.NoError(t, cmd.Run())

	err := cA.Push("_trellis")
	assert.NoError(t, err)
}

func TestPush_Rejected_ReturnsError(t *testing.T) {
	t.Parallel()
	_, workerA, workerB := setupTwoWorkerRepos(t)

	// Worker A makes a commit and pushes
	opsA := filepath.Join(workerA, "ops")
	require.NoError(t, os.MkdirAll(opsA, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(opsA, "worker-a.log"), []byte("op1\n"), 0644))
	cmdA := exec.Command("git", "-C", workerA, "add", "ops/worker-a.log")
	require.NoError(t, cmdA.Run())
	cmdA = exec.Command("git", "-C", workerA, "commit", "-m", "ops: worker-a")
	require.NoError(t, cmdA.Run())
	cmdA = exec.Command("git", "-C", workerA, "push", "origin", "_trellis")
	require.NoError(t, cmdA.Run())

	// Worker B makes a commit from the old tip — push should be rejected
	cB := git.New(workerB)
	opsB := filepath.Join(workerB, "ops")
	require.NoError(t, os.MkdirAll(opsB, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(opsB, "worker-b.log"), []byte("op1\n"), 0644))
	cmdB := exec.Command("git", "-C", workerB, "add", "ops/worker-b.log")
	require.NoError(t, cmdB.Run())
	cmdB = exec.Command("git", "-C", workerB, "commit", "-m", "ops: worker-b")
	require.NoError(t, cmdB.Run())

	err := cB.Push("_trellis")
	assert.Error(t, err, "expected push to be rejected")
}

func TestFetchAndRebase_Then_Push_Succeeds(t *testing.T) {
	t.Parallel()
	_, workerA, workerB := setupTwoWorkerRepos(t)

	// Worker A pushes first
	opsA := filepath.Join(workerA, "ops")
	require.NoError(t, os.MkdirAll(opsA, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(opsA, "worker-a.log"), []byte("op1\n"), 0644))
	cmdA := exec.Command("git", "-C", workerA, "add", "ops/worker-a.log")
	require.NoError(t, cmdA.Run())
	cmdA = exec.Command("git", "-C", workerA, "commit", "-m", "ops: worker-a")
	require.NoError(t, cmdA.Run())
	cmdA = exec.Command("git", "-C", workerA, "push", "origin", "_trellis")
	require.NoError(t, cmdA.Run())

	// Worker B commits from old tip
	cB := git.New(workerB)
	opsB := filepath.Join(workerB, "ops")
	require.NoError(t, os.MkdirAll(opsB, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(opsB, "worker-b.log"), []byte("op1\n"), 0644))
	cmdB := exec.Command("git", "-C", workerB, "add", "ops/worker-b.log")
	require.NoError(t, cmdB.Run())
	cmdB = exec.Command("git", "-C", workerB, "commit", "-m", "ops: worker-b")
	require.NoError(t, cmdB.Run())

	// FetchAndRebase resolves the conflict, then push succeeds
	require.NoError(t, cB.FetchAndRebase("_trellis"))
	err := cB.Push("_trellis")
	assert.NoError(t, err)
}

func TestBranchMergedInto_NonexistentBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := git.New(repo)

	// Non-existent branch should return (false, nil) not an error
	merged, err := c.BranchMergedInto("feature/ghost", "main")
	assert.NoError(t, err)
	assert.False(t, merged)
}

func TestListFilesAtCommit(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := git.New(repo)

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
	c := git.New(repo)

	_, err := c.ListFilesAtCommit("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	assert.Error(t, err)
}

func TestShowFileAtCommit(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := git.New(repo)

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
	c := git.New(repo)

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
	c := git.New(repo)

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
	c := git.New(repo)

	_, err := c.LogBranch("no-such-branch", 10)
	assert.Error(t, err)
}
