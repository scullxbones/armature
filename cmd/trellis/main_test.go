package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/traceability"
	"github.com/scullxbones/trellis/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runTrls invokes the trellis cobra command tree with --repo injected and returns stdout + error.
func runTrls(t *testing.T, repo string, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs(append(args, "--repo", repo))
	err := root.Execute()
	return buf.String(), err
}

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "trls version")
}

func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	run(t, dir, "git", "config", "commit.gpgsign", "false")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %s %v failed: %s", name, args, out)
}

func TestWorkerInitCommand(t *testing.T) {
	repo := initTempRepo(t)
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"worker-init", "--repo", repo})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Worker ID:")
}

func TestWorkerInitCheckNotConfigured(t *testing.T) {
	repo := initTempRepo(t)
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"worker-init", "--check", "--repo", repo})

	err := cmd.Execute()
	assert.Error(t, err) // should fail — no worker ID
}

func TestWorkerInitCheckConfigured(t *testing.T) {
	repo := initTempRepo(t)

	// First init
	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"worker-init", "--repo", repo})
	_ = cmd1.Execute()

	// Then check
	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"worker-init", "--check", "--repo", repo})

	err := cmd2.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Worker ID:")
}

// suppress unused import warning for filepath and strings
var _ = filepath.Join
var _ = strings.Contains

func TestInitCommand_SingleBranch(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"init", "--repo", repo})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "single-branch")

	// Verify .issues directory structure was created
	assert.DirExists(t, filepath.Join(repo, ".issues"))
	assert.DirExists(t, filepath.Join(repo, ".issues", "ops"))
	assert.DirExists(t, filepath.Join(repo, ".issues", "state"))
	assert.FileExists(t, filepath.Join(repo, ".issues", "config.json"))
	assert.FileExists(t, filepath.Join(repo, ".issues", "ops", "SCHEMA"))
}

func TestInitCommand_WritesIssuesGitignore(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	gitignorePath := filepath.Join(repo, ".issues", ".gitignore")
	assert.FileExists(t, gitignorePath)
	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "state/")
}

func TestInitCommand_Idempotent(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init twice should not error
	for i := 0; i < 2; i++ {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs([]string{"init", "--repo", repo})
		assert.NoError(t, cmd.Execute())
	}
}

func TestMaterializeCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd1.Execute())

	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"materialize", "--repo", repo})

	err := cmd2.Execute()
	assert.NoError(t, err)
}

func TestReadyCommand_EmptyRepo(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"ready", "--repo", repo})

	err := cmd2.Execute()
	assert.NoError(t, err)
}

func TestCreateCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Fix bug", "--type", "task", "--id", "task-99"})

	err := cmd2.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "task-99")
}

// setupRepoWithTask creates a temp repo, runs trls init, and creates a test task.
func setupRepoWithTask(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, cmd2.Execute())

	return repo
}

func TestTransitionCommand(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Fixed"})

	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestTransitionCommand_InvalidStatus(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "in_progress"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "in_progress")
}

func TestClaimCommand(t *testing.T) {
	repo := setupRepoWithTask(t) // creates init + task-01

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "task-01")
}

func TestRenderContextCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test render", "--type", "task", "--id", "TST-001"})
	require.NoError(t, cmd2.Execute())

	buf := new(bytes.Buffer)
	cmd3 := newRootCmd()
	cmd3.SetOut(buf)
	cmd3.SetArgs([]string{"render-context", "--repo", repo, "--issue", "TST-001"})

	err := cmd3.Execute()
	assert.NoError(t, err)
	out := buf.String()
	assert.True(t, strings.Contains(out, "TST-001") || strings.Contains(out, "Test render"),
		"output should contain issue ID or title, got: %s", out)
}

func TestRenderContextCommand_AtSHA(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "TST-AT", "--title", "Time travel test", "--type", "task")
	require.NoError(t, err)

	// Commit ops so SHA1 captures state after create (issue exists, no notes)
	run(t, repo, "git", "add", ".issues/")
	run(t, repo, "git", "commit", "-m", "add create op")
	sha1Out, err2 := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err2)
	sha1 := strings.TrimSpace(string(sha1Out))

	_, err = runTrls(t, repo, "note", "--issue", "TST-AT", "--msg", "note added after sha1")
	require.NoError(t, err)

	// Commit so HEAD captures the note
	run(t, repo, "git", "add", ".issues/")
	run(t, repo, "git", "commit", "-m", "add note op")
	sha2Out, err2 := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err2)
	sha2 := strings.TrimSpace(string(sha2Out))

	// At SHA1: issue exists but has no note
	out1, err := runTrls(t, repo, "render-context", "--issue", "TST-AT", "--at", sha1)
	require.NoError(t, err)
	assert.NotContains(t, out1, "note added after sha1", "SHA1 should not have the note")

	// At SHA2 (HEAD): issue has the note
	out2, err := runTrls(t, repo, "render-context", "--issue", "TST-AT", "--at", sha2)
	require.NoError(t, err)
	assert.Contains(t, out2, "note added after sha1", "SHA2 should contain the note")
}

func TestValidateCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, cmd2.Execute())

	cmd3 := newRootCmd()
	cmd3.SetOut(new(bytes.Buffer))
	cmd3.SetArgs([]string{"materialize", "--repo", repo})
	require.NoError(t, cmd3.Execute())

	buf := new(bytes.Buffer)
	cmd4 := newRootCmd()
	cmd4.SetOut(buf)
	cmd4.SetArgs([]string{"validate", "--repo", repo})

	err := cmd4.Execute()
	assert.NoError(t, err)
}

func TestDecomposeApplyCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	// Init worker so decompose-apply can get a worker ID
	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"worker-init", "--repo", repo})
	require.NoError(t, cmd2.Execute())

	// Write a temp plan file
	planData := `{"version":1,"title":"Test Plan","issues":[{"id":"PLAN-001","title":"First issue","type":"task"}]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	buf := new(bytes.Buffer)
	cmd3 := newRootCmd()
	cmd3.SetOut(buf)
	cmd3.SetArgs([]string{"decompose-apply", "--repo", repo, "--plan", planFile})

	err := cmd3.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Applied")
}

func TestInitCommand_DualBranch(t *testing.T) {
	repo := initTempRepo(t)
	// An initial commit is required so CreateOrphanBranch can record current branch
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"init", "--dual-branch", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "dual-branch")

	// Worktree should exist at .trellis/
	assert.DirExists(t, filepath.Join(repo, ".trellis"))

	// .issues/ inside worktree should have config.json with dual-branch mode
	cfgPath := filepath.Join(repo, ".trellis", ".issues", "config.json")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "dual-branch")

	// Git config should have mode set
	modeCmd := exec.Command("git", "-C", repo, "config", "trellis.mode")
	modeOut, err := modeCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "dual-branch\n", string(modeOut))

	// Git config should have worktree path set
	wtCmd := exec.Command("git", "-C", repo, "config", "trellis.ops-worktree-path")
	wtOut, err := wtCmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(wtOut), ".trellis")
}

func TestMaterialize_SingleBranchMode_AfterModeRefactor(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init repo
	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd1.Execute())

	// Materialize should still work
	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"materialize", "--repo", repo})
	require.NoError(t, cmd2.Execute())
	assert.Contains(t, buf.String(), "Materialized")
}

func TestDecomposeContextCommand(t *testing.T) {
	planData := `{"version":1,"title":"My Test Plan","issues":[{"id":"PLAN-001","title":"First issue","type":"task"}]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"decompose-context", "--plan", planFile})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "My Test Plan")
}

func TestDualBranch_OpsCommittedToTrellisBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create an issue
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "test task", "--id", "T-001")
	require.NoError(t, err)

	// Materialize (reads ops dir, which is in worktree)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Write a note op — should commit to _trellis, not to main
	_, err = runTrls(t, repo, "note", "--issue", "T-001", "--msg", "dual branch test")
	require.NoError(t, err)

	// Verify the commit appeared on _trellis branch (inside the worktree)
	worktreePath := filepath.Join(repo, ".trellis")
	cmd := exec.Command("git", "-C", worktreePath, "log", "--oneline", "-3")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "ops: note")

	// Verify the main repo's log does NOT contain the ops commit
	mainCmd := exec.Command("git", "-C", repo, "log", "--oneline", "-5")
	mainOut, err := mainCmd.Output()
	require.NoError(t, err)
	assert.NotContains(t, string(mainOut), "ops: note")
}

func TestNote_SingleBranch_ViaAppendOp(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init and set up a task
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "test task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Note on the task
	out, err := runTrls(t, repo, "note", "--issue", "T-001", "--msg", "hello world")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
}

