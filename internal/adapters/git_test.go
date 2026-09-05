package adapters_test

import (
	"context"
	"fmt"
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

func installGitCommitFailureWrapper(t *testing.T, repo string) string {
	t.Helper()

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "git")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)

	script := `#!/bin/sh
real_git=%q
repo=%q
cmd=""
target=""
skip=""
for arg in "$@"; do
  if [ -n "$skip" ]; then
    if [ "$skip" = "-C" ]; then
      target="$arg"
    fi
    skip=""
    continue
  fi
  case "$arg" in
    -C|-c)
      skip="$arg"
      continue
      ;;
    commit)
      cmd="$arg"
      ;;
  esac
done
if [ "$cmd" = "commit" ] && [ "$target" = "$repo" ]; then
  exit 1
fi
exec "$real_git" "$@"
`
	require.NoError(t, os.WriteFile(wrapperPath, fmt.Appendf(nil, script, realGit, repo), 0o755))
	return wrapperDir
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

func TestCreateOrphanBranch_DirtyWorkingTree_Fails(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	// Capture the current branch name (might be master or main)
	currentBranchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	require.NoError(t, err)
	originalBranch := strings.TrimSpace(string(currentBranchOut))

	// Create a tracked file and commit it
	testFile := filepath.Join(repo, "tracked.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("original content"), 0644))

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("add", "tracked.txt")
	gitRun("commit", "-m", "add tracked file")

	// Modify the tracked file without committing (dirty working tree)
	require.NoError(t, os.WriteFile(testFile, []byte("modified content"), 0644))

	// Verify the file is modified and not staged
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "status", "--porcelain")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "tracked.txt", "file should show as modified")

	// Now try to create orphan branch - this should fail with dirty working tree error
	c := adapters.New(repo)
	err = c.CreateOrphanBranch("_armature")

	// Must fail with a clear error about dirty working tree
	require.Error(t, err, "CreateOrphanBranch should fail when working tree is dirty")
	assert.Contains(t, err.Error(), "dirty", "error should mention dirty working tree")

	// Most importantly: verify that the uncommitted change is still there (not destroyed)
	content, readErr := os.ReadFile(testFile)
	require.NoError(t, readErr, "file should still exist")
	assert.Equal(t, "modified content", string(content), "uncommitted changes must be preserved")

	// Verify we're still on the original branch
	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, originalBranch, strings.TrimSpace(string(branchOut)), "should still be on original branch after error")
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

// TestCommitWorktreeOp_AppendMetaDirNotDirty_REQ_TOPTIER_S4_PRFIX proves that
// after an AppendLog.Append call (which creates lock/pending-marker sidecars
// under .arm-append-meta/) followed by CommitWorktreeOp, the ops worktree is
// not left dirty by those untracked sidecar files. The .armature/.gitignore
// shipped in this repo must ignore .arm-append-meta/ for this to hold.
func TestCommitWorktreeOp_AppendMetaDirNotDirty_REQ_TOPTIER_S4_PRFIX(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_armature"))
	worktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, c.AddWorktree("_armature", worktreePath))

	opsDir := filepath.Join(worktreePath, ".armature", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	// Ship the same .gitignore content `arm bootstrap` writes to the ops
	// worktree root, so this test tracks the real shipped behavior instead of
	// a hand-maintained on-disk copy (which isn't committed to the repo).
	gitignoreDst := filepath.Join(worktreePath, ".armature", ".gitignore")
	require.NoError(t, os.WriteFile(gitignoreDst, []byte(adapters.OpsGitignore), 0644))

	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, adapters.NewAppendLog(logFile).Append([]byte("{\"op\":1}\n")))

	// Sidecar dir must exist as a side effect of AppendLog.Append.
	_, statErr := os.Stat(filepath.Join(opsDir, ".arm-append-meta"))
	require.NoError(t, statErr, "expected .arm-append-meta sidecar dir to be created")

	wc := adapters.New(worktreePath)
	require.NoError(t, wc.CommitWorktreeOp(".armature/ops/worker-abc.log", "ops: append claim for E2-001"))

	cmd := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "status", "--porcelain")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.NotContains(t, string(out), ".arm-append-meta", "ops worktree left dirty by append-meta sidecar files: %s", out)
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

