package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	cmd1.Execute()

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