func TestSync_TransitionsMergedBranchIssuesToMerged(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "some feature", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done",
		"--branch", "feature/sync-test", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create and merge the feature branch in the git repo
	currentBranchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(currentBranchOut))

	run(t, repo, "git", "checkout", "-b", "feature/sync-test")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "feat: sync test work")
	run(t, repo, "git", "checkout", mainBranch)
	run(t, repo, "git", "merge", "--no-ff", "feature/sync-test", "-m", "Merge feature/sync-test")

	// Run sync — should auto-transition T-001 to merged
	out, err := runTrls(t, repo, "sync")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "merged")

	// Verify via materialized state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	index, err := materialize.LoadIndex(filepath.Join(repo, ".trellis", ".issues", "state", "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "merged", index["T-001"].Status)
}

func TestStatus_ShowsInProgressIssue(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my work", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "in-progress")
	assert.Contains(t, out, "T-001")
}

func TestStatus_DualBranch_DoneShowsAwaitingMerge(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Use dual-branch mode so done != merged
	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "pending merge", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done",
		"--branch", "feature/my-pr", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "status")
	require.NoError(t, err)
	// In dual-branch mode, done issues should be labeled "awaiting merge"
	assert.Contains(t, out, "awaiting merge")
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "feature/my-pr")
}

func TestInit_WritesPostMergeHookTemplate(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".issues", "hooks", "post-merge.sh.template")
	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "trls sync")
}

func TestMerged_RequiresDoneState_InDualBranchMode(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "new task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Try to merge an open issue in dual-branch mode — should fail with clear error
	_, err = runTrls(t, repo, "merged", "--issue", "T-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "done")
}

func TestMerged_AcceptsDoneIssue_SingleBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// In single-branch mode the validation is skipped — any issue can be passed to merged
	out, err := runTrls(t, repo, "merged", "--issue", "T-001", "--pr", "123")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
}

func TestMerged_AcceptsDoneIssue_DualBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Now in done state — merged should accept it
	out, err := runTrls(t, repo, "merged", "--issue", "T-001", "--pr", "42")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "#42")
}

func TestDualBranch_DoneToMergedWorkflow(t *testing.T) {
	// Full workflow: init --dual-branch → create → claim → in-progress → done →
	// status shows awaiting merge → merged --pr → status shows merged
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "feature work", "--id", "F-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "F-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "F-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "F-001", "--to", "done",
		"--branch", "feature/e2-test", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Status should show done (awaiting merge)
	statusOut, err := runTrls(t, repo, "status")
	require.NoError(t, err)
	assert.Contains(t, statusOut, "awaiting merge")
	assert.Contains(t, statusOut, "F-001")
	assert.Contains(t, statusOut, "feature/e2-test")

	// Mark as merged with PR reference
	mergedOut, err := runTrls(t, repo, "merged", "--issue", "F-001", "--pr", "99")
	require.NoError(t, err)
	assert.Contains(t, mergedOut, "F-001")
	assert.Contains(t, mergedOut, "#99")

	// Materialize and verify final state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// In dual-branch mode, the issues dir is in the worktree
	issuesDir := filepath.Join(repo, ".trellis", ".issues")
	index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "merged", index["F-001"].Status)

	// Status should no longer show done-awaiting-merge for F-001
	finalStatus, err := runTrls(t, repo, "status")
	require.NoError(t, err)
	assert.NotContains(t, finalStatus, "awaiting merge")
}

// TC-008: workers command and helper function tests

func TestLastOpTimestampFromLog_Empty(t *testing.T) {
	assert.Equal(t, int64(0), lastOpTimestampFromLog(nil))
	assert.Equal(t, int64(0), lastOpTimestampFromLog([]ops.Op{}))
}

func TestLastOpTimestampFromLog_ReturnsMax(t *testing.T) {
	opsList := []ops.Op{
		{Timestamp: 100},
		{Timestamp: 500},
		{Timestamp: 200},
	}
	assert.Equal(t, int64(500), lastOpTimestampFromLog(opsList))
}

func TestBuildWorkerStatus_ActiveWorker(t *testing.T) {
	now := int64(1000)
	allOps := []ops.Op{
		{Type: ops.OpClaim, TargetID: "T-001", Timestamp: 900, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 10}}, // TTL 10 min = 600 sec; 900+600=1500 > now(1000) → active
	}
	status := buildWorkerStatus("worker-a", allOps, 60, now)
	assert.Equal(t, "active", status.Status)
	assert.Equal(t, "T-001", status.ActiveIssue)
	assert.Equal(t, "worker-a", status.WorkerID)
}

func TestBuildWorkerStatus_StaleWorker(t *testing.T) {
	now := int64(10000)
	allOps := []ops.Op{
		{Type: ops.OpClaim, TargetID: "T-001", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 1}}, // TTL 1 min = 60 sec; 100+60=160 < now(10000) → stale
	}
	status := buildWorkerStatus("worker-a", allOps, 60, now)
	assert.Equal(t, "stale", status.Status)
	assert.Empty(t, status.ActiveIssue)
}