func TestMergeBase_REQ_LNGHZN_S4_T2(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "base commit")
	shaOut, err := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	gitRun("checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("feature\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "feature commit")

	got, err := c.MergeBase("feature", "master")
	if err != nil {
		// Default branch name may be "main" in some git configs.
		got, err = c.MergeBase("feature", "main")
	}
	require.NoError(t, err)
	assert.Equal(t, baseSHA, got)
}

func TestMergeBase_InvalidRevision_REQ_LNGHZN_S4_T2(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	_, err := c.MergeBase("does-not-exist", "HEAD")
	assert.Error(t, err)
}

func TestLogRange_ExcludesBaseAndEarlier_REQ_LNGHZN_S4_T2(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("v0\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "base commit")
	shaOut, err := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("v1\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "commit after base")

	entries, err := c.LogRange(baseSHA, "HEAD")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "commit after base", entries[0].Subject)
}

// TestLogRange_ReportsParentCount_REQ_LNGHZN_S4 verifies LogEntry exposes
// the number of parent commits (from git log's %P), so callers can
// distinguish an ordinary single-parent commit from a genuine merge commit
// without a second git invocation.
func TestLogRange_ReportsParentCount_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("v0\n"), 0644))
	gitRun("add", "file.txt")
	gitRun("commit", "-m", "base commit")
	shaOut, err := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	// A branch with a single-parent commit.
	gitRun("checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("f\n"), 0644))
	gitRun("add", "feature.txt")
	gitRun("commit", "-m", "single parent commit")

	// Merge feature back into a second branch off base to create a genuine
	// two-parent merge commit.
	gitRun("checkout", "-b", "other", baseSHA)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "other.txt"), []byte("o\n"), 0644))
	gitRun("add", "other.txt")
	gitRun("commit", "-m", "other branch commit")
	gitRun("merge", "--no-ff", "feature", "-m", "merge: LNGHZN-S4 merge feature into other")

	entries, err := c.LogRange(baseSHA, "HEAD")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 3)

	var mergeEntry, singleEntry *adapters.LogEntry
	for i := range entries {
		switch entries[i].Subject {
		case "merge: LNGHZN-S4 merge feature into other":
			mergeEntry = &entries[i]
		case "single parent commit":
			singleEntry = &entries[i]
		}
	}
	require.NotNil(t, mergeEntry, "merge commit entry not found")
	require.NotNil(t, singleEntry, "single-parent commit entry not found")
	assert.GreaterOrEqual(t, mergeEntry.ParentCount(), 2, "merge commit should report 2+ parents")
	assert.Equal(t, 1, singleEntry.ParentCount(), "ordinary commit should report exactly 1 parent")
}

func TestDiffNameStatus_DetectsRenameWithBothPaths(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	content := strings.Repeat("line of content\n", 20)
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "outside"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "outside", "a.go"), []byte(content), 0644))
	gitRun("add", "outside/a.go")
	gitRun("commit", "-m", "add outside file")

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	require.NoError(t, os.MkdirAll(filepath.Join(repo, "inside"), 0755))
	gitRun("mv", "outside/a.go", "inside/a.go")
	gitRun("commit", "-m", "rename outside to inside")

	entries, err := c.DiffNameStatus(baseSHA)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, strings.HasPrefix(entries[0].Status, "R"), "expected rename status, got %q", entries[0].Status)
	assert.Equal(t, "outside/a.go", entries[0].OldPath)
	assert.Equal(t, "inside/a.go", entries[0].Path)
}