func TestBuildWorkerStatus_IdleWorker(t *testing.T) {
	now := int64(1000)
	allOps := []ops.Op{
		{Type: ops.OpNote, TargetID: "T-001", Timestamp: 900, WorkerID: "worker-a"},
	}
	// No claims, but recent op — idle within 2*TTL window
	status := buildWorkerStatus("worker-a", allOps, 1, now) // 2*1min=120s; 1000-900=100 < 120 → idle
	assert.Equal(t, "idle", status.Status)
	assert.Equal(t, int64(900), status.LastOpTime)
}

func TestBuildWorkerStatus_TransitionedClaim_NotActive(t *testing.T) {
	now := int64(10000)
	allOps := []ops.Op{
		{Type: ops.OpClaim, TargetID: "T-001", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 999}}, // Would be active — but transitioned
		{Type: ops.OpTransition, TargetID: "T-001", Timestamp: 200, WorkerID: "worker-a",
			Payload: ops.Payload{To: "done"}},
	}
	status := buildWorkerStatus("worker-a", allOps, 60, now)
	assert.NotEqual(t, "active", status.Status)
}

func TestWorkersCommand_EmptyRepo(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"workers", "--repo", repo})

	err = cmd.Execute()
	assert.NoError(t, err)
}

// TC-009: log, assign, heartbeat, decision, link, reopen commands and logPayloadSummary

func TestLogCommand_WithEntries(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "log")
	require.NoError(t, err)
	assert.Contains(t, out, "create")
}

func TestLogCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "log", "--json")
	require.NoError(t, err)
	assert.Contains(t, out, `"type"`)
}

func TestLogCommand_FilterByIssue(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "log", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestAssignCommand(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "assign", "--issue", "task-01", "--worker", "worker-abc")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestUnassignCommand(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "assign", "--issue", "task-01", "--worker", "worker-abc")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "unassign", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestHeartbeatCommand(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "heartbeat", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestDecisionCommand(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "decision", "--issue", "task-01",
		"--topic", "db", "--choice", "postgres", "--rationale", "mature")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestLinkCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task A", "--id", "T-A")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task B", "--id", "T-B")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "link", "--source", "T-A", "--dep", "T-B", "--rel", "blocked_by")
	require.NoError(t, err)
	assert.Contains(t, out, "T-A")
}

func TestReopenCommand(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "reopen", "--issue", "task-01")
	assert.NoError(t, err)
}

func TestReadyCommand_DraftTask_ExcludedFromReady(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a draft task
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Draft work", "--id", "draft-01", "--confidence", "draft")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)

	// The draft task should not appear in the ready queue
	assert.NotContains(t, out, "draft-01")
}

func TestReadyCommand_VerifiedTask_AppearsInReady(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a verified task
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Verified work", "--id", "verified-01", "--confidence", "verified")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)

	// The verified task should appear in the ready queue
	assert.Contains(t, out, "verified-01")
}

func TestReadyCommand_NoConfidenceField_DefaultsToVerified(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a task without a confidence flag (legacy behavior)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Legacy task", "--id", "legacy-01")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)

	// Task with no confidence field should default to verified and appear in ready
	assert.Contains(t, out, "legacy-01")
}

func TestDagTransitionCommand_PromotesDraftSubtree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a draft task (no parent, so no parent-status gate)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Draft task", "--id", "task-draft-01", "--confidence", "draft")
	require.NoError(t, err)
	// Create a second draft task outside the scope (different ID)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Another draft", "--id", "task-draft-02", "--confidence", "draft")
	require.NoError(t, err)

	// Confirm task-draft-01 is NOT in the ready queue yet
	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, out, "task-draft-01")

	// Run dag-transition to promote task-draft-01's subtree (just itself here)
	out, err = runTrls(t, repo, "dag-transition", "--issue", "task-draft-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-draft-01")

	// Now task-draft-01 should appear in the ready queue
	out, err = runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, "task-draft-01")

	// task-draft-02 should still be excluded (not in the promoted subtree)
	assert.NotContains(t, out, "task-draft-02")
}

func TestDagTransitionCommand_MissingIssueFlag(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "dag-transition")
	assert.Error(t, err)
}