func TestDiffNameStatus_NonRenameChangesHaveNoOldPath(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	require.NoError(t, os.WriteFile(filepath.Join(repo, "alpha.txt"), []byte("a\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "beta.txt"), []byte("b\n"), 0644))
	gitRun("add", "alpha.txt", "beta.txt")
	gitRun("commit", "-m", "add files")

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "alpha.txt"), []byte("changed\n"), 0644))
	gitRun("add", "alpha.txt")
	gitRun("rm", "beta.txt")
	gitRun("commit", "-m", "modify alpha, delete beta")

	entries, err := c.DiffNameStatus(baseSHA)
	require.NoError(t, err)
	byPath := map[string]adapters.DiffStatusEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	require.Contains(t, byPath, "alpha.txt")
	require.Contains(t, byPath, "beta.txt")
	assert.Empty(t, byPath["alpha.txt"].OldPath)
	assert.Empty(t, byPath["beta.txt"].OldPath)
	assert.Equal(t, "M", byPath["alpha.txt"].Status)
	assert.Equal(t, "D", byPath["beta.txt"].Status)
}

// TestCommitChangedFiles_DetectsRenameAsSinglePath verifies that a pure
// rename commit is reported by CommitChangedFiles as its single (new) path,
// consistent with DiffNameStatus's rename detection (-M), rather than as a
// delete-of-old-path plus add-of-new-path pair. Before this fix,
// CommitChangedFiles ran `git diff-tree` without -M, so a pure rename showed
// up as two unrelated paths instead of one, inconsistent with DiffNameStatus.
func TestCommitChangedFiles_DetectsRenameAsSinglePath(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	content := strings.Repeat("line of content\n", 20)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte(content), 0644))
	gitRun("add", "a.txt")
	gitRun("commit", "-m", "add a.txt")

	gitRun("mv", "a.txt", "b.txt")
	gitRun("commit", "-m", "rename a.txt to b.txt")

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaOut))

	files, err := c.CommitChangedFiles(sha)
	require.NoError(t, err)
	assert.Equal(t, []string{"b.txt"}, files, "pure rename should report only the new path, not a delete+add pair")
}

// TestDiffNameStatus_HandlesNonASCIIPath_REQ_LNGHZN_S4 verifies that
// DiffNameStatus reports the literal path for a filename containing
// non-ASCII characters, rather than git's default octal-escaped quoted form
// (e.g. "caf\303\251.go"). Without -z, `git diff --name-status` quotes such
// paths, which breaks downstream scope-containment comparisons (e.g.
// claim.IsWithinScope) against the literal path recorded in the issue's
// declared scope.
func TestDiffNameStatus_HandlesNonASCIIPath_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	shaCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(shaOut))

	nonASCIIPath := "café.go"
	require.NoError(t, os.WriteFile(filepath.Join(repo, nonASCIIPath), []byte("package main\n"), 0644))
	gitRun("add", nonASCIIPath)
	gitRun("commit", "-m", "add non-ascii file")

	entries, err := c.DiffNameStatus(baseSHA)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, nonASCIIPath, entries[0].Path, "path must be the literal non-ASCII name, not git's octal-escaped quoted form")
	assert.Equal(t, "A", entries[0].Status)
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