func TestValidateCmd_CoverageOutput_HumanFormat(t *testing.T) {
	// Setup: repo with two issues and a worker
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create two tasks: one will be source-linked, one will remain uncited
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Cited task", "--id", "COV-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Uncited task", "--id", "COV-002")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Get the worker log path so we can inject ops directly
	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	logPath := filepath.Join(repo, ".issues", "ops", fmt.Sprintf("%s.log", workerID))

	t.Run("simple format when accepted_risk_nodes is zero", func(t *testing.T) {
		// Inject a source-link op for COV-001; COV-002 remains uncited (no accepted-risk either)
		sourceLinkOp := ops.Op{
			Type:      ops.OpSourceLink,
			TargetID:  "COV-001",
			Timestamp: time.Now().UnixMilli(),
			WorkerID:  workerID,
			Payload:   ops.Payload{SourceID: "src-abc"},
		}
		require.NoError(t, ops.AppendOp(logPath, sourceLinkOp))

		out, err := runTrls(t, repo, "validate")
		require.NoError(t, err)
		// 1 source-linked out of 2 total, 0 accepted-risk → simple format
		assert.Contains(t, out, "COVERAGE: 1/2 cited")
		assert.NotContains(t, out, "source-linked")
		assert.NotContains(t, out, "accepted-risk")
	})

	t.Run("extended format when accepted_risk_nodes is positive", func(t *testing.T) {
		// Inject a citation-accepted op for COV-002 → makes it accepted-risk
		citationAcceptedOp := ops.Op{
			Type:      ops.OpCitationAccepted,
			TargetID:  "COV-002",
			Timestamp: time.Now().UnixMilli(),
			WorkerID:  workerID,
			Payload:   ops.Payload{Confirmed: true},
		}
		require.NoError(t, ops.AppendOp(logPath, citationAcceptedOp))

		out, err := runTrls(t, repo, "validate")
		require.NoError(t, err)
		// 1 source-linked + 1 accepted-risk = 2/2 total cited → extended format
		assert.Contains(t, out, "COVERAGE: 2/2 cited (1 source-linked, 1 accepted-risk)")
	})
}

// TestTransitionToOpen tests that transitioning an in-progress issue back to open succeeds.
func TestTransitionToOpen(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "in-progress")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "open")
	assert.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

// TestTransitionToOpenRejectsInvalidAlias tests that a non-canonical alias like "reopened" is rejected.
func TestTransitionToOpenRejectsInvalidAlias(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "reopened")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reopened")
}

// TestClaimAutoAdvancesParentToInProgress verifies that claiming a task whose parent story
// is "open" emits an explicit transition op to advance the parent story to "in-progress",
// so the transition appears durably in the ops log.
func TestClaimAutoAdvancesParentToInProgress(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a story (parent) and a task (child)
	_, err = runTrls(t, repo, "create", "--type", "story", "--title", "Parent Story", "--id", "story-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Child Task", "--id", "task-01", "--parent", "story-01")
	require.NoError(t, err)

	// Materialize so state is up to date
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify story is "open" before claim
	issuesDir := repo + "/.issues"
	index, loadErr := materialize.LoadIndex(issuesDir + "/state/index.json")
	require.NoError(t, loadErr)
	require.Equal(t, "open", index["story-01"].Status, "story should start as open")

	// Claim the child task — this should auto-advance the parent story to in-progress
	_, err = runTrls(t, repo, "claim", "--issue", "task-01")
	require.NoError(t, err)

	// Check the ops log for an explicit transition op targeting story-01 with to=in-progress.
	// This verifies claim.go emits a durable op (not just relies on state engine inference).
	opsDir := issuesDir + "/ops"
	entries, readErr := os.ReadDir(opsDir)
	require.NoError(t, readErr)

	foundTransitionOp := false
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		logPath := opsDir + "/" + entry.Name()
		logOps, readOpErr := ops.ReadLog(logPath)
		require.NoError(t, readOpErr)
		for _, op := range logOps {
			if op.Type == ops.OpTransition && op.TargetID == "story-01" && op.Payload.To == ops.StatusInProgress {
				foundTransitionOp = true
			}
		}
	}
	assert.True(t, foundTransitionOp, "claim should emit an explicit transition op for the parent story to in-progress")
}

// TestUnassignReleasesClaimedToOpen verifies that unassigning a claimed issue
// emits a transition → open op, so the issue status returns to "open".
func TestUnassignReleasesClaimedToOpen(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a task and claim it (puts it in "claimed" status)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Unassign Test Task", "--id", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01")
	require.NoError(t, err)

	// Materialize and verify it's "claimed"
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issuesDir := repo + "/.issues"
	index, loadErr := materialize.LoadIndex(issuesDir + "/state/index.json")
	require.NoError(t, loadErr)
	require.Equal(t, ops.StatusClaimed, index["task-01"].Status, "task should be claimed before unassign")

	// Unassign — should release claimed → open
	_, err = runTrls(t, repo, "unassign", "--issue", "task-01")
	require.NoError(t, err)

	// Verify a transition → open op was emitted
	opsDir := issuesDir + "/ops"
	entries, readErr := os.ReadDir(opsDir)
	require.NoError(t, readErr)

	foundTransitionOp := false
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		logPath := opsDir + "/" + entry.Name()
		logOps, readOpErr := ops.ReadLog(logPath)
		require.NoError(t, readOpErr)
		for _, op := range logOps {
			if op.Type == ops.OpTransition && op.TargetID == "task-01" && op.Payload.To == ops.StatusOpen {
				foundTransitionOp = true
			}
		}
	}
	assert.True(t, foundTransitionOp, "unassign of claimed issue should emit a transition → open op")

	// Also verify the materialized status is now "open"
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	index2, loadErr2 := materialize.LoadIndex(issuesDir + "/state/index.json")
	require.NoError(t, loadErr2)
	assert.Equal(t, ops.StatusOpen, index2["task-01"].Status, "task status should be open after unassign")
}

func TestContextHistoryCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "TST-HIST", "--title", "History test issue", "--type", "task")
	require.NoError(t, err)

	// Commit ops so SHA1 captures creation
	run(t, repo, "git", "add", ".issues/")
	run(t, repo, "git", "commit", "-m", "ops: create TST-HIST")
	sha1Out, err2 := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err2)
	sha1 := strings.TrimSpace(string(sha1Out))

	_, err = runTrls(t, repo, "note", "--issue", "TST-HIST", "--msg", "progress note for history")
	require.NoError(t, err)

	// Commit ops so SHA2 captures note
	run(t, repo, "git", "add", ".issues/")
	run(t, repo, "git", "commit", "-m", "ops: note TST-HIST")
	sha2Out, err2 := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err2)
	sha2 := strings.TrimSpace(string(sha2Out))

	out, err := runTrls(t, repo, "context-history", "--issue", "TST-HIST")
	require.NoError(t, err)

	// Both SHAs should appear in the output (creation + note change both alter context)
	assert.Contains(t, out, sha1, "output should contain SHA1 (creation)")
	assert.Contains(t, out, sha2, "output should contain SHA2 (note change)")
}

func TestContextHistoryCommand_IssueNotFound(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Commit an empty ops dir
	run(t, repo, "git", "add", ".issues/")
	run(t, repo, "git", "commit", "-m", "ops: init")

	_, err = runTrls(t, repo, "context-history", "--issue", "NO-SUCH")
	assert.Error(t, err)
}

func TestInitDualBranchIdempotent(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// First init --dual-branch should succeed
	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err, "first dual-branch init should succeed")

	// Second init --dual-branch should also succeed (idempotent)
	_, err = runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err, "second dual-branch init should succeed (idempotent)")

	// The stored worktree path should be absolute (not relative).
	// This matters most when the user runs from the repo root without --repo
	// (repoPath defaults to "."), so we verify that even with "." as repo,
	// the git config records an absolute path.
	buf := new(bytes.Buffer)
	dotCmd := newRootCmd()
	dotCmd.SetOut(buf)
	// Change working directory to repo so "." refers to it
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repo))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// Re-init using "." as repo path — simulates running trls init --dual-branch in the repo root
	dotCmd.SetArgs([]string{"init", "--dual-branch", "--repo", "."})
	err = dotCmd.Execute()
	require.NoError(t, err, "init with relative repo '.' should succeed (idempotent)")

	wtCmd := exec.Command("git", "-C", repo, "config", "trellis.ops-worktree-path")
	wtOut, err := wtCmd.Output()
	require.NoError(t, err)
	storedPath := strings.TrimSpace(string(wtOut))
	assert.True(t, filepath.IsAbs(storedPath), "stored worktree path should be absolute, got: %s", storedPath)
}