func TestCreateOrphanBranch_WithRemoteBranch(t *testing.T) {
	t.Parallel()

	// Create a bare repo that acts as shared origin
	originDir := t.TempDir()

	gitRun := func(dir string, args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	gitRun(originDir, "init", "--bare")

	// Create a temp clone to push initial content to origin
	tempDir := t.TempDir()
	gitRun(tempDir, "init")
	gitRun(tempDir, "config", "user.email", "test@test.com")
	gitRun(tempDir, "config", "user.name", "Test")
	gitRun(tempDir, "config", "commit.gpgsign", "false")
	gitRun(tempDir, "remote", "add", "origin", originDir)
	gitRun(tempDir, "commit", "--allow-empty", "-m", "init")
	gitRun(tempDir, "branch", "-M", "main")
	gitRun(tempDir, "push", "-u", "origin", "main")

	// Create and push _armature branch from main
	gitRun(tempDir, "checkout", "-b", "_armature")
	gitRun(tempDir, "commit", "--allow-empty", "-m", "init armature")
	gitRun(tempDir, "push", "-u", "origin", "_armature")

	// Create a fresh clone (will have origin/_armature but no local _armature)
	cloneDir := t.TempDir()
	gitRun(cloneDir, "clone", originDir, "cloned")
	clonePath := filepath.Join(cloneDir, "cloned")
	gitRun(clonePath, "config", "user.email", "test@test.com")
	gitRun(clonePath, "config", "user.name", "Test")
	gitRun(clonePath, "config", "commit.gpgsign", "false")

	// Verify preconditions: origin/_armature exists but local _armature doesn't
	checkBranch := func(ref string) bool {
		cmd := exec.CommandContext(context.Background(), "git", "-C", clonePath, "rev-parse", "--verify", ref)
		return cmd.Run() == nil
	}
	require.True(t, checkBranch("origin/_armature"), "origin/_armature should exist")
	require.False(t, checkBranch("_armature"), "local _armature should not exist yet")

	// Call CreateOrphanBranch
	c := adapters.New(clonePath)
	err := c.CreateOrphanBranch("_armature")
	require.NoError(t, err)

	// Verify _armature now exists locally
	require.True(t, checkBranch("_armature"), "local _armature should exist after CreateOrphanBranch")

	// Verify it's a tracking branch (same commit as origin/_armature)
	getCommit := func(ref string) string {
		cmd := exec.CommandContext(context.Background(), "git", "-C", clonePath, "rev-parse", ref)
		out, err := cmd.Output()
		require.NoError(t, err)
		return strings.TrimSpace(string(out))
	}
	originCommit := getCommit("origin/_armature")
	localCommit := getCommit("_armature")
	require.Equal(t, originCommit, localCommit, "local _armature should have same commit as origin/_armature")
}

func TestCreateOrphanBranch_RestoresOnCommitFailure(t *testing.T) {
	repo := initTestRepo(t)

	// Get current branch
	getCurrentBranch := func() string {
		cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
		out, err := cmd.Output()
		require.NoError(t, err)
		return strings.TrimSpace(string(out))
	}
	originalBranch := getCurrentBranch()

	// Track a file so we can verify the working tree is actually restored,
	// not just the branch name.
	trackedPath := filepath.Join(repo, "tracked.txt")
	const trackedContent = "original content\n"
	require.NoError(t, os.WriteFile(trackedPath, []byte(trackedContent), 0o644))
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("add", "tracked.txt")
	gitRun("commit", "-m", "add tracked file")

	wrapperDir := installGitCommitFailureWrapper(t, repo)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Try to create orphan branch (commit will fail due to pre-commit hook)
	c := adapters.New(repo)
	err := c.CreateOrphanBranch("_armature")
	require.Error(t, err, "CreateOrphanBranch should fail when commit fails")

	// Verify we're back on the original branch (not on the broken orphan)
	currentBranch := getCurrentBranch()
	require.Equal(t, originalBranch, currentBranch, "should be back on original branch after CreateOrphanBranch error")

	// Verify the working tree was actually restored, not just the branch name
	restoredContent, readErr := os.ReadFile(trackedPath)
	require.NoError(t, readErr, "tracked file should still exist after restore")
	require.Equal(t, trackedContent, string(restoredContent), "tracked file content should survive the restore")
}

func TestCreateOrphanBranch_RestoresDetachedHEADOnCommitFailure(t *testing.T) {
	repo := initTestRepo(t)

	// Get the current commit SHA before detaching
	headSHACmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	headSHAOut, err := headSHACmd.Output()
	require.NoError(t, err)
	originalSHA := strings.TrimSpace(string(headSHAOut))

	// Detach HEAD at the current commit
	detachCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "checkout", "--detach")
	_, err = detachCmd.CombinedOutput()
	require.NoError(t, err)

	// Verify we're in detached HEAD state
	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	require.Equal(t, "HEAD", strings.TrimSpace(string(branchOut)), "should be in detached HEAD state")

	wrapperDir := installGitCommitFailureWrapper(t, repo)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Call CreateOrphanBranch; the commit on the orphan branch should fail due
	// to the pre-commit hook, and CreateOrphanBranch should restore HEAD back
	// to the original detached SHA (not leave it on the broken orphan branch).
	c := adapters.New(repo)
	err = c.CreateOrphanBranch("_armature")
	require.Error(t, err, "CreateOrphanBranch should fail when commit fails")

	// Verify we're back in detached HEAD state (not on _armature)
	currentBranchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "HEAD", strings.TrimSpace(string(currentBranchOut)), "should be back in detached HEAD state")

	// Verify we're at the original commit SHA
	currentSHACmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	currentSHAOut, err := currentSHACmd.Output()
	require.NoError(t, err)
	currentSHA := strings.TrimSpace(string(currentSHAOut))
	assert.Equal(t, originalSHA, currentSHA, "HEAD should be restored to the original detached commit SHA")
}

func TestCreateOrphanBranch_SingleBranchClone(t *testing.T) {
	t.Parallel()

	// Create a bare repo that acts as shared origin
	originDir := t.TempDir()

	gitRun := func(dir string, args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	gitRun(originDir, "init", "--bare")

	// Create a temp clone to push initial content to origin
	tempDir := t.TempDir()
	gitRun(tempDir, "init")
	gitRun(tempDir, "config", "user.email", "test@test.com")
	gitRun(tempDir, "config", "user.name", "Test")
	gitRun(tempDir, "config", "commit.gpgsign", "false")
	gitRun(tempDir, "remote", "add", "origin", originDir)
	gitRun(tempDir, "commit", "--allow-empty", "-m", "init")
	gitRun(tempDir, "branch", "-M", "main")
	gitRun(tempDir, "push", "-u", "origin", "main")

	// Create and push _armature branch from main
	gitRun(tempDir, "checkout", "-b", "_armature")
	gitRun(tempDir, "commit", "--allow-empty", "-m", "init armature")
	gitRun(tempDir, "push", "-u", "origin", "_armature")

	// Get the commit SHA of the _armature branch on origin
	armatureCmd := exec.CommandContext(context.Background(), "git", "-C", tempDir, "rev-parse", "origin/_armature")
	armatureOut, err := armatureCmd.Output()
	require.NoError(t, err)
	expectedArmatureSHA := strings.TrimSpace(string(armatureOut))

	// Create a clone with --single-branch --branch main
	// This will NOT create origin/_armature remote-tracking ref even though it exists on the remote
	cloneDir := t.TempDir()
	gitRun(cloneDir, "clone", "--single-branch", "--branch", "main", originDir, "cloned")
	clonePath := filepath.Join(cloneDir, "cloned")
	gitRun(clonePath, "config", "user.email", "test@test.com")
	gitRun(clonePath, "config", "user.name", "Test")
	gitRun(clonePath, "config", "commit.gpgsign", "false")

	// Verify preconditions: origin/_armature doesn't exist (because of single-branch),
	// but the remote branch does exist on the server
	checkBranch := func(ref string) bool {
		cmd := exec.CommandContext(context.Background(), "git", "-C", clonePath, "rev-parse", "--verify", ref)
		return cmd.Run() == nil
	}
	require.False(t, checkBranch("origin/_armature"), "origin/_armature should not exist (single-branch clone)")
	require.False(t, checkBranch("_armature"), "local _armature should not exist yet")

	// Call CreateOrphanBranch
	c := adapters.New(clonePath)
	err = c.CreateOrphanBranch("_armature")
	require.NoError(t, err, "CreateOrphanBranch should fetch _armature from origin if not present locally")

	// Verify _armature now exists locally
	require.True(t, checkBranch("_armature"), "local _armature should exist after CreateOrphanBranch")

	// Verify it adopted the remote history (not a new orphan)
	getCommit := func(ref string) string {
		cmd := exec.CommandContext(context.Background(), "git", "-C", clonePath, "rev-parse", ref)
		out, err := cmd.Output()
		require.NoError(t, err)
		return strings.TrimSpace(string(out))
	}
	localCommit := getCommit("_armature")
	require.Equal(t, expectedArmatureSHA, localCommit, "local _armature should have the remote's commit, not a new orphan")
}

func TestCreateOrphanBranch_RestoresDetachedHEAD(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// Get the current commit SHA before detaching
	headSHACmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	headSHAOut, err := headSHACmd.Output()
	require.NoError(t, err)
	originalSHA := strings.TrimSpace(string(headSHAOut))

	// Detach HEAD at the current commit
	detachCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "checkout", "--detach")
	_, err = detachCmd.CombinedOutput()
	require.NoError(t, err)

	// Verify we're in detached HEAD state
	branchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "HEAD", strings.TrimSpace(string(branchOut)), "should be in detached HEAD state")

	// Call CreateOrphanBranch
	err = c.CreateOrphanBranch("_armature")
	require.NoError(t, err)

	// Verify _armature branch was created
	verifyBranch := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--verify", "_armature")
	require.NoError(t, verifyBranch.Run(), "_armature branch should exist")

	// Verify we're back in detached HEAD state at the original SHA (NOT on _armature)
	currentBranchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "HEAD", strings.TrimSpace(string(currentBranchOut)), "should be back in detached HEAD state")

	// Verify we're at the original commit SHA (not on _armature)
	currentSHACmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	currentSHAOut, err := currentSHACmd.Output()
	require.NoError(t, err)
	currentSHA := strings.TrimSpace(string(currentSHAOut))
	assert.Equal(t, originalSHA, currentSHA, "HEAD should be back at the original commit SHA")
}