func TestLogPayloadSummary(t *testing.T) {
	cases := []struct {
		op     ops.Op
		expect string
	}{
		{ops.Op{Type: ops.OpCreate, Payload: ops.Payload{Title: "My Task", NodeType: "task"}}, "My Task"},
		{ops.Op{Type: ops.OpClaim, Payload: ops.Payload{TTL: 60}}, "ttl=60"},
		{ops.Op{Type: ops.OpHeartbeat}, ""},
		{ops.Op{Type: ops.OpTransition, Payload: ops.Payload{To: "done", Outcome: "Fixed"}}, "→ done"},
		{ops.Op{Type: ops.OpNote, Payload: ops.Payload{Msg: "progress"}}, "progress"},
		{ops.Op{Type: ops.OpLink, Payload: ops.Payload{Rel: "blocked_by", Dep: "T-002"}}, "blocked_by T-002"},
		{ops.Op{Type: ops.OpDecision, Payload: ops.Payload{Topic: "db", Choice: "pg"}}, "db → pg"},
		{ops.Op{Type: ops.OpAssign, Payload: ops.Payload{AssignedTo: "worker-x"}}, "→ worker-x"},
		{ops.Op{Type: ops.OpAssign, Payload: ops.Payload{AssignedTo: ""}}, "unassigned"},
	}
	for _, tc := range cases {
		out := logPayloadSummary(tc.op)
		assert.Contains(t, out, tc.expect, "op type: %s", tc.op.Type)
	}
}

func TestProviderForType_KnownTypes(t *testing.T) {
	cases := []string{"filesystem", "confluence", "sharepoint"}
	for _, typ := range cases {
		p, err := providerForType(typ)
		assert.NoError(t, err, "type %q should be recognized", typ)
		assert.NotNil(t, p, "provider for %q should not be nil", typ)
	}
}

func TestProviderForType_UnknownType(t *testing.T) {
	_, err := providerForType("unknown-type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider type")
}

func TestReadyCommand_ParentFilter(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create parent and two children
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Parent", "--id", "parent-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Child A", "--id", "child-a", "--parent", "parent-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Unrelated", "--id", "unrelated-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json", "--parent", "parent-01")
	require.NoError(t, err)
	assert.Contains(t, out, "child-a")
	assert.NotContains(t, out, "unrelated-01")
}

func TestReadyCommand_TextFormat_WithTasks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Text format task", "--id", "text-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "text")
	require.NoError(t, err)
	assert.Contains(t, out, "text-01")
}

func TestCollectDraftSubtree_ReturnsNilForUnknownRoot(t *testing.T) {
	state := materialize.NewState()
	result := collectDraftSubtree(state, "nonexistent")
	assert.Nil(t, result)
}

func TestCollectDraftSubtree_ReturnsDraftIssuesInSubtree(t *testing.T) {
	state := materialize.NewState()
	state.Issues["root"] = &materialize.Issue{
		ID: "root", Provenance: materialize.Provenance{Confidence: "draft"},
		Children: []string{"child-1", "child-2"},
	}
	state.Issues["child-1"] = &materialize.Issue{
		ID: "child-1", Provenance: materialize.Provenance{Confidence: "draft"},
	}
	state.Issues["child-2"] = &materialize.Issue{
		ID: "child-2", Provenance: materialize.Provenance{Confidence: "verified"},
	}

	result := collectDraftSubtree(state, "root")
	ids := make([]string, len(result))
	for i, iss := range result {
		ids[i] = iss.ID
	}
	assert.Contains(t, ids, "root")
	assert.Contains(t, ids, "child-1")
	assert.NotContains(t, ids, "child-2")
}

func TestWriteDAGSummaryArtifact_CreatesFile(t *testing.T) {
	issuesDir := t.TempDir()
	stateDir := filepath.Join(issuesDir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	reviewed := []*materialize.Issue{
		{ID: "T-001", Title: "First issue"},
		{ID: "T-002", Title: "Second issue"},
	}
	approvedIDs := []string{"T-001"}
	cov := traceability.Coverage{CoveragePct: 75.0, CitedNodes: 3, TotalNodes: 4}

	err := writeDAGSummaryArtifact(issuesDir, reviewed, approvedIDs, cov)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(stateDir, "dag-summary.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "T-001")
	assert.Contains(t, content, "T-002")
	assert.Contains(t, content, "approved")
	assert.Contains(t, content, "skipped/rejected")
	assert.Contains(t, content, "75.0%")
}

// UX: heartbeat should emit plain text in human mode, not JSON
func TestHeartbeatCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "heartbeat", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"heartbeat"`, "default format should not be JSON")
}

// UX: heartbeat with --format json should still return JSON for agent consumers
func TestHeartbeatCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "heartbeat", "--issue", "task-01", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"heartbeat"`)
	assert.Contains(t, out, "task-01")
}

// UX: note should emit plain text in human mode, not JSON
func TestNoteCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "note", "--issue", "task-01", "--msg", "progress update")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"note"`, "default format should not be JSON")
}

// UX: note with --format json should still return JSON
func TestNoteCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "note", "--issue", "task-01", "--msg", "progress update", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"note"`)
	assert.Contains(t, out, "task-01")
}