// TestDirtyEntriesReturnsBothModifiedAndUntrackedPaths verifies that
// DirtyEntries (unlike IsWorkingTreeDirty, which ignores untracked files)
// surfaces every dirty path in the working tree with its tracked/untracked
// classification. Callers that need to classify dirty paths (e.g. an
// allow-list for known-safe debris) need the full set, not just a yes/no
// signal.
func TestDirtyEntriesReturnsBothModifiedAndUntrackedPaths(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	// Clean worktree: no dirty entries.
	entries, err := c.DirtyEntries()
	require.NoError(t, err)
	assert.Empty(t, entries)

	// Modify a tracked file.
	trackedFile := filepath.Join(repo, "tracked.txt")
	require.NoError(t, os.WriteFile(trackedFile, []byte("v1"), 0o600))
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("add", "tracked.txt")
	gitRun("commit", "-m", "add tracked file")
	require.NoError(t, os.WriteFile(trackedFile, []byte("v2"), 0o600))

	// Add an untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("new"), 0o600))

	entries, err = c.DirtyEntries()
	require.NoError(t, err)
	assert.ElementsMatch(t, []adapters.DirtyEntry{
		{Path: "tracked.txt", Untracked: false},
		{Path: "untracked.txt", Untracked: true},
	}, entries,
		"DirtyEntries must report the modified tracked file and the untracked file with correct classification")
}

// TestDirtyEntriesReportsOldPathForRename verifies that a staged rename
// reports both its destination (Path) and source (OldPath) path, so callers
// can detect a rename that crosses a boundary (e.g. into a state directory)
// even though `git status --porcelain` collapses the rename to one line.
func TestDirtyEntriesReportsOldPathForRename(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)

	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	origFile := filepath.Join(repo, "original.txt")
	require.NoError(t, os.WriteFile(origFile, []byte("content"), 0o600))
	gitRun("add", "original.txt")
	gitRun("commit", "-m", "add original file")

	gitRun("mv", "original.txt", "renamed.txt")

	entries, err := c.DirtyEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "renamed.txt", entries[0].Path)
	assert.Equal(t, "original.txt", entries[0].OldPath)
}

func addFileSubmodule(t *testing.T, parent string) {
	t.Helper()
	sub := t.TempDir()
	gitRun := func(dir string, args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun(sub, "init")
	gitRun(sub, "config", "user.email", "test@test.com")
	gitRun(sub, "config", "user.name", "Test")
	gitRun(sub, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(sub, "a.txt"), []byte("a"), 0o644))
	gitRun(sub, "add", "a.txt")
	gitRun(sub, "commit", "-m", "init")
	gitRun(parent, "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendor/lib")
	gitRun(parent, "commit", "-m", "add submodule")
}

func TestDirtyEntriesIncludingSubmodules_IgnoresLocalIgnoreConfig_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	addFileSubmodule(t, repo)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "vendor/lib", "dirty.txt"), []byte("x"), 0o644))
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("config", "submodule.vendor/lib.ignore", "dirty")

	hidden, err := c.DirtyEntries()
	require.NoError(t, err)
	assert.Empty(t, hidden, "DirtyEntries contract is unchanged: local submodule.ignore still hides dirt")

	got, err := c.DirtyEntriesIncludingSubmodules()
	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.False(t, got[0].Ignored)
	assert.Equal(t, "vendor/lib", got[0].Path)
}

func TestIndexConcealmentEntries_SkipWorktree_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("v1"), 0o600))
	gitRun("add", "src.txt")
	gitRun("commit", "-m", "add src")

	got, err := c.IndexConcealmentEntries()
	require.NoError(t, err)
	assert.Empty(t, got)

	gitRun("update-index", "--skip-worktree", "src.txt")
	got, err = c.IndexConcealmentEntries()
	require.NoError(t, err)
	assert.Equal(t, []string{"src.txt"}, got)
}

func TestIndexConcealmentEntries_AssumeUnchanged_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("v1"), 0o600))
	gitRun("add", "src.txt")
	gitRun("commit", "-m", "add src")
	gitRun("update-index", "--assume-unchanged", "src.txt")
	got, err := c.IndexConcealmentEntries()
	require.NoError(t, err)
	assert.Equal(t, []string{"src.txt"}, got)
}

func TestIndexConcealmentEntries_SkipWorktreeInsideSubmodule_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	addFileSubmodule(t, repo)
	gitRun := func(dir string, args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	sub := filepath.Join(repo, "vendor/lib")
	gitRun(sub, "update-index", "--skip-worktree", "a.txt")
	require.NoError(t, os.WriteFile(filepath.Join(sub, "a.txt"), []byte("mutated"), 0o644))

	got, err := c.IndexConcealmentEntries()
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join("vendor/lib", "a.txt")}, got)
}

func TestIndexConcealmentEntries_NotARepo(t *testing.T) {
	t.Parallel()
	_, err := adapters.New(t.TempDir()).IndexConcealmentEntries()
	require.Error(t, err)
}

func TestDirtyEntriesIncludingSubmodules_ShowsUntrackedWhenConfigHidesThem_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("config", "status.showUntrackedFiles", "no")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "helper.txt"), []byte("x"), 0o644))

	hidden, err := c.DirtyEntries()
	require.NoError(t, err)
	assert.Empty(t, hidden, "DirtyEntries contract is unchanged: showUntrackedFiles=no still hides untracked")

	got, err := c.DirtyEntriesIncludingSubmodules()
	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.True(t, got[0].Untracked)
	assert.Equal(t, "helper.txt", got[0].Path)
}

func TestIsolatedClientIgnoresGITWorkTree_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := initTestRepo(t)
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("v1"), 0o644))
	gitRun("add", "src.txt")
	gitRun("commit", "-m", "add src")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("mutated"), 0o644))

	exportDir := gitArchiveHEAD(t, repo)
	t.Setenv("GIT_WORK_TREE", exportDir)

	hidden, err := adapters.New(repo).DirtyEntriesIncludingSubmodules()
	require.NoError(t, err)
	assert.Empty(t, hidden, "default client is fooled by GIT_WORK_TREE pointing at a clean export")

	got, err := adapters.NewIsolated(repo).DirtyEntriesIncludingSubmodules()
	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.Equal(t, "src.txt", got[0].Path)
}