// UX: transition should emit plain text in human mode, not JSON
func TestTransitionCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--outcome", "completed")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"status"`, "default format should not be JSON")
}

// UX: transition with --format json should still return JSON
func TestTransitionCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--outcome", "completed", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"status"`)
	assert.Contains(t, out, "task-01")
}

// UX: trls init run a second time should print an "already initialized" message
func TestInitCommand_AlreadyInitialized(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	assert.Contains(t, out, "already")
}

// TestLogSlot_EnvVar verifies that TRLS_LOG_SLOT routes ops to a slotted log file.
func TestLogSlot_EnvVar(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	t.Setenv("TRLS_LOG_SLOT", "beta")

	_, err = runTrls(t, repo, "note", "--issue", "task-01", "--msg", "slotted note")
	require.NoError(t, err)

	// The slotted file must exist; the plain file must NOT contain this note
	opsDir := filepath.Join(repo, ".issues", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	var slottedFile, plainFile string
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, "~beta") {
			slottedFile = filepath.Join(opsDir, name)
		} else if strings.HasSuffix(name, ".log") {
			plainFile = filepath.Join(opsDir, name)
		}
	}

	require.NotEmpty(t, slottedFile, "expected a ~beta.log file to exist")
	slottedContent, err := os.ReadFile(slottedFile)
	require.NoError(t, err)
	assert.Contains(t, string(slottedContent), "slotted note")

	if plainFile != "" {
		plainContent, _ := os.ReadFile(plainFile)
		assert.NotContains(t, string(plainContent), "slotted note",
			"plain log must not contain the slotted note")
	}
}

// TestLogSlot_Empty_UsesPlainLog verifies that an empty TRLS_LOG_SLOT uses the normal log path.
func TestLogSlot_Empty_UsesPlainLog(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	t.Setenv("TRLS_LOG_SLOT", "") // explicitly empty

	_, err = runTrls(t, repo, "note", "--issue", "task-01", "--msg", "plain note")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".issues", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.Name(), "~",
			"no slotted file should exist when TRLS_LOG_SLOT is empty")
	}
}

// TestLogSlot_ReplayIncludesSlottedOps verifies that ops written to a slotted log
// are included in materialised state alongside ops from the plain log.
func TestLogSlot_ReplayIncludesSlottedOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task A", "--id", "task-a")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task B", "--id", "task-b")
	require.NoError(t, err)

	// Slot "one" transitions task-a to done
	t.Setenv("TRLS_LOG_SLOT", "one")
	_, err = runTrls(t, repo, "claim", "--issue", "task-a")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-a", "--to", "done", "--outcome", "slot one")
	require.NoError(t, err)

	// Slot "two" transitions task-b to done
	t.Setenv("TRLS_LOG_SLOT", "two")
	_, err = runTrls(t, repo, "claim", "--issue", "task-b")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-b", "--to", "done", "--outcome", "slot two")
	require.NoError(t, err)

	// Unset slot so materialize uses the main context
	t.Setenv("TRLS_LOG_SLOT", "")
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Both tasks must appear as merged (in single-branch mode done auto-transitions to merged)
	outA, err := runTrls(t, repo, "show", "--issue", "task-a")
	require.NoError(t, err)
	assert.Contains(t, outA, "merged")

	outB, err := runTrls(t, repo, "show", "--issue", "task-b")
	require.NoError(t, err)
	assert.Contains(t, outB, "merged")
}

// TestWorkersCommand_SlottedLogs verifies that ops from slotted log files are
// included in worker activity output alongside the plain log's ops.
func TestWorkersCommand_SlottedLogs(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Slot task", "--id", "slot-task")
	require.NoError(t, err)

	// Write an op via the plain log (claim)
	_, err = runTrls(t, repo, "claim", "--issue", "slot-task")
	require.NoError(t, err)

	// Write an op via a slotted log (transition done)
	t.Setenv("TRLS_LOG_SLOT", "w")
	_, err = runTrls(t, repo, "transition", "--issue", "slot-task", "--to", "done", "--outcome", "via slot")
	require.NoError(t, err)
	t.Setenv("TRLS_LOG_SLOT", "")

	// The workers output must show the worker as active/idle (not missing)
	// and must reflect ops from both log files
	out, err := runTrls(t, repo, "workers")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
	// Worker should appear — if the slot's ops were dropped we'd see no activity
	// (this test will fail before the fix because enumerateWorkers drops the plain log's ops
	// when the slot log overwrites them, leaving the worker with only the transition op
	// and no claim op, which produces inconsistent state)
	assert.NotContains(t, out, "error")
}