func TestIsolatedClientIgnoresCoreWorktree_REQ_LNGHZN_S10_T3(t *testing.T) { //nolint:paralleltest // t.Setenv is process-wide
	repo := initTestRepo(t)
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("v1"), 0o644))
	gitRun("add", "src.txt")
	gitRun("commit", "-m", "add src")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("mutated"), 0o644))

	exportDir := gitArchiveHEAD(t, repo)
	gitRun("config", "core.worktree", exportDir)

	hidden, err := adapters.New(repo).DirtyEntriesIncludingSubmodules()
	require.NoError(t, err)
	assert.Empty(t, hidden, "default client is fooled by core.worktree pointing at a clean export")

	got, err := adapters.NewIsolated(repo).DirtyEntriesIncludingSubmodules()
	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.Equal(t, "src.txt", got[0].Path)
}

func gitArchiveHEAD(t *testing.T, repo string) string {
	t.Helper()
	exportDir := t.TempDir()
	archive := exec.CommandContext(context.Background(), "git", "-C", repo, "archive", "--format=tar", "HEAD")
	tarOut, err := archive.Output()
	require.NoError(t, err)
	untar := exec.CommandContext(context.Background(), "tar", "-C", exportDir, "-xf", "-")
	untar.Stdin = strings.NewReader(string(tarOut))
	out, err := untar.CombinedOutput()
	require.NoError(t, err, "tar: %s", out)
	return exportDir
}

func TestDirtyEntriesIncludingSubmodules_SubmoduleUntrackedDespiteShowUntrackedNo_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	addFileSubmodule(t, repo)
	sub := filepath.Join(repo, "vendor/lib")
	gitRun := func(dir string, args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun(sub, "config", "status.showUntrackedFiles", "no")
	require.NoError(t, os.WriteFile(filepath.Join(sub, "helper.txt"), []byte("x"), 0o644))

	got, err := c.DirtyEntriesIncludingSubmodules()
	require.NoError(t, err)
	found := false
	for _, e := range got {
		if e.Path == filepath.Join("vendor/lib", "helper.txt") && e.Untracked {
			found = true
		}
	}
	assert.True(t, found, "expected untracked helper.txt inside submodule, got %#v", got)
}

func TestDirtyEntriesIncludingSubmodules_GitlinkWithoutGitIsDirty_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	addFileSubmodule(t, repo)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "vendor/lib", "dirty.txt"), []byte("x"), 0o644))
	require.NoError(t, os.RemoveAll(filepath.Join(repo, "vendor/lib", ".git")))

	got, err := c.DirtyEntriesIncludingSubmodules()
	require.NoError(t, err)
	require.NotEmpty(t, got)
	found := false
	for _, e := range got {
		if e.Path == "vendor/lib" {
			found = true
		}
	}
	assert.True(t, found, "expected vendor/lib gitlink without .git to be dirty, got %#v", got)
}

func TestCheckIgnoreSource_InfoExclude_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("helper.txt\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "helper.txt"), []byte("x"), 0o644))

	src, ignored, err := c.CheckIgnoreSource("helper.txt")
	require.NoError(t, err)
	assert.True(t, ignored)
	assert.Contains(t, src, filepath.Join(".git", "info", "exclude"))
}

func TestCheckIgnoreSource_NotIgnored(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	src, ignored, err := adapters.New(repo).CheckIgnoreSource("no-such-ignore-rule.txt")
	require.NoError(t, err)
	assert.False(t, ignored)
	assert.Empty(t, src)
}

func TestCheckIgnoreSource_NotARepo(t *testing.T) {
	t.Parallel()
	_, _, err := adapters.New(t.TempDir()).CheckIgnoreSource("x")
	require.Error(t, err)
}

func TestCheckIgnoreSource_Gitignore_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	c := adapters.New(repo)
	gitRun := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("coverage.out\n"), 0o644))
	gitRun("add", ".gitignore")
	gitRun("commit", "-m", "ignore")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "coverage.out"), []byte("x"), 0o644))

	src, ignored, err := c.CheckIgnoreSource("coverage.out")
	require.NoError(t, err)
	assert.True(t, ignored)
	assert.Equal(t, ".gitignore", src)
}

func TestDirtyEntriesIncludingSubmodules_NotARepo(t *testing.T) {
	t.Parallel()
	_, err := adapters.New(t.TempDir()).DirtyEntriesIncludingSubmodules()
	require.Error(t, err)
}
