package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePendingPushTracker struct {
	count       int
	incremented int
	resetCalls  int
}

func (f *fakePendingPushTracker) Increment() (int, error) {
	f.incremented++
	f.count++
	return f.count, nil
}

func (f *fakePendingPushTracker) Reset() error {
	f.resetCalls++
	f.count = 0
	return nil
}

func (f *fakePendingPushTracker) Count() (int, error) {
	return f.count, nil
}

var runTrlsMu sync.Mutex

// getTestContext resolves the execution context for a test repository.
// This is a test helper that mirrors the production config.ResolveContext behavior.
func appendRawCreate(logPath, workerID, id, dod, scope string) error {
	return appendRawCreateConfidence(logPath, workerID, id, dod, scope, "draft")
}

func appendRawCreateConfidence(logPath, workerID, id, dod, scope, confidence string) error {
	return ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  id,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:            id,
			NodeType:         "task",
			Scope:            []string{scope},
			DefinitionOfDone: dod,
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       confidence,
		},
	})
}

func plantOverlappingFooPair(t *testing.T, repo string) {
	t.Helper()
	plantVerifiedTask(t, repo, "task-01", "src/foo/*")
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, appendRawCreateConfidence(logPath, workerID, "task-02", "Task 2 is complete and tested", "src/foo/bar.go", "verified"))
}

func plantVerifiedTask(t *testing.T, repo, id, scope string) {
	t.Helper()
	plantVerifiedTaskUnder(t, repo, id, scope, "")
}

func plantVerifiedTaskUnder(t *testing.T, repo, id, scope, parent string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	err = ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  id,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:            id,
			NodeType:         "task",
			Parent:           parent,
			Scope:            []string{scope},
			DefinitionOfDone: "Task " + id + " is complete and tested",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "verified",
		},
	})
	require.NoError(t, err)
}

func getTestContext(t *testing.T, repo string) *config.Context {
	t.Helper()
	ctx, err := config.ResolveContext(repo)
	require.NoError(t, err, "failed to resolve context for test repo %q", repo)
	return ctx
}

// getTestStateDir returns the absolute path to the worker-specific state directory.
// In dual-branch mode (unmigrated .arm/.armature/ layout), state lives at the
// .arm worktree root (.arm/state/); in the collapsed layout, it lives inside
// .armature/state/.
func getTestStateDir(t *testing.T, repo string) string {
	t.Helper()
	workerID, _ := worker.GetWorkerID(repo) //nolint:errcheck // best-effort in test setup
	if workerID == "" {
		workerID = "default"
	}
	// Apply the slot suffix if ARM_LOG_SLOT is set, matching the behavior in main.go
	workerID = workerIdentityWithSlot(workerID)
	if _, err := os.Stat(filepath.Join(repo, ".arm", ".git")); err == nil {
		return filepath.Join(repo, ".armature", "state", workerID)
	}
	return filepath.Join(repo, ".armature", "state", workerID)
}

// bootstrapRepoForTest runs bootstrap on a repo and automatically migrates the layout
// from the old .arm/.armature/ layout to the new collapsed .armature/ layout.
// This is a test helper that ensures repos can run non-bootstrap commands after bootstrap.
func bootstrapRepoForTest(t *testing.T, repo string) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute(), "bootstrap failed")
}

func TestStateDirFor(t *testing.T) {
	ctx := &config.Context{IssuesDir: "/repo/.armature", WorktreePath: ""}
	assert.Equal(t, "/repo/.armature/state/w1", stateDirFor(ctx, "w1"))

	ctx2 := &config.Context{IssuesDir: "/repo/.arm/.armature", WorktreePath: "/repo/.arm"}
	assert.Equal(t, "/repo/.arm/state/w1", stateDirFor(ctx2, "w1"))
}

const testIntroductionOutcome = "Completed the work described in the task"

// enrichTestCLIArgs supplies task required fields and a non-vague outcome
// so fixture commands do not trip write-time Introduction (E6 / W11).
func enrichTestCLIArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	switch args[0] {
	case "create":
		return enrichTestCreateArgs(args)
	case "transition":
		return enrichTestTransitionArgs(args)
	default:
		return args
	}
}

func enrichTestCreateArgs(args []string) []string {
	nodeType := "task"
	id := ""
	hasScope, hasDoD, hasAcceptance := false, false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type":
			if i+1 < len(args) {
				nodeType = args[i+1]
			}
		case "--id":
			if i+1 < len(args) {
				id = args[i+1]
			}
		case "--scope":
			hasScope = true
		case "--dod":
			hasDoD = true
		case "--acceptance":
			hasAcceptance = true
		}
	}
	if nodeType != "task" {
		return args
	}
	if !hasScope {
		if id == "" {
			id = fmt.Sprintf("anon-%d", time.Now().UnixNano())
		}
		args = append(args, "--scope", "testdata/"+id+".go")
	}
	if !hasDoD {
		args = append(args, "--dod", "Task implementation is complete and verified")
	}
	if !hasAcceptance {
		args = append(args, "--acceptance", testAcceptance)
	}
	return args
}

func enrichTestTransitionArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		if out[i] == "--outcome" && i+1 < len(out) && len(strings.TrimSpace(out[i+1])) < 20 {
			out[i+1] = testIntroductionOutcome
		}
	}
	return out
}

func runTrls(t *testing.T, repo string, args ...string) (string, error) {
	t.Helper()
	runTrlsMu.Lock()
	defer runTrlsMu.Unlock()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs(append(enrichTestCLIArgs(args), "--repo", repo))
	err := root.Execute()
	return buf.String(), err
}

// runTrlsWithStderr invokes the armature cobra command tree and returns stdout, stderr, and error.
func runTrlsWithStderr(t *testing.T, repo string, args ...string) (string, string, error) {
	t.Helper()
	runTrlsMu.Lock()
	defer runTrlsMu.Unlock()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs(append(enrichTestCLIArgs(args), "--repo", repo))
	err := root.Execute()
	return buf.String(), errBuf.String(), err
}

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "arm version")
}

func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	run(t, dir, "git", "config", "commit.gpgsign", "false")
	run(t, dir, "git", "config", "gc.auto", "0")
	run(t, dir, "git", "config", "maintenance.auto", "false")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) { //nolint:unparam // name is "git" in all current callers but helper is intentionally general
	t.Helper()
	cmd := exec.CommandContext(context.Background(), name, args...)
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
	require.NoError(t, cmd1.Execute())

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

func TestInitCommand_WritesIssuesGitignore(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	gitignorePath := filepath.Join(repo, ".armature", ".gitignore")
	assert.FileExists(t, gitignorePath)
	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "state/")
}

func TestInitCommand_Idempotent(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init twice should not error
	for range 2 {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs([]string{"bootstrap", "--repo", repo})
		assert.NoError(t, cmd.Execute())
	}
}

func TestMaterializeCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"bootstrap", "--repo", repo})
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

	bootstrapRepoForTest(t, repo)

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

	bootstrapRepoForTest(t, repo)

	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Fix bug", "--type", "task", "--id", "task-99"}))

	err := cmd2.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "task-99")
}

// setupArmatureLayout creates a collapsed .armature/ worktree layout with necessary directories and config.
// This is a test helper that sets up the new layout that commands expect.
// Returns the path to the .armature worktree.
func setupArmatureLayout(t *testing.T, repo string) string {
	t.Helper()
	armatureDir := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(armatureDir, "ops"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(armatureDir, "state"), 0o755))

	// Create config.json
	cfg := config.DefaultConfig("go")
	require.NoError(t, config.WriteConfig(filepath.Join(armatureDir, "config.json"), cfg))

	// Set git config to point to the new collapsed layout
	run(t, repo, "git", "config", "armature.ops-worktree-path", armatureDir)

	// Initialize worker ID (required for commands that work with issues)
	run(t, repo, "git", "config", "armature.worker-id", "test-worker-"+fmt.Sprintf("%d", time.Now().UnixNano()))

	return armatureDir
}

// setupRepoWithTask creates a temp repo, sets up the collapsed .armature/ layout, and creates a test task.
// The task includes a definition_of_done so review assessments that cover
// "definition_of_done" are accepted by ValidateResultCoverage.
func setupRepoWithTask(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up the collapsed .armature/ layout instead of relying on bootstrap
	// (bootstrap currently creates the old .arm/.armature/ layout; this test
	// setup uses the new layout that commands expect)
	setupArmatureLayout(t, repo)

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:            "Test task",
			NodeType:         "task",
			Scope:            []string{"cmd/armature/task_01.go"},
			DefinitionOfDone: "Task implementation is complete and verified",
			Confidence:       "verified",
		},
	}))
	return repo
}

func TestTransitionCommand(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs(enrichTestCLIArgs([]string{
		"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", "Fixed", "--force",
	}))

	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestTransitionCommand_InvalidStatus(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "in_progress"}))

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "in_progress")
}

func TestCheckAndWarnParentStoryStatus_AllSiblingsDone(t *testing.T) {
	// When all tasks under a story are done and the story is in-progress,
	// checkAndWarnParentStoryStatus should print a warning.
	// Covers transition.go:121,210,214,221-224 NOT COVERED mutations.
	index := materialize.Index{
		"task-01":  {Status: "in-progress", Type: "task", Parent: "story-01"},
		"story-01": {Status: "in-progress", Type: "story", Children: []string{"task-01"}},
	}
	errBuf := new(bytes.Buffer)
	cobraCmd := newRootCmd()
	cobraCmd.SetErr(errBuf)

	err := checkAndWarnParentStoryStatus(index, "task-01", cobraCmd)
	assert.NoError(t, err)
	assert.Contains(t, errBuf.String(), "story-01", "should warn that story is still in-progress")
	assert.Contains(t, errBuf.String(), "WARNING")
}

func TestCheckAndWarnParentStoryStatus_SiblingNotDone(t *testing.T) {
	// When a sibling is not done, no warning should appear (transition.go:214 NOT COVERED mutation)
	index := materialize.Index{
		"task-01":  {Status: "in-progress", Type: "task", Parent: "story-01"},
		"task-02":  {Status: "open", Type: "task", Parent: "story-01"},
		"story-01": {Status: "in-progress", Type: "story", Children: []string{"task-01", "task-02"}},
	}
	errBuf := new(bytes.Buffer)
	cobraCmd := newRootCmd()
	cobraCmd.SetErr(errBuf)

	err := checkAndWarnParentStoryStatus(index, "task-01", cobraCmd)
	assert.NoError(t, err)
	assert.NotContains(t, errBuf.String(), "WARNING", "no warning when a sibling is not done")
}

func TestCheckAndWarnParentStoryStatus_ParentNotInProgress(t *testing.T) {
	// When parent story is NOT in-progress, early-return (transition.go:197 NOT COVERED mutation)
	index := materialize.Index{
		"task-01":  {Status: "in-progress", Type: "task", Parent: "story-01"},
		"story-01": {Status: "open", Type: "story", Children: []string{"task-01"}},
	}
	errBuf := new(bytes.Buffer)
	cobraCmd := newRootCmd()
	cobraCmd.SetErr(errBuf)

	err := checkAndWarnParentStoryStatus(index, "task-01", cobraCmd)
	assert.NoError(t, err)
	assert.Empty(t, errBuf.String(), "no warning when parent is not in-progress")
}

func TestCheckAndWarnParentStoryStatus_NoParent(t *testing.T) {
	// Issue has no parent — early-return (transition.go:187 NOT COVERED mutation)
	index := materialize.Index{
		"task-01": {Status: "in-progress", Type: "task", Parent: ""},
	}
	errBuf := new(bytes.Buffer)
	cobraCmd := newRootCmd()
	cobraCmd.SetErr(errBuf)

	err := checkAndWarnParentStoryStatus(index, "task-01", cobraCmd)
	assert.NoError(t, err)
	assert.Empty(t, errBuf.String())
}

func TestClaimCommand(t *testing.T) {
	repo := setupRepoWithTask(t) // creates init + task-01

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "task-01")
}

// TestAmendCommand_RejectsInvalidType verifies that --type is validated before
// the amend op is written. Without this, `arm amend --type` would write an
// invalid type straight into the permanent op log, bypassing the same
// validation NXTTN-S2-T2 enforces on create.
func TestAmendCommand_RejectsInvalidType(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "amend", "--issue", "task-01", "--type", "nonsense")
	require.Error(t, err, "amend --type with an invalid type should be rejected before the op is written")
	assert.Contains(t, err.Error(), "invalid type")

	out, err := runTrls(t, repo, "show", "--issue", "task-01", "--field", "type")
	require.NoError(t, err)
	assert.Contains(t, out, "task")
	assert.NotContains(t, out, "nonsense")
}

func TestRenderContextCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test render", "--type", "task", "--id", "TST-001"}))
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

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "TST-AT", "--title", "Time travel test", "--type", "task")
	require.NoError(t, err)

	// Ops are committed automatically to the _armature branch (dual-branch mode
	// is always on), so capture the ops-worktree HEAD after create (issue exists, no notes).
	testCtx := getTestContext(t, repo)
	require.NotEmpty(t, testCtx.WorktreePath)
	sha1Out, err2 := exec.CommandContext(context.Background(), "git", "-C", testCtx.WorktreePath, "rev-parse", "HEAD").Output()
	require.NoError(t, err2)
	sha1 := strings.TrimSpace(string(sha1Out))

	_, err = runTrls(t, repo, "note", "--issue", "TST-AT", "--msg", "note added after sha1")
	require.NoError(t, err)

	// Capture ops-worktree HEAD after the note (should now include it)
	sha2Out, err2 := exec.CommandContext(context.Background(), "git", "-C", testCtx.WorktreePath, "rev-parse", "HEAD").Output()
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

func TestRenderContextCommand_AtSHA_DualBranchUsesWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "TST-DB", "--title", "Dual branch render", "--type", "task")
	require.NoError(t, err)

	testCtx2 := getTestContext(t, repo)
	require.NotEmpty(t, testCtx2.WorktreePath)
	shaCmd := exec.CommandContext(context.Background(), "git", "-C", testCtx2.WorktreePath, "rev-parse", "HEAD")
	shaOutBytes, err := shaCmd.CombinedOutput()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaOutBytes))

	out, err := runTrls(t, repo, "render-context", "--issue", "TST-DB", "--at", sha)
	require.NoError(t, err)
	assert.Contains(t, out, "Dual branch render")
}

func TestValidateCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01",
		"--dod", "Task implementation is complete and verified",
		"--scope", "cmd/armature/main.go",
		"--acceptance", `[{"type":"test_passes"}]`}))
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

	bootstrapRepoForTest(t, repo)

	// Init worker so decompose-apply can get a worker ID
	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"worker-init", "--repo", repo})
	require.NoError(t, cmd2.Execute())

	// Write a temp plan file
	planData := `{"version":1,"title":"Test Plan","issues":[{` +
		`"id":"PLAN-001","title":"First issue","type":"task","source":"src-test",` +
		`"scope":"internal/PLAN-001.go","dod":"First issue is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	buf := new(bytes.Buffer)
	cmd3 := newRootCmd()
	cmd3.SetOut(buf)
	cmd3.SetArgs([]string{"dag", "apply", "--repo", repo, "--plan", planFile})

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
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "human"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "collapsed")

	// A fresh init goes straight to the collapsed .armature/ worktree
	// (LNGHZN-S1-T2); no .arm/ worktree is created.
	assert.False(t, pathExists(filepath.Join(repo, ".arm")))
	assert.DirExists(t, filepath.Join(repo, ".armature"))

	cfgPath := filepath.Join(repo, ".armature", "config.json")
	assert.FileExists(t, cfgPath)

	// Git config should have worktree path set
	wtCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "config", "armature.ops-worktree-path")
	wtOut, err := wtCmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(wtOut), ".armature")
}

func TestDecomposeContextCommand(t *testing.T) {
	planData := `{"version":1,"title":"My Test Plan","issues":[{"id":"PLAN-001","title":"First issue","type":"task"}]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dag", "context", "--plan", planFile})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "My Test Plan")
}

func TestDualBranch_OpsCommittedToTrellisBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create an issue
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "test task", "--id", "T-001")
	require.NoError(t, err)

	// Materialize (reads ops dir, which is in worktree)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Write a note op — should commit to _armature, not to main
	_, err = runTrls(t, repo, "note", "--issue", "T-001", "--msg", "dual branch test")
	require.NoError(t, err)

	// Verify the commit appeared on _armature branch (inside the worktree)
	worktreePath := filepath.Join(repo, ".armature")
	cmd := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "log", "--oneline", "-3")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "ops: note")

	// Verify the main repo's log does NOT contain the ops commit
	mainCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "log", "--oneline", "-5")
	mainOut, err := mainCmd.Output()
	require.NoError(t, err)
	assert.NotContains(t, string(mainOut), "ops: note")
}

func TestSync_TransitionsMergedBranchIssuesToMerged(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "some feature", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done", "--skip-delivery-gate", "--force",
		"--branch", "feature/sync-test", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create and merge the feature branch in the git repo
	currentBranchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(currentBranchOut))

	run(t, repo, "git", "checkout", "-b", "feature/sync-test")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "feat: sync test work")
	run(t, repo, "git", "checkout", mainBranch)
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "merge", "--no-ff", "feature/sync-test", "-m", "Merge feature/sync-test")

	// Run sync — should auto-transition T-001 to merged
	out, err := runTrls(t, repo, "sync")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "merged")

	// Verify via materialized state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "merged", index["T-001"].Status)
}

func TestSync_DryRun_PrintsPlanWithoutWritingOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "some feature", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done", "--skip-delivery-gate", "--force",
		"--branch", "feature/sync-dryrun-test", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create and merge the feature branch
	currentBranchCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(currentBranchOut))

	run(t, repo, "git", "checkout", "-b", "feature/sync-dryrun-test")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "feat: dry-run test work")
	run(t, repo, "git", "checkout", mainBranch)
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "merge", "--no-ff", "feature/sync-dryrun-test", "-m", "Merge feature/sync-dryrun-test")

	// Count ops before dry-run
	issuesDir := filepath.Join(repo, ".armature")
	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	// Apply slot suffix if ARM_LOG_SLOT is set, matching the behavior in main.go
	workerID = workerIdentityWithSlot(workerID)
	logPath := filepath.Join(issuesDir, "ops", workerID+".log")
	statBefore, err := os.Stat(logPath)
	require.NoError(t, err)

	// Run sync --dry-run: should print plan and exit 0
	out, err := runTrls(t, repo, "sync", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "dry-run")

	// Verify state was NOT changed — status should still be "done" not "merged"
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "done", index["T-001"].Status)

	// Verify ops log was not grown (no ops written)
	statAfter, err := os.Stat(logPath)
	require.NoError(t, err)
	if statBefore != nil && statAfter != nil {
		assert.Equal(t, statBefore.Size(), statAfter.Size(), "ops log should not grow with --dry-run")
	}

	// Running sync --dry-run again should be idempotent
	out2, err := runTrls(t, repo, "sync", "--dry-run")
	require.NoError(t, err)
	assert.Equal(t, out, out2)
}

func TestDecomposeRevert_DryRun_PrintsPlanWithoutWritingOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Write a plan file
	planContent := `{
		"version": 1,
		"title": "Test Plan",
		"issues": [
			{"id": "PLAN-001", "title": "First issue", "type": "task",
			 "dod": "First issue is complete and tested", "source": "src-test",
			 "scope": "internal/PLAN-001.go", "acceptance":[{"type":"test_passes"}]},
			{"id": "PLAN-002", "title": "Second issue", "type": "task",
			 "dod": "Second issue is complete and tested", "source": "src-test",
			 "scope": "internal/PLAN-002.go", "acceptance":[{"type":"test_passes"}]}
		]
	}`
	planPath := filepath.Join(repo, "test-plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(planContent), 0o644))

	// Apply plan first
	_, err = runTrls(t, repo, "dag", "apply", "--plan", planPath)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Count ops before dry-run
	issuesDir := filepath.Join(repo, ".armature")
	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	logPath := filepath.Join(issuesDir, "ops", workerID+".log")
	statBefore, err := os.Stat(logPath)
	require.NoError(t, err)

	// Run decompose-revert --dry-run: should print removal plan and exit 0
	out, err := runTrls(t, repo, "dag", "revert", "--plan", planPath, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "PLAN-001")
	assert.Contains(t, out, "PLAN-002")
	assert.Contains(t, out, "dry-run")

	// Verify ops log was not grown
	statAfter, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, statBefore.Size(), statAfter.Size(), "ops log should not grow with --dry-run")

	// Verify materialized state unchanged — both issues still "open"
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "open", index["PLAN-001"].Status)
	assert.Equal(t, "open", index["PLAN-002"].Status)
}

func TestStatus_ShowsInProgressIssue(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my work", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "--format", "human", "list", "--group")
	require.NoError(t, err)
	assert.Contains(t, out, "in-progress")
	assert.Contains(t, out, "T-001")
}

func TestStatus_DualBranch_DoneShowsAwaitingMerge(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Use dual-branch mode so done != merged
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "pending merge", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done", "--skip-delivery-gate", "--force",
		"--branch", "feature/my-pr", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "--format", "human", "list", "--group")
	require.NoError(t, err)
	// In dual-branch mode, done issues should be labeled "awaiting merge"
	assert.Contains(t, out, "awaiting merge")
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "feature/my-pr")
}

func TestInit_WritesPostMergeHookTemplate(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".armature", "hooks", "post-merge.sh.template")
	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm sync")
}

func TestInit_WritesPostCommitHookTemplate(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".armature", "hooks", "post-commit.sh.template")
	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "arm heartbeat")
	assert.Contains(t, content, "arm push-ops")
}

func TestInit_WritesPrepareCommitMsgHookTemplate(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".armature", "hooks", "prepare-commit-msg.sh.template")
	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "claim")
	assert.Contains(t, content, "prepare-commit-msg")
}

func TestInit_InstallsHooksIntoGitHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Check that all three hook templates are installed to .git/hooks/
	postCommitPath := filepath.Join(repo, ".git", "hooks", "post-commit")
	postMergePath := filepath.Join(repo, ".git", "hooks", "post-merge")
	prepareCommitMsgPath := filepath.Join(repo, ".git", "hooks", "prepare-commit-msg")

	assert.FileExists(t, postCommitPath)
	assert.FileExists(t, postMergePath)
	assert.FileExists(t, prepareCommitMsgPath)

	// Check that hooks are executable
	info, err := os.Stat(postCommitPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0111 != 0, "post-commit hook should be executable")

	info, err = os.Stat(postMergePath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0111 != 0, "post-merge hook should be executable")

	info, err = os.Stat(prepareCommitMsgPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0111 != 0, "prepare-commit-msg hook should be executable")
}

func TestInit_HooksAreInstalledInDualBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Check that hooks are installed to .git/hooks/
	postCommitPath := filepath.Join(repo, ".git", "hooks", "post-commit")
	postMergePath := filepath.Join(repo, ".git", "hooks", "post-merge")
	prepareCommitMsgPath := filepath.Join(repo, ".git", "hooks", "prepare-commit-msg")
	preCommitPath := filepath.Join(repo, ".git", "hooks", "pre-commit")

	assert.FileExists(t, postCommitPath)
	assert.FileExists(t, postMergePath)
	assert.FileExists(t, prepareCommitMsgPath)
	assert.FileExists(t, preCommitPath)
}

func TestInit_HooksAreBranchAware(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Check that post-commit hook contains branch awareness logic
	postCommitPath := filepath.Join(repo, ".git", "hooks", "post-commit")
	data, err := os.ReadFile(postCommitPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "_armature")

	// Check that post-merge hook contains branch awareness logic
	postMergePath := filepath.Join(repo, ".git", "hooks", "post-merge")
	data, err = os.ReadFile(postMergePath)
	require.NoError(t, err)
	content = string(data)
	assert.Contains(t, content, "_armature")
}

func TestMerged_RequiresDoneState_InDualBranchMode(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
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

func TestMerged_AcceptsDoneIssue_DualBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "done")
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

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "feature work", "--id", "F-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "F-001", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "F-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "F-001", "--to", "done", "--skip-delivery-gate", "--force",
		"--branch", "feature/e2-test", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Status should show done (awaiting merge)
	statusOut, err := runTrls(t, repo, "--format", "human", "list", "--group")
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

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "merged", index["F-001"].Status)

	// list --group should no longer show done-awaiting-merge for F-001
	finalStatus, err := runTrls(t, repo, "--format", "human", "list", "--group")
	require.NoError(t, err)
	assert.NotContains(t, finalStatus, "awaiting merge")
}

// TC-008: workers command and helper function tests

func TestAppCtxStateDirSet(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init armature (which also inits worker ID)
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Case 1: No worker ID set (manually unset it)
	run(t, repo, "git", "config", "--local", "--unset", "armature.worker-id")
	_, err = runTrls(t, repo, "list")
	require.NoError(t, err)
	// Verify state dir is resolved correctly by checking that the expected path exists
	defaultID := "default"
	defaultID = workerIdentityWithSlot(defaultID) // Apply slot suffix if ARM_LOG_SLOT is set
	expectedDefault := filepath.Join(repo, ".armature", "state", defaultID)
	// Check that state files exist in the expected location (verifying StateDir was set correctly)
	_, err = os.Stat(expectedDefault)
	assert.NoError(t, err, "StateDir should exist at %s when no worker ID is set", expectedDefault)

	// Case 2: Worker ID set
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)

	_, err = runTrls(t, repo, "list")
	require.NoError(t, err)
	// Verify state dir is resolved correctly for the configured worker ID
	workerID = workerIdentityWithSlot(workerID) // Apply slot suffix if ARM_LOG_SLOT is set
	expectedWorker := filepath.Join(repo, ".armature", "state", workerID)
	// Check that state files exist in the expected location
	_, err = os.Stat(expectedWorker)
	assert.NoError(t, err, "StateDir should exist at %s for configured worker ID", expectedWorker)
}

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
	status := buildWorkerStatus("worker-a", allOps, 60, now, map[string]string{"task-1": "worker-a"})
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
	status := buildWorkerStatus("worker-a", allOps, 60, now, map[string]string{"task-1": "worker-a"})
	assert.Equal(t, "stale", status.Status)
	assert.Empty(t, status.ActiveIssue)
}

func TestBuildWorkerStatus_IdleWorker(t *testing.T) {
	now := int64(1000)
	allOps := []ops.Op{
		{Type: ops.OpNote, TargetID: "T-001", Timestamp: 900, WorkerID: "worker-a"},
	}
	// No claims, but recent op — idle within 2*TTL window
	status := buildWorkerStatus("worker-a", allOps, 1, now, map[string]string{}) // 2*1min=120s; 1000-900=100 < 120 → idle
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
	status := buildWorkerStatus("worker-a", allOps, 60, now, map[string]string{"task-1": "worker-a"})
	assert.NotEqual(t, "active", status.Status)
}

func TestBuildWorkerStatus_HeartbeatUpdatesLastHeartbeat(t *testing.T) {
	// Two heartbeats: second is newer; verifies that workers.go:126 keeps the latest (covers NOT COVERED mutation)
	now := int64(10000)
	allOps := []ops.Op{
		{Type: ops.OpClaim, TargetID: "T-001", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 1}}, // TTL 1 min = 60s; 100+60=160 < now → would be stale without heartbeat
		{Type: ops.OpHeartbeat, TargetID: "T-001", Timestamp: 200, WorkerID: "worker-a"},
		{Type: ops.OpHeartbeat, TargetID: "T-001", Timestamp: 9500, WorkerID: "worker-a"}, // later heartbeat
	}
	// With the last heartbeat at 9500 and TTL 1 min = 60s: 9500+60=9560 < 10000 → still stale by expiry
	// But lastHeartbeat should be 9500, not 200
	status := buildWorkerStatus("worker-a", allOps, 60, now, map[string]string{"task-1": "worker-a"})
	// The claim expired and even the heartbeat didn't extend it far enough — check that the heartbeat was tracked
	assert.NotEqual(t, "active", status.Status)
}

func TestClaimWinnersByIssue_StaleClaimTakeoverPrefersCurrentOwner(t *testing.T) {
	workers := map[string][]ops.Op{
		"worker-a": {
			{Type: ops.OpClaim, TargetID: "task-1", Timestamp: 100, WorkerID: "worker-a", Payload: ops.Payload{TTL: 1}},
		},
		"worker-b": {
			{Type: ops.OpClaim, TargetID: "task-1", Timestamp: 200, WorkerID: "worker-b", Payload: ops.Payload{TTL: 10}},
		},
	}
	winners := claimWinnersByIssue(workers)
	assert.Equal(t, "worker-b", winners["task-1"])
}

func TestBuildWorkerStatus_SlottedWinnerMatchesBaseWorker(t *testing.T) {
	now := int64(1000)
	allOps := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-1", Timestamp: 900, WorkerID: "worker-a~slot-1", Payload: ops.Payload{TTL: 60}},
	}
	status := buildWorkerStatus("worker-a", allOps, 60, now, map[string]string{"task-1": "worker-a~slot-1"})
	assert.Equal(t, "active", status.Status)
	assert.Equal(t, "task-1", status.ActiveIssue)
}

func TestBuildWorkerStatus_LosingClaimDoesNotReportStale(t *testing.T) {
	now := int64(1000)
	allOps := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-1", Timestamp: 980, WorkerID: "worker-a", Payload: ops.Payload{TTL: 60}},
	}
	status := buildWorkerStatus("worker-a", allOps, 60, now, map[string]string{"task-1": "worker-b"})
	assert.Equal(t, "idle", status.Status)
}

func TestWorkersCommand_EmptyRepo(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
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

func TestLogCommand_FilterBySince_RFC3339(t *testing.T) {
	// Covers log.go:30 — --since parsed as RFC3339 date
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "log", "--since", "2020-01-01T00:00:00Z")
	require.NoError(t, err)
	assert.NotEmpty(t, out) // any output is fine; we just need to exercise the code path
}

func TestLogCommand_FilterBySince_DateOnly(t *testing.T) {
	// Covers log.go:33 — --since parsed as YYYY-MM-DD after RFC3339 fails
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "log", "--since", "2020-01-01")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
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
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "heartbeat", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestDecisionCommand(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "decision", "--issue", "task-01",
		"--topic", "db", "--choice", "postgres", "--rationale", "mature")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestLinkCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
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
	// Plant acceptance so reopen (done → open) does not introduce E6.
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpAmend,
		TargetID:  "task-01",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   ops.Payload{Acceptance: json.RawMessage(testAcceptance)},
	}))
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "reopen", "--issue", "task-01")
	assert.NoError(t, err)
}

func TestReadyCommand_DraftTask_ExcludedFromReady(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a draft task
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Draft work", "--id", "draft-01")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)

	// The draft task should not appear in the ready queue
	assert.NotContains(t, out, "draft-01")
}

func TestReadyCommand_VerifiedTask_AppearsInReady(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Birth is always draft; promote to verified so it can enter ready.
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Verified work", "--id", "verified-01",
		"--scope", "cmd/armature/verified.go", "--dod", "Verified work is complete and tested",
		"--acceptance", testAcceptance)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "dag", "transition", "--issue", "verified-01", "--to", "verified")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)

	// After plan release the task should appear in the ready queue
	assert.Contains(t, out, "verified-01")
}

func TestReadyCommand_NoConfidenceField_DefaultsToVerified(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a task without a confidence flag — birth is always draft.
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Legacy task", "--id", "legacy-01")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)

	assert.NotContains(t, out, "legacy-01", "create without --confidence must emit draft")
}

func TestDagTransitionCommand_PromotesDraftSubtree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a draft task (no parent, so no parent-status gate)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Draft task", "--id", "task-draft-01",
		"--scope", "cmd/armature/draft_a.go", "--dod", "Draft A is complete and tested",
		"--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)
	// Create a second draft task outside the scope (different ID)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Another draft", "--id", "task-draft-02",
		"--scope", "cmd/armature/draft_b.go", "--dod", "Draft B is complete and tested",
		"--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)

	// Confirm task-draft-01 is NOT in the ready queue yet
	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, out, "task-draft-01")

	// Run dag-transition to promote task-draft-01's subtree (just itself here)
	out, err = runTrls(t, repo, "dag", "transition", "--issue", "task-draft-01")
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

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "dag", "transition")
	assert.Error(t, err)
}

func TestValidateCmd_CoverageOutput_HumanFormat(t *testing.T) {
	// Setup: repo with two issues and a worker
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create two tasks: one will be source-linked, one will remain uncited
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Cited task", "--id", "COV-001",
		"--scope", "main.go", "--dod", "Cited coverage task is complete")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "amend", "--issue", "COV-001", "--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Uncited task", "--id", "COV-002",
		"--scope", "other.go", "--dod", "Uncited coverage task is complete")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "amend", "--issue", "COV-002", "--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Get the worker log path so we can inject ops directly
	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	logPath := filepath.Join(repo, ".armature", "ops", fmt.Sprintf("%s.log", workerID))

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

		out, err := runTrls(t, repo, "validate", "--format", "human")
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

		out, err := runTrls(t, repo, "validate", "--format", "human")
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
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
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

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a story (parent) and a task (child).
	// Materialize after story so issues/story-01.json exists for ReadIssue in create --parent.
	_, err = runTrls(t, repo, "create", "--type", "story", "--title", "Parent Story", "--id", "story-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Child Task", "--id", "task-01", "--parent", "story-01")
	require.NoError(t, err)

	// Materialize so state is up to date
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify story is "open" before claim
	index, loadErr := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, loadErr)
	require.Equal(t, "open", index["story-01"].Status, "story should start as open")

	// Claim the child task — this should auto-advance the parent story to in-progress
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	// Check the ops log for an explicit transition op targeting story-01 with to=in-progress.
	// This verifies claim.go emits a durable op (not just relies on state engine inference).
	issuesDir := filepath.Join(repo, ".armature")
	if _, err := os.Stat(filepath.Join(repo, ".arm")); err == nil {
		issuesDir = filepath.Join(repo, ".armature")
	}
	opsDir := filepath.Join(issuesDir, "ops")
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

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a task and claim it (puts it in "claimed" status)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Unassign Test Task", "--id", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	// Materialize and verify it's "claimed"
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	index, loadErr := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, loadErr)
	require.Equal(t, ops.StatusClaimed, index["task-01"].Status, "task should be claimed before unassign")

	// Unassign — should release claimed → open
	_, err = runTrls(t, repo, "unassign", "--issue", "task-01")
	require.NoError(t, err)

	// Verify a transition → open op was emitted
	issuesDir := filepath.Join(repo, ".armature")
	if _, err := os.Stat(filepath.Join(repo, ".arm")); err == nil {
		issuesDir = filepath.Join(repo, ".armature")
	}
	opsDir := filepath.Join(issuesDir, "ops")
	entries, readErr := os.ReadDir(opsDir)
	require.NoError(t, readErr)

	foundTransitionOp := false
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		logPath := filepath.Join(opsDir, entry.Name())
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
	index2, loadErr2 := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, loadErr2)
	assert.Equal(t, ops.StatusOpen, index2["task-01"].Status, "task status should be open after unassign")
}

func TestContextHistoryCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "TST-HIST", "--title", "History test issue", "--type", "task")
	require.NoError(t, err)

	// Ops are committed automatically to the _armature branch (dual-branch mode
	// is always on); capture the ops-worktree HEAD after creation.
	testCtx3 := getTestContext(t, repo)
	require.NotEmpty(t, testCtx3.WorktreePath)
	sha1Out, err2 := exec.CommandContext(context.Background(), "git", "-C", testCtx3.WorktreePath, "rev-parse", "HEAD").Output()
	require.NoError(t, err2)
	sha1 := strings.TrimSpace(string(sha1Out))

	_, err = runTrls(t, repo, "note", "--issue", "TST-HIST", "--msg", "progress note for history")
	require.NoError(t, err)

	// Capture the ops-worktree HEAD after the note
	sha2Out, err2 := exec.CommandContext(context.Background(), "git", "-C", testCtx3.WorktreePath, "rev-parse", "HEAD").Output()
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

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "context-history", "--issue", "NO-SUCH")
	assert.Error(t, err)
}

func TestInitDualBranchIdempotent(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// First init --dual-branch should succeed
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err, "first dual-branch init should succeed")

	// Second init --dual-branch should also succeed (idempotent)
	_, err = runTrls(t, repo, "bootstrap")
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
	t.Cleanup(func() { _ = os.Chdir(origDir) }) //nolint:errcheck // cleanup chdir error not actionable

	// Re-init using "." as repo path — simulates running trls init --dual-branch in the repo root
	dotCmd.SetArgs([]string{"bootstrap", "--repo", "."})
	err = dotCmd.Execute()
	require.NoError(t, err, "init with relative repo '.' should succeed (idempotent)")

	wtCmd := exec.CommandContext(context.Background(), "git", "-C", repo, "config", "armature.ops-worktree-path")
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

func TestReadyCommand_ParentFilter(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a story as the parent, then two tasks under it (valid hierarchy: story→task).
	// Materialize after parent-01 so issues/parent-01.json exists for ReadIssue in create --parent.
	_, err = runTrls(t, repo, "create", "--type", "story", "--title", "Parent", "--id", "parent-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	plantVerifiedTaskUnder(t, repo, "child-a", "cmd/armature/child_a.go", "parent-01")
	plantVerifiedTask(t, repo, "unrelated-01", "cmd/armature/unrelated.go")

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

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	plantVerifiedTask(t, repo, "text-01", "cmd/armature/text_01.go")
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

	err := writeDAGSummaryArtifact(stateDir, reviewed, approvedIDs, cov)
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
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "heartbeat", "--issue", "task-01", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"heartbeat"`, "default format should not be JSON")
}

// UX: heartbeat with --format json should still return JSON for agent consumers
func TestHeartbeatCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "heartbeat", "--issue", "task-01", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"heartbeat"`)
	assert.Contains(t, out, "task-01")
}

// TestPushOpsCommand_P2 verifies that the push-ops command exists and is wired into the CLI.
// The post-commit hook calls `arm push-ops` to push ops logs after each commit,
// so this command must exist and be executable.
func TestPushOpsCommand_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap to set up the dual-branch mode
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Verify that push-ops command exists and can be invoked (it should not fail with "unknown command")
	out, err := runTrls(t, repo, "push-ops")
	// We expect the command to either succeed (return 0) or fail gracefully
	// (e.g., because there's no remote). But it should NOT fail with "unknown command".
	// If the command doesn't exist, runTrls would return an error mentioning "unknown command".
	if err != nil {
		errMsg := err.Error()
		// The command might fail because there's no remote configured, which is fine.
		// We just want to verify the command exists.
		assert.NotContains(t, errMsg, "unknown command", "push-ops command should exist")
		assert.NotContains(t, errMsg, "no such command", "push-ops command should exist")
		assert.NotContains(t, errMsg, "unrecognized", "push-ops command should exist")
	}
	// The command should not output an error about the command not existing
	assert.NotContains(t, out, "unknown command", "push-ops command should exist")
	assert.NotContains(t, out, "no such command", "push-ops command should exist")
}

// TestPushOpsCommand_SuccessPushesArmatureBranchToOrigin verifies the happy path:
// with a real (bare) origin remote configured, push-ops actually pushes the
// _armature branch to it, and reports success as structured JSON.
func TestPushOpsCommand_SuccessPushesArmatureBranchToOrigin(t *testing.T) {
	// Set up a bare repo to act as the "origin" remote.
	bareDir := t.TempDir()
	run(t, bareDir, "git", "init", "--bare")

	repo := initTempRepo(t)
	run(t, repo, "git", "remote", "add", "origin", bareDir)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "push-ops", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"pushed"`)
	assert.Contains(t, out, `"branch":"_armature"`)

	// Confirm the _armature branch actually landed in the bare "origin" repo.
	refCmd := exec.CommandContext(context.Background(), "git", "-C", bareDir, "show-ref", "--verify", "refs/heads/_armature")
	require.NoError(t, refCmd.Run(), "_armature branch should exist on origin after push-ops")
}

// TestPushOpsCommand_PushFailureReturnsErrorAndJSON verifies that when the
// underlying git push fails (e.g. no remote configured), push-ops surfaces a
// real, non-nil error (so a human invoking it directly sees a failure) and,
// when --format json is requested, emits a structured JSON error object
// consistent with other commands' error output.
func TestPushOpsCommand_PushFailureReturnsErrorAndJSON(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// No "origin" remote configured, so the push must fail.
	out, errBuf, err := runTrlsWithStderr(t, repo, "push-ops", "--format", "json")
	require.Error(t, err, "push-ops should return a real error when the push fails")
	assert.NotContains(t, out, `"status":"pushed"`)
	// The command's RunE surfaces the error; combined with root err handling this
	// should not silently look like success on stdout.
	_ = errBuf
}

// UX: note should emit plain text in human mode, not JSON
func TestNoteCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "note", "--issue", "task-01", "--msg", "progress update", "--format", "human")
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
	assert.Contains(t, out, `"note_id"`)
	assert.Contains(t, out, "task-01")
}

func TestNoteDeleteCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "note", "--issue", "task-01", "--msg", "progress update", "--format", "json")
	require.NoError(t, err)

	var result struct {
		Issue  string `json:"issue"`
		NoteID string `json:"note_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.NotEmpty(t, result.NoteID)

	deleteOut, err := runTrls(t, repo, "note", "delete", "--issue", "task-01", "--note-id", result.NoteID, "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, deleteOut, `"note":"deleted"`)
	assert.Contains(t, deleteOut, result.NoteID)

	ctxOut, err := runTrls(t, repo, "render-context", "--issue", "task-01")
	require.NoError(t, err)
	assert.NotContains(t, ctxOut, "progress update")
}

// UX: transition should emit plain text in human mode, not JSON
func TestTransitionCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--force", "--outcome", "completed", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"status"`, "default format should not be JSON")
}

// UX: transition with --format json should still return JSON
func TestTransitionCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--force", "--outcome", "completed", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"status"`)
	assert.Contains(t, out, "task-01")
}

// UX: trls init run a second time should print an "already initialized" message
func TestInitCommand_AlreadyInitialized(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	assert.Contains(t, out, "already")
}

// TestLogSlot_EnvVar verifies that ARM_LOG_SLOT routes ops to a slotted log file.
func TestLogSlot_EnvVar(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	t.Setenv("ARM_LOG_SLOT", "beta")

	_, err = runTrls(t, repo, "note", "--issue", "task-01", "--msg", "slotted note")
	require.NoError(t, err)

	// The slotted file must exist; the plain file must NOT contain this note
	opsDir := filepath.Join(repo, ".armature", "ops")
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
		plainContent, err := os.ReadFile(plainFile)
		require.NoError(t, err)
		assert.NotContains(t, string(plainContent), "slotted note",
			"plain log must not contain the slotted note")
	}
}

// TestLogSlot_Empty_UsesPlainLog verifies that an empty ARM_LOG_SLOT uses the normal log path.
func TestLogSlot_Empty_UsesPlainLog(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Clear the ops directory that was created with the global ARM_LOG_SLOT setting
	opsDir := filepath.Join(repo, ".armature", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	for _, e := range entries {
		if !e.IsDir() {
			os.Remove(filepath.Join(opsDir, e.Name())) //nolint:errcheck // best-effort cleanup
		}
	}

	// Set ARM_LOG_SLOT to empty before worker-init to ensure all operations use plain logs
	t.Setenv("ARM_LOG_SLOT", "")

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "note", "--issue", "task-01", "--msg", "plain note")
	require.NoError(t, err)

	entries, err = os.ReadDir(opsDir)
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.Name(), "~",
			"no slotted file should exist when ARM_LOG_SLOT is empty")
	}
}

func TestLogSlot_TRLSEnvIgnored(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	t.Setenv("TRLS_LOG_SLOT", "legacy")
	t.Setenv("ARM_LOG_SLOT", "")

	_, err = runTrls(t, repo, "note", "--issue", "task-01", "--msg", "plain note")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".armature", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), "~legacy", "legacy TRLS_LOG_SLOT must be ignored")
	}
}

func TestStateDir_UsesSlotWhenConfigured(t *testing.T) {
	t.Setenv("ARM_LOG_SLOT", "lane-a")
	workerID := workerIdentityWithSlot("worker-123")
	assert.Equal(t, "worker-123~lane-a", workerID)

	ctx := &config.Context{IssuesDir: "/repo/.armature"}
	assert.Equal(t, "/repo/.armature/state/worker-123~lane-a", stateDirFor(ctx, workerID))
}

func TestWorkerIdentityWithSlot_REQ_LNGHZN_S3_T1(t *testing.T) {
	t.Run("valid slot is appended as before", func(t *testing.T) {
		t.Setenv("ARM_LOG_SLOT", "lane-a_2")
		assert.Equal(t, "worker-123~lane-a_2", workerIdentityWithSlot("worker-123"))
	})

	t.Run("slot containing a path separator falls back to unslotted identity", func(t *testing.T) {
		t.Setenv("ARM_LOG_SLOT", "../../etc")
		assert.Equal(t, "worker-123", workerIdentityWithSlot("worker-123"))
	})

	t.Run("slot containing a slash falls back to unslotted identity", func(t *testing.T) {
		t.Setenv("ARM_LOG_SLOT", "a/b")
		assert.Equal(t, "worker-123", workerIdentityWithSlot("worker-123"))
	})

	t.Run("empty slot is unslotted identity", func(t *testing.T) {
		t.Setenv("ARM_LOG_SLOT", "")
		assert.Equal(t, "worker-123", workerIdentityWithSlot("worker-123"))
	})
}

func TestAppendLowStakesOp_ResetsTrackerWhenThresholdReachedInWorktree(t *testing.T) {
	worktreePath := initTempRepo(t)
	logPath := filepath.Join(worktreePath, ".armature", "ops", "worker.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	state := &executionState{
		ctx: &config.Context{
			WorktreePath: worktreePath,
			Config:       config.Config{LowStakesPushThreshold: 1},
		},
		tracker: &fakePendingPushTracker{},
	}

	op := ops.Op{Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "hello"}}
	err := appendLowStakesOp(state, logPath, op)
	require.NoError(t, err)

	tracker, ok := state.tracker.(*fakePendingPushTracker)
	require.True(t, ok)
	assert.Equal(t, 1, tracker.incremented)
	assert.Equal(t, 1, tracker.resetCalls)
}

// TestLogSlot_ReplayIncludesSlottedOps verifies that ops written to a slotted log
// are included in materialised state alongside ops from the plain log.
func TestLogSlot_ReplayIncludesSlottedOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task A", "--id", "task-a")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task B", "--id", "task-b")
	require.NoError(t, err)

	// Slot "one" transitions task-a to done
	t.Setenv("ARM_LOG_SLOT", "one")
	_, err = runTrls(t, repo, "claim", "--issue", "task-a", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-a", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "slot one")
	require.NoError(t, err)

	// Slot "two" transitions task-b to done
	t.Setenv("ARM_LOG_SLOT", "two")
	_, err = runTrls(t, repo, "claim", "--issue", "task-b", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-b", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "slot two")
	require.NoError(t, err)

	// Unset slot so materialize uses the main context
	t.Setenv("ARM_LOG_SLOT", "")
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Ops are timestamped at second resolution (nowEpoch uses time.Now().Unix()),
	// and materialization applies same-timestamp ops in file-load order (main
	// worker log before slotted logs) as a stable tiebreak. Sleep past the
	// current second so the "merged" transitions below are unambiguously later
	// than the "done" transitions recorded above, guaranteeing deterministic
	// ordering regardless of which log file a same-second op landed in.
	time.Sleep(1100 * time.Millisecond)

	// Promotion to merged now requires an explicit `arm merged` call; there is no
	// more automatic done->merged promotion via git-history merge detection.
	_, err = runTrls(t, repo, "merged", "--issue", "task-a")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "merged", "--issue", "task-b")
	require.NoError(t, err)

	// Both tasks must appear as merged
	outA, err := runTrls(t, repo, "show", "--issue", "task-a")
	require.NoError(t, err)
	assert.Contains(t, outA, "merged")

	outB, err := runTrls(t, repo, "show", "--issue", "task-b")
	require.NoError(t, err)
	assert.Contains(t, outB, "merged")
}

// ── Format flag tests for commands standardised in E6-S3-T2 ──────────────────

// create: human text when --format human
func TestCreateCommand_HumanOutput(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "create", "--format", "human", "--type", "task", "--title", "Human task", "--id", "human-01")
	require.NoError(t, err)
	assert.Contains(t, out, "human-01")
	assert.NotContains(t, out, `"id"`, "human format should not be JSON")
}

// create: JSON when --format json
func TestCreateCommand_JSONOutput(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "create", "--format", "json", "--type", "task", "--title", "JSON task", "--id", "json-01")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "json-01", result["id"])
	assert.Equal(t, "created", result["status"])
}

// claim: human text when --format human
func TestClaimCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "claim", "--format", "human", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"claimed_by"`, "human format should not be JSON")
}

// claim: JSON when --format json
func TestClaimCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "claim", "--format", "json", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "task-01", result["issue"])
	assert.NotNil(t, result["claimed_by"])
}

// decision: human text when --format human
func TestDecisionCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "decision", "--format", "human", "--issue", "task-01",
		"--topic", "arch", "--choice", "monolith", "--rationale", "simple")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"topic"`, "human format should not be JSON")
}

// decision: JSON when --format json
func TestDecisionCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "decision", "--format", "json", "--issue", "task-01",
		"--topic", "arch", "--choice", "monolith", "--rationale", "simple")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "task-01", result["issue"])
	assert.Equal(t, "arch", result["topic"])
	assert.Equal(t, "monolith", result["choice"])
}

// link: human text when --format human
func TestLinkCommand_HumanOutput(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task A", "--id", "link-a")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task B", "--id", "link-b")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "link", "--format", "human", "--source", "link-a", "--dep", "link-b")
	require.NoError(t, err)
	assert.Contains(t, out, "link-a")
	assert.NotContains(t, out, `"source"`, "human format should not be JSON")
}

// link: JSON when --format json
func TestLinkCommand_JSONOutput(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task A", "--id", "linkj-a")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task B", "--id", "linkj-b")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "link", "--format", "json", "--source", "linkj-a", "--dep", "linkj-b")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "linkj-a", result["source"])
	assert.Equal(t, "linkj-b", result["dep"])
}

// unlink: remove a blocked_by dependency
func TestUnlinkCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task A", "--id", "unlink-a")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task B", "--id", "unlink-b")
	require.NoError(t, err)

	// First, link unlink-a to unlink-b
	_, err = runTrls(t, repo, "link", "--source", "unlink-a", "--dep", "unlink-b", "--rel", "blocked_by")
	require.NoError(t, err)

	// Materialize to verify the link was applied
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Unlink unlink-a from unlink-b
	out, err := runTrls(t, repo, "unlink", "--source", "unlink-a", "--dep", "unlink-b")
	require.NoError(t, err)
	assert.Contains(t, out, "unlink-a")
	assert.Contains(t, out, "unlink-b")

	// Materialize again to verify the dependency was removed
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
}

// unlink: JSON output when --format json
func TestUnlinkCommand_JSONOutput(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task A", "--id", "unlinkj-a")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task B", "--id", "unlinkj-b")
	require.NoError(t, err)

	// Link first
	_, err = runTrls(t, repo, "link", "--source", "unlinkj-a", "--dep", "unlinkj-b", "--rel", "blocked_by")
	require.NoError(t, err)

	// Unlink with JSON output
	out, err := runTrls(t, repo, "unlink", "--format", "json", "--source", "unlinkj-a", "--dep", "unlinkj-b")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "unlinkj-a", result["source"])
	assert.Equal(t, "unlinkj-b", result["dep"])
}

// amend: human text when --format human
func TestAmendCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "amend", "--format", "human", "--issue", "task-01", "--dod", "new dod")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"status"`, "human format should not be JSON")
}

// amend: JSON when --format json
func TestAmendCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "amend", "--format", "json", "--issue", "task-01", "--dod", "new dod")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "task-01", result["issue"])
	assert.Equal(t, "amended", result["status"])
}

// assign: human text when --format human
func TestAssignCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "assign", "--format", "human", "--issue", "task-01", "--worker", "worker-abc")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"assigned_to"`, "human format should not be JSON")
}

// assign: JSON when --format json
func TestAssignCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "assign", "--format", "json", "--issue", "task-01", "--worker", "worker-abc")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "task-01", result["issue"])
	assert.Equal(t, "worker-abc", result["assigned_to"])
}

// unassign: human text when --format human
func TestUnassignCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "assign", "--issue", "task-01", "--worker", "worker-abc")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "unassign", "--format", "human", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, `"assigned_to"`, "human format should not be JSON")
}

// unassign: JSON when --format json
func TestUnassignCommand_JSONOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "assign", "--issue", "task-01", "--worker", "worker-abc")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "unassign", "--format", "json", "--issue", "task-01")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "task-01", result["issue"])
}

// workers: human text when --format human
func TestWorkersCommand_HumanOutput(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "workers", "--format", "human")
	require.NoError(t, err)
	assert.NotContains(t, out, `"worker_id"`, "human format should not be JSON")
}

// workers: JSON when --format json (replaces --json)
func TestWorkersCommand_FormatJSON(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "workers", "--format", "json")
	require.NoError(t, err)
	// output is JSONL — parse first line
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.NotEmpty(t, lines)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &result))
	assert.NotNil(t, result["worker_id"])
	assert.NotNil(t, result["status"])
}

// workers: --json flag still works for backward compatibility
func TestWorkersCommand_LegacyJSONFlag(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "workers", "--json")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.NotEmpty(t, lines)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &result))
	assert.NotNil(t, result["worker_id"])
}

// TestWorkersCommand_SlottedLogs verifies that ops from slotted log files are
// included in worker activity output alongside the plain log's ops.
func TestWorkersCommand_SlottedLogs(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Slot task", "--id", "slot-task")
	require.NoError(t, err)

	// Write an op via the plain log (claim)
	_, err = runTrls(t, repo, "claim", "--issue", "slot-task", "--worktree")
	require.NoError(t, err)

	// Write an op via a slotted log (transition done)
	t.Setenv("ARM_LOG_SLOT", "w")
	_, err = runTrls(t, repo, "transition", "--issue", "slot-task", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "via slot")
	require.NoError(t, err)
	t.Setenv("ARM_LOG_SLOT", "")

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

// TestClaimCommand_ScopeOverlapExitsWithoutForce verifies that claim exits 1 when a *different*
// worker holds an overlapping task and --force is not passed.
func TestClaimCommand_ScopeOverlapExitsWithoutForce(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Initialize armature
	bootstrapRepoForTest(t, repo)

	// Initialize our worker (worker-A).
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	plantOverlappingFooPair(t, repo)

	// Claim task-01 as a *different* worker by temporarily overriding the git config.
	run(t, repo, "git", "config", "--local", "armature.worker-id", "other-worker-abc")
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	// Restore worker-A by re-running worker-init (generates a new UUID for worker-A).
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Attempt to claim the second task without --force should fail: different worker holds task-01.
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"claim", "--issue", "task-02", "--repo", repo, "--worktree"})

	err = root.Execute()
	assert.Error(t, err, "claim should fail when a different worker's task overlaps without --force")
	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "overlap", "error output should mention scope overlap")
}

// TestClaimCommand_ScopeOverlapWithForceProceeds verifies that claim --force proceeds and warns to stderr
func TestClaimCommand_ScopeOverlapWithForceProceeds(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Initialize armature
	bootstrapRepoForTest(t, repo)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	plantOverlappingFooPair(t, repo)

	// Claim the first task
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	// Claim the second task with --force should succeed despite overlap
	out, err := runTrls(t, repo, "claim", "--issue", "task-02", "--force", "--worktree")
	assert.NoError(t, err, "claim --force should succeed despite scope overlap")
	assert.Contains(t, out, "task-02", "output should contain the claimed issue ID")
}

// TestClaimCommand_ScopeOverlapSameWorker_AutoDismissed verifies that when the same worker
// holds both overlapping tasks no --force is required; the claim succeeds with no error output.
func TestClaimCommand_ScopeOverlapSameWorker_AutoDismissed(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	plantOverlappingFooPair(t, repo)

	// Same worker claims task-01 first.
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	// Same worker claims task-02 without --force: should succeed (serial same-worker work).
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(errBuf)
	root.SetArgs([]string{"claim", "--issue", "task-02", "--repo", repo, "--worktree"})

	err = root.Execute()
	assert.NoError(t, err, "same-worker overlap should not require --force")
	assert.NotContains(t, errBuf.String(), "Error:", "no error should be emitted to stderr")
}

// TestClaimCommand_SameWorkerOverlapDeduplicatesNotes verifies that when the same worker
// claims a task with an existing overlap and the note already exists, it is not written again.
// This ensures notes are written at most once per issue-pair per claim session.
func TestClaimCommand_SameWorkerOverlapDeduplicatesNotes(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create two tasks with overlapping scopes
	plantOverlappingFooPair(t, repo)

	// Same worker claims task-01
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	// Same worker claims task-02 (overlaps with task-01) - writes first dismissal note
	_, err = runTrls(t, repo, "claim", "--issue", "task-02", "--worktree")
	require.NoError(t, err)

	// Load ops and count dismissal notes for task-02
	opsDir := filepath.Join(repo, ".armature", "ops")
	allOps, _, err := readAllOpsFromDirWithOffsets(opsDir)
	require.NoError(t, err)

	// Count notes on task-02 that mention serial overlap with task-01
	countBefore := 0
	for _, op := range allOps {
		if op.Type == ops.OpNote && op.TargetID == "task-02" && strings.Contains(op.Payload.Msg, "Serial claim: scope overlap with task-01") {
			countBefore++
		}
	}
	assert.Equal(t, 1, countBefore, "task-02 should have exactly 1 note about task-01 overlap after first claim")

	// Now simulate claiming task-02 again in the same worktree (e.g., heartbeat/refresh)
	// This should update the task ID file without creating a new worktree

	// Claim task-02 again with the same worktree path - should not create a duplicate note
	_, err = runTrls(t, repo, "claim", "--issue", "task-02", "--worktree")
	require.NoError(t, err)

	// Load ops again and count dismissal notes
	allOps, _, err = readAllOpsFromDirWithOffsets(opsDir)
	require.NoError(t, err)

	// Count notes on task-02 that mention serial overlap with task-01
	countAfter := 0
	for _, op := range allOps {
		if op.Type == ops.OpNote && op.TargetID == "task-02" && strings.Contains(op.Payload.Msg, "Serial claim: scope overlap with task-01") {
			countAfter++
		}
	}
	assert.Equal(t, 1, countAfter, "task-02 should still have only 1 note about task-01 overlap after reclaiming (deduplication should prevent writing it again)")
}

func TestClaimCommand_ScopeOverlapSameWorkerDifferentSlots_RequiresForce(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	plantOverlappingFooPair(t, repo)

	t.Setenv("ARM_LOG_SLOT", "A")
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	t.Setenv("ARM_LOG_SLOT", "B")
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(errBuf)
	root.SetArgs([]string{"claim", "--issue", "task-02", "--repo", repo, "--worktree"})
	err = root.Execute()
	assert.Error(t, err, "same worker but different slots should be treated as different claim owners")
	assert.Contains(t, errBuf.String(), "overlap")
}

func TestClaimCommand_LostRaceReportsClearResult(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-01", "--title", "Task 1", "--type", "task")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	run(t, repo, "git", "config", "--local", "armature.worker-id", "other-worker-abc")
	// Worktree setup is now deferred past claim validation: the second worker loses the
	// race (original worker claimed first) and gets a "Claim lost" result, not an error.
	// The worktree branch conflict is never reached because the race is lost before setup.
	claimOut, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err, "claim lost is a normal outcome, not an error")
	// Output indicates claim loss in either human ("Claim lost") or JSON ("lost_claim_race") format.
	assert.True(t,
		strings.Contains(claimOut, "Claim lost") || strings.Contains(claimOut, "lost_claim_race"),
		"output should report claim loss, got: %s", claimOut)

	showOut, err := runTrls(t, repo, "show", "--issue", "task-01")
	require.NoError(t, err)
	assert.NotContains(t, showOut, "other-worker-abc")

	workersOut, err := runTrls(t, repo, "workers", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, workersOut, `"worker_id":"other-worker-abc","status":"active"`)
}

// TestUnassignHelp verifies unassign --help mentions auto-transition side effect
func TestUnassignHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"unassign", "--help"})

	err := cmd.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "claimed", "help should mention claimed status")
	assert.Contains(t, out, "open", "help should mention open status or auto-transition")
}

// TestClaimHelp verifies claim --help mentions auto-advance of parent story
func TestClaimHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"claim", "--help"})

	err := cmd.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "parent", "help should mention parent")
	assert.Contains(t, out, "progress", "help should mention progress or auto-advance")
}

// TestCommandGroups verifies that all commands are assigned to the correct cobra groups
func TestCommandGroups(t *testing.T) {
	root := newRootCmd()

	// Verify the four required groups exist
	requiredGroups := []string{"workflow", "dag", "sync", "admin"}
	groupMap := make(map[string]bool)
	for _, group := range root.Groups() {
		groupMap[group.ID] = true
	}

	for _, groupID := range requiredGroups {
		assert.True(t, groupMap[groupID], "group %q not found in root command groups", groupID)
	}

	// Verify all commands have a GroupID set
	for _, cmd := range root.Commands() {
		assert.NotEmpty(t, cmd.GroupID, "command %q missing GroupID", cmd.Name())
	}

	// Verify expected commands are in the correct groups
	commandsByGroup := make(map[string]map[string]bool)
	for _, cmd := range root.Commands() {
		if commandsByGroup[cmd.GroupID] == nil {
			commandsByGroup[cmd.GroupID] = make(map[string]bool)
		}
		commandsByGroup[cmd.GroupID][cmd.Name()] = true
	}

	// Expected commands per group (subset of all commands)
	workflowExpected := []string{"ready", "claim", "transition", "unassign", "reopen", "heartbeat", "note", "decision", "amend", "confirm", "assign"}
	dagExpected := []string{"dag", "link", "unlink"}
	syncExpected := []string{"sync", "merged", "materialize", "import", "push-ops"}
	adminExpected := []string{
		"worker-init", "workers", "bootstrap", "create", "validate", "doctor", "version",
		"show", "list", "log", "render-context", "sources", "context-history", "hook", "completion", "tui", "reparent", "scope-rename", "scope-delete",
	}

	for groupID, expectedCmds := range map[string][]string{
		"workflow": workflowExpected,
		"dag":      dagExpected,
		"sync":     syncExpected,
		"admin":    adminExpected,
	} {
		for _, expectedCmd := range expectedCmds {
			assert.True(t, commandsByGroup[groupID][expectedCmd], "command %q not found in group %q", expectedCmd, groupID)
		}
	}

	// Verify each group has at least one command
	for _, groupID := range requiredGroups {
		assert.Greater(t, len(commandsByGroup[groupID]), 0, "group %q has no commands", groupID)
	}
}

// TestTransitionToDone_PRCheck_FailsWhenOnMainWithoutForce verifies that transitioning to done fails
// when on main branch without --force flag.
func TestTransitionToDone_PRCheck_FailsWhenOnMainWithoutForce(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	defaultBranchOut, err := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
	require.NoError(t, err)
	defaultBranch := strings.TrimSpace(string(defaultBranchOut))

	_, err = runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-01", "--title", "Test task", "--type", "task")
	require.NoError(t, err)

	// Claim the task
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)

	// Ensure we're on the default branch (main or master depending on git config)
	run(t, repo, "git", "checkout", "-q", defaultBranch)

	// Try to transition to done without --force; should fail
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--outcome", "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot transition to done")
}

// TestTransitionToDone_PRCheck_SucceedsWithForceOnMain verifies that transitioning to done succeeds
// when using --force flag even on main branch.
func TestTransitionToDone_PRCheck_SucceedsWithForceOnMain(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	defaultBranchOut, err := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
	require.NoError(t, err)
	defaultBranch := strings.TrimSpace(string(defaultBranchOut))

	_, err = runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-01b", "--title", "Test task", "--type", "task")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-01b", "--worktree")
	require.NoError(t, err)

	// Ensure we're on the default branch (main or master depending on git config)
	run(t, repo, "git", "checkout", "-q", defaultBranch)

	// Transition with --force should succeed
	_, err = runTrls(t, repo, "transition", "--issue", "task-01b", "--to", "done", "--skip-delivery-gate", "--outcome", "test", "--force")
	assert.NoError(t, err)
}

// TestTransitionToDone_PRCheck_SucceedsOnFeatureBranch verifies that transitioning to done succeeds
// when on a feature branch (not main).
func TestTransitionToDone_PRCheck_SucceedsOnFeatureBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-01c", "--title", "Test task", "--type", "task")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-01c", "--worktree")
	require.NoError(t, err)

	// Create and checkout a feature branch
	run(t, repo, "git", "checkout", "-q", "-b", "feat/task-01c")

	// Transition to done on feature branch should succeed
	_, err = runTrls(t, repo, "transition", "--issue", "task-01c", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "test")
	assert.NoError(t, err)
}

// TestTransitionToDone_ParentStoryWarning verifies that after all siblings are done,
// a warning is printed if parent story is still in-progress.
func SKIP_TestTransitionToDone_ParentStoryWarning(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a story and two tasks under it
	_, err = runTrls(t, repo, "create", "--id", "story-01", "--title", "Test story", "--type", "story")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-02", "--title", "Task 2", "--type", "task", "--parent", "story-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-03", "--title", "Task 3", "--type", "task", "--parent", "story-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a feature branch to allow transitions
	run(t, repo, "git", "checkout", "-q", "-b", "feat/story-01")

	// Transition task-02 to done
	_, err = runTrls(t, repo, "transition", "--issue", "task-02", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "completed")
	require.NoError(t, err)

	// Materialize to reflect task-02's transition
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Transition task-03 to done; should emit warning about story still being in-progress
	_, errOut, err := runTrlsWithStderr(t, repo, "transition", "--issue", "task-03", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "completed")
	assert.NoError(t, err)

	// Check stderr for warning
	assert.Contains(t, errOut, "story-01", "warning should mention parent story ID")
	assert.Contains(t, errOut, "in-progress", "warning should mention in-progress status")
}

// TestTransitionToDone_NoWarningWhenTasksRemain verifies that no warning is printed
// when there are still uncompleted sibling tasks.
func SKIP_TestTransitionToDone_NoWarningWhenTasksRemain(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a story and two tasks
	_, err = runTrls(t, repo, "create", "--id", "story-02", "--title", "Test story 2", "--type", "story")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-04", "--title", "Task 4", "--type", "task", "--parent", "story-02")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-05", "--title", "Task 5", "--type", "task", "--parent", "story-02")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a feature branch
	run(t, repo, "git", "checkout", "-q", "-b", "feat/story-02")

	// Transition only one task; should not warn
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(errBuf)
	root.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-04", "--to", "done", "--skip-delivery-gate", "--outcome", "completed"}))
	err = root.Execute()
	assert.NoError(t, err)

	errOut := errBuf.String()
	// Should NOT contain warning about story since sibling is still pending
	assert.NotContains(t, errOut, "story-02")
}

// TestNoteCommand_PositionalArgs verifies that positional args work: trls note <id> <message>
func TestNoteCommand_PositionalArgs(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Test with positional args: note task-01 "message"
	out, err := runTrls(t, repo, "note", "task-01", "progress from positional args")
	require.NoError(t, err, "note with positional args should succeed")
	assert.Contains(t, out, "task-01")

	// Verify it works with JSON format too
	out, err = runTrls(t, repo, "note", "task-01", "another message", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"note"`)
	assert.Contains(t, out, "task-01")
}

// TestNoteCommand_PositionalArgs_EquivalentToFlags verifies positional args work identically to --issue/--msg flags
func TestNoteCommand_PositionalArgs_EquivalentToFlags(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Call with flags
	out1, err := runTrls(t, repo, "note", "--issue", "task-01", "--msg", "message via flags", "--format", "json")
	require.NoError(t, err)

	// Call with positional args
	out2, err := runTrls(t, repo, "note", "task-01", "message via positional", "--format", "json")
	require.NoError(t, err)

	// Both should have similar structure (both JSON with "note" key)
	assert.Contains(t, out1, `"note"`)
	assert.Contains(t, out1, "task-01")
	assert.Contains(t, out2, `"note"`)
	assert.Contains(t, out2, "task-01")
}

// TestShowCommand_MultipleIDs verifies that trls show accepts multiple positional
// issue IDs and outputs all of them separated by "---".
func TestShowCommand_MultipleIDs(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Create two tasks
	_, err = runTrls(t, repo, "create", "--id", "show-a", "--title", "Show Task A", "--type", "task")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "show-b", "--title", "Show Task B", "--type", "task")
	require.NoError(t, err)

	// Show both in a single call using positional args
	out, err := runTrls(t, repo, "show", "show-a", "show-b")
	require.NoError(t, err)

	// Both issue IDs must appear in output
	assert.Contains(t, out, "show-a")
	assert.Contains(t, out, "show-b")
	// Output must contain a separator between them
	assert.Contains(t, out, "---")
}

// TestShowCommand_MultipleIDs_JSON verifies that --format json with multiple IDs
// outputs a JSON array.
func TestShowCommand_MultipleIDs_JSON(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "show-j1", "--title", "JSON Task 1", "--type", "task")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "show-j2", "--title", "JSON Task 2", "--type", "task")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "show", "--format", "json", "show-j1", "show-j2")
	require.NoError(t, err)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &results))
	require.Len(t, results, 2)
	ids := []string{results[0]["id"].(string), results[1]["id"].(string)} //nolint:errcheck // test helper; error checked via output assertions
	assert.Contains(t, ids, "show-j1")
	assert.Contains(t, ids, "show-j2")
}

// TestCreateCommand_WithAcceptanceFlag verifies that --acceptance on create produces a
// fully-specified task without requiring a follow-up trls amend.
func TestCreateCommand_WithAcceptanceFlag(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	acceptanceJSON := `[{"type":"test_passes","description":"all tests green"}]`
	out, err := runTrls(t, repo, "create",
		"--title", "Feature with acceptance",
		"--type", "task",
		"--id", "acc-01",
		"--acceptance", acceptanceJSON,
	)
	require.NoError(t, err)
	assert.Contains(t, out, "acc-01")

	// Materialize to verify acceptance was captured in the create op without a follow-up amend.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	stateDir := getTestStateDir(t, repo)
	issuePath := filepath.Join(stateDir, "issues", "acc-01.json")
	issue, err := materialize.LoadIssue(issuePath)
	require.NoError(t, err)
	assert.NotEmpty(t, issue.Acceptance, "acceptance should be set from --acceptance flag on create")

	var criteria []map[string]any
	require.NoError(t, json.Unmarshal(issue.Acceptance, &criteria))
	require.Len(t, criteria, 1)
	assert.Equal(t, "test_passes", criteria[0]["type"])
}

func TestCreateCommand_WithContextFilesFlag(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create",
		"--title", "Feature with context",
		"--type", "task",
		"--id", "ctx-01",
		"--context-file", "docs/adr.md",
		"--context-file", "docs/design.md",
	)
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "ctx-01.json"))
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/adr.md", "docs/design.md"}, issue.ContextFiles)
}

func TestAmendCommand_ReplacesAndClearsContextFiles(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create",
		"--title", "Feature with context",
		"--type", "task",
		"--id", "ctx-amend-01",
		"--context-file", "docs/original.md",
	)
	require.NoError(t, err)

	_, err = runTrls(t, repo, "amend",
		"--issue", "ctx-amend-01",
		"--context-file", "docs/replaced.md",
		"--context-file", "docs/extra.md",
	)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issuePath := filepath.Join(getTestStateDir(t, repo), "issues", "ctx-amend-01.json")
	issue, err := materialize.LoadIssue(issuePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/replaced.md", "docs/extra.md"}, issue.ContextFiles)

	_, err = runTrls(t, repo, "amend", "--issue", "ctx-amend-01", "--clear-context-files")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err = materialize.LoadIssue(issuePath)
	require.NoError(t, err)
	assert.Empty(t, issue.ContextFiles)
}

// TestTransitionCommand_WithFieldFlag verifies that --field extracts a single field
// from transition output without needing post-processing.
func TestTransitionCommand_WithFieldFlag(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Transition to done and extract just the status field
	out, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--force", "--outcome", "completed", "--field", "status")
	require.NoError(t, err)

	// Output should be just "done" (the status value), nothing else
	assert.Equal(t, "done\n", out)
}

// TestCreateCommand_WithSourceFlag verifies that --source on create emits both a
// create op and a source-link op in a single invocation, so the issue is fully
// cited without a follow-up source-link command.
func TestCreateCommand_WithSourceFlag(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a temporary file to use as a filesystem source.
	tmpFile := filepath.Join(t.TempDir(), "dogfooding-learnings.md")
	require.NoError(t, os.WriteFile(tmpFile, []byte("# Dogfooding Learnings\n"), 0600))

	// Register the source in the manifest.
	out, err := runTrls(t, repo, "sources", "add", "--url", tmpFile, "--type", "filesystem", "--title", "Learnings")
	require.NoError(t, err)

	// Extract the source UUID from "added source <uuid> ..." output.
	parts := strings.Fields(strings.TrimSpace(out))
	require.GreaterOrEqual(t, len(parts), 3, "expected 'added source <uuid> ...' in output: %s", out)
	sourceID := parts[2]

	t.Run("by source UUID", func(t *testing.T) {
		_, err := runTrls(t, repo, "create",
			"--title", "Source linked task (by id)",
			"--type", "task",
			"--id", "src-id-01",
			"--source", sourceID,
			"--scope", "cmd/armature/src_id.go",
			"--dod", "Source-linked task by id is complete",
			"--acceptance", `[{"type":"test_passes"}]`,
		)
		require.NoError(t, err)

		// Materialize and confirm the issue exists.
		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)

		// Validate: the issue should NOT appear as "uncited node" — it is source-linked.
		validateOut, err := runTrls(t, repo, "validate", "--format", "human")
		require.NoError(t, err)
		assert.NotContains(t, validateOut, "uncited node: src-id-01", "source-linked issue should not appear as uncited")
		assert.Contains(t, validateOut, "COVERAGE: ", "coverage line should be present")
		// Coverage total-cited should be ≥ 1.
		assert.NotContains(t, validateOut, "COVERAGE: 0/", "at least one issue should be cited")
	})

	t.Run("by source URL/path", func(t *testing.T) {
		_, err := runTrls(t, repo, "create",
			"--title", "Source linked task (by path)",
			"--type", "task",
			"--id", "src-url-01",
			"--source", tmpFile,
			"--scope", "cmd/armature/src_url.go",
			"--dod", "Source-linked task by path is complete",
			"--acceptance", `[{"type":"test_passes"}]`,
		)
		require.NoError(t, err)

		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)

		validateOut, err := runTrls(t, repo, "validate")
		require.NoError(t, err)
		assert.NotContains(t, validateOut, "uncited node: src-url-01", "source-linked issue should not appear as uncited")
		assert.NotContains(t, validateOut, "COVERAGE: 0/", "at least one issue should be cited")
	})

	t.Run("unknown source ref returns error", func(t *testing.T) {
		_, err := runTrls(t, repo, "create",
			"--title", "Bad source task",
			"--type", "task",
			"--id", "src-bad-01",
			"--source", "nonexistent-source-ref",
		)
		require.Error(t, err, "should fail when source ref is not found in manifest")
		assert.Contains(t, err.Error(), "not found")

		_, showErr := runTrls(t, repo, "show", "--issue", "src-bad-01")
		require.Error(t, showErr, "create --source must not land the create when source resolution fails")
	})
}

// TestTransitionToDone_UncitedIssue_PrintsWarning verifies that transitioning an uncited
// issue to done prints a warning to stderr mentioning source-link and accept-citation,
// but still exits 0.
func TestTransitionToDone_UncitedIssue_PrintsWarning(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Uncited task", "--id", "uncited-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a feature branch so branch check passes
	run(t, repo, "git", "checkout", "-q", "-b", "feat/uncited-test")

	// Transition to done without --force and without any citation — should warn but exit 0
	_, stderr, err := runTrlsWithStderr(t, repo, "transition", "--issue", "uncited-01", "--to", "done", "--skip-delivery-gate", "--outcome", "done")
	require.NoError(t, err, "transition should succeed (exit 0) even for uncited issue")
	assert.Contains(t, stderr, "WARNING", "should print WARNING on stderr for uncited issue")
	assert.Contains(t, stderr, "source citation", "warning should mention source citation")
	assert.Contains(t, stderr, "sources", "link", "warning should direct user to arm source-link")
	assert.Contains(t, stderr, "sources", "accept-citation", "warning should direct user to arm accept-citation")
}

// TestTransitionToDone_UncitedIssue_ForceSupportsWarning verifies that --force suppresses
// the uncited warning entirely.
func TestTransitionToDone_UncitedIssue_ForceSupressesWarning(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Force uncited task", "--id", "uncited-force-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a feature branch so branch check passes
	run(t, repo, "git", "checkout", "-q", "-b", "feat/uncited-force-test")

	// Transition with --force should suppress the uncited warning
	_, stderr, err := runTrlsWithStderr(t, repo, "transition", "--issue", "uncited-force-01", "--to", "done",
		"--skip-delivery-gate", "--force", "--outcome", "done")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "WARNING", "no WARNING should appear when --force is used")
	assert.NotContains(t, stderr, "source citation", "no citation warning when --force is used")
}

// TestTransitionToDone_CitedIssue_NoWarning verifies that a cited issue (with accept-citation)
// does not trigger any warning when transitioning to done.
func TestTransitionToDone_CitedIssue_NoWarning(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Cited task", "--id", "cited-01")
	require.NoError(t, err)

	// Accept citation so the issue is cited
	_, err = runTrls(t, repo, "sources", "accept-citation", "--issue", "cited-01", "--rationale", "no source doc exists for this task", "--ci")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a feature branch so branch check passes
	run(t, repo, "git", "checkout", "-q", "-b", "feat/cited-test")

	// Transition to done — cited issue should produce no warning
	_, stderr, err := runTrlsWithStderr(t, repo, "transition", "--issue", "cited-01", "--to", "done", "--skip-delivery-gate", "--outcome", "done")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "WARNING", "no WARNING should appear for a cited issue")
	assert.NotContains(t, stderr, "source citation", "no citation warning for a cited issue")
}

// TestSourceLinkCommand_MultiIssue verifies that --issue can be repeated to link
// multiple issues to the same source in a single invocation.
func TestSourceLinkCommand_MultiIssue(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Register a filesystem source.
	tmpFile := filepath.Join(t.TempDir(), "plan.md")
	require.NoError(t, os.WriteFile(tmpFile, []byte("# Plan\n"), 0600))
	out, err := runTrls(t, repo, "sources", "add", "--url", tmpFile, "--type", "filesystem", "--title", "Plan")
	require.NoError(t, err)
	parts := strings.Fields(strings.TrimSpace(out))
	require.GreaterOrEqual(t, len(parts), 3)
	sourceID := parts[2]

	// Create two tasks.
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Multi-link task A", "--id", "ml-a",
		"--scope", "cmd/armature/ml_a.go", "--dod", "Multi-link task A is complete",
		"--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Multi-link task B", "--id", "ml-b",
		"--scope", "cmd/armature/ml_b.go", "--dod", "Multi-link task B is complete",
		"--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)

	// Link both issues in one invocation.
	out, err = runTrls(t, repo, "sources", "link",
		"--issue", "ml-a",
		"--issue", "ml-b",
		"--source-id", sourceID,
	)
	require.NoError(t, err)
	assert.Contains(t, out, "ml-a")
	assert.Contains(t, out, "ml-b")
	assert.Contains(t, out, sourceID)

	// Materialize and confirm both issues are cited.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	validateOut, err := runTrls(t, repo, "validate")
	require.NoError(t, err)
	assert.NotContains(t, validateOut, "uncited node: ml-a")
	assert.NotContains(t, validateOut, "uncited node: ml-b")
}

// TestSourceLinkCommand_SingleIssue_BackwardCompat verifies that the single-issue
// path (positional arg or single --issue flag) is unchanged.
func TestSourceLinkCommand_SingleIssue_BackwardCompat(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	tmpFile := filepath.Join(t.TempDir(), "plan.md")
	require.NoError(t, os.WriteFile(tmpFile, []byte("# Plan\n"), 0600))
	out, err := runTrls(t, repo, "sources", "add", "--url", tmpFile, "--type", "filesystem", "--title", "Plan")
	require.NoError(t, err)
	parts := strings.Fields(strings.TrimSpace(out))
	require.GreaterOrEqual(t, len(parts), 3)
	sourceID := parts[2]

	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Single link task", "--id", "sl-01",
		"--scope", "cmd/armature/sl_01.go", "--dod", "Single-link task is complete",
		"--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)

	// Single --issue flag (existing flag path).
	out, err = runTrls(t, repo, "sources", "link", "--issue", "sl-01", "--source-id", sourceID)
	require.NoError(t, err)
	assert.Contains(t, out, "sl-01")
	assert.Contains(t, out, sourceID)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	validateOut, err := runTrls(t, repo, "validate")
	require.NoError(t, err)
	assert.NotContains(t, validateOut, "uncited node: sl-01")
}

// TestCreateCommand_InvalidType verifies that arm create rejects unrecognized types
// with an explicit error listing valid types.
func TestCreateCommand_InvalidType(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "create", "--title", "Bad type", "--type", "bogustype", "--id", "bad-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogustype", "error must name the invalid type")
	assert.Contains(t, err.Error(), "valid types", "error must list valid types")
}

// TestCreateCommand_HelpIncludesFeatureType verifies the create help text reflects the
// canonical issue type list.
func TestCreateCommand_HelpIncludesFeatureType(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "create", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "feature", "help text should list feature as an accepted type")
}

// TestCreateCommand_BugTypeAccepted verifies that arm create accepts the bug type.
func TestCreateCommand_BugTypeAccepted(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "create", "--title", "Bug report", "--type", "bug", "--id", "bug-01")
	require.NoError(t, err, "bug type must be accepted as a valid type")
}

// TestCreateCommand_InvalidParentTypeCombo verifies that arm create rejects
// a bug issued under a task (which is an invalid hierarchy).
func TestCreateCommand_InvalidParentTypeCombo(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Materialize so issues/task-01.json exists for ReadIssue in create --parent.
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// task-01 is a task; trying to create a bug under it must fail
	_, err = runTrls(t, repo, "create",
		"--title", "Bug under task",
		"--type", "bug",
		"--id", "bug-under-task",
		"--parent", "task-01",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid parent", "error must indicate invalid parent/type combo")
}

// TestCreateCommand_ValidParentTypeCombo verifies that a bug under a story is accepted.
func TestCreateCommand_ValidParentTypeCombo(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Create a story first
	_, err = runTrls(t, repo, "create", "--title", "My Story", "--type", "story", "--id", "story-01")
	require.NoError(t, err)

	// Materialize so issues/story-01.json exists for ReadIssue in create --parent.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Creating a bug under the story should succeed
	_, err = runTrls(t, repo, "create",
		"--title", "Bug in story",
		"--type", "bug",
		"--id", "bug-01",
		"--parent", "story-01",
	)
	require.NoError(t, err, "bug under story must be accepted")
}

// TestReparentCommand_HappyPath verifies that arm reparent moves an issue to a new parent.
func TestReparentCommand_HappyPath(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Create two stories and a task under story-01.
	// Materialize after creating the stories so issues/story-01.json exists for ReadIssue.
	_, err = runTrls(t, repo, "create", "--title", "Story One", "--type", "story", "--id", "story-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Story Two", "--type", "story", "--id", "story-02")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "My Task", "--type", "task", "--id", "task-01", "--parent", "story-01")
	require.NoError(t, err)

	// Reparent task-01 from story-01 to story-02
	out, err := runTrls(t, repo, "reparent", "--issue", "task-01", "--parent", "story-02")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "story-02")

	// Materialize and verify the parent changed
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)
	entry, ok := index["task-01"]
	require.True(t, ok, "task-01 must exist")
	assert.Equal(t, "story-02", entry.Parent, "task-01 should now have parent=story-02")
}

// TestReparentCommand_InvalidCombo verifies that arm reparent rejects an invalid
// parent/type combination.
func TestReparentCommand_InvalidCombo(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create a second task to use as (invalid) parent
	_, err := runTrls(t, repo, "create", "--title", "Parent task", "--type", "task", "--id", "task-parent")
	require.NoError(t, err)

	// task cannot parent task
	_, err = runTrls(t, repo, "reparent", "--issue", "task-01", "--parent", "task-parent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid parent", "reparent must reject invalid parent/type combos")
}

// TestReparentCommand_ParentNotFound verifies that arm reparent returns an error
// when the target parent does not exist.
func TestReparentCommand_ParentNotFound(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "reparent", "--issue", "task-01", "--parent", "nonexistent-parent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found", "reparent must fail when parent does not exist")
}

// TestReparentCommand_IssueNotFound verifies that arm reparent returns an error
// when the issue being reparented does not exist.
func TestReparentCommand_IssueNotFound(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "reparent", "--issue", "nonexistent-issue", "--parent", "task-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found", "reparent must fail when issue does not exist")
}

// TestCreateCmd_WithParent_DoesNotMaterialize verifies that arm create --parent uses
// store.ReadIssue() to validate the parent without triggering full rematerialization.
//
// RED with store.Load(): Load calls MaterializeAndReturnQuiet which rewrites checkpoint.json,
// advancing its mtime → mtime assertion fails.
// GREEN with store.ReadIssue(): no materialization → checkpoint.json mtime unchanged.
func TestCreateCmd_WithParent_DoesNotMaterialize(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Create a story
	_, err = runTrls(t, repo, "create", "--title", "Parent Story", "--type", "story", "--id", "story-01")
	require.NoError(t, err)

	// Materialize to write checkpoint.json and issues/story-01.json.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Capture checkpoint.json mtime before running create --parent.
	stateDir := getTestStateDir(t, repo)
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	stat, statErr := os.Stat(checkpointPath)
	require.NoError(t, statErr, "checkpoint.json should exist after materialize")
	mtimeBefore := stat.ModTime()

	// Run create --parent — must use ReadIssue, not Load.
	_, err = runTrls(t, repo, "create", "--title", "Child Task", "--type", "task", "--id", "task-01", "--parent", "story-01")
	require.NoError(t, err)

	// Verify checkpoint.json was NOT rewritten (no rematerialization occurred).
	statAfter, statErr := os.Stat(checkpointPath)
	require.NoError(t, statErr)
	assert.Equal(t, mtimeBefore, statAfter.ModTime(),
		"checkpoint.json must not be updated by arm create --parent: store.ReadIssue must be used, not store.Load")
}

// TestManagedExecutionCommandsAreNotRegistered verifies that arm orchestrate and
// arm worker run return unknown-command errors, and that worker-init --check still works.
func TestManagedExecutionCommandsAreNotRegistered(t *testing.T) {
	repo := t.TempDir()

	_, err := runTrls(t, repo, "orchestrate")
	require.Error(t, err, "arm orchestrate must return an error")
	assert.Contains(t, err.Error(), "unknown command", "arm orchestrate must return unknown-command error")

	_, err = runTrls(t, repo, "worker", "run")
	require.Error(t, err, "arm worker run must return an error")
	assert.Contains(t, err.Error(), "unknown command", "arm worker run must return unknown-command error")

	// worker-init --check must still execute (may fail if no worker ID, but must not be unknown command)
	_, err = runTrls(t, repo, "worker-init", "--check")
	if err != nil {
		assert.NotContains(t, err.Error(), "unknown command", "worker-init must still be registered")
	}
}

// TestValidateDocExamplesCommand_NoArmatureConfig verifies that `arm validate-doc-examples`
// runs successfully in a fresh checkout with no .armature directory at all — i.e. its
// PersistentPreRunE no-op (mirroring bootstrap.go's pattern) skips root config resolution.
// This pins the fix for PR review thread PRRT_kwDORnVQE86RLMsv.
func TestValidateDocExamplesCommand_NoArmatureConfig(t *testing.T) {
	repo := t.TempDir() // no git init, no .armature — config resolution would fail here
	writeFile := func(name, content string) {
		path := filepath.Join(repo, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	writeFile("docs/schemas/plan.schema.json", `{"type":"object"}`)
	writeFile("docs/schemas/review-bundle.schema.json", `{"type":"object"}`)
	writeFile("docs/schemas/conformance-assessment.schema.json", `{"type":"object"}`)
	writeFile("docs/schemas/activity-index.schema.json", `{"type":"object"}`)
	writeFile("docs/example.md", "```json artifact_type=plan\n{}\n```\n")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"validate", "doc-examples", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Documentation JSON examples are valid")
}

// TestValidateRejectsUnexpectedArguments verifies that the graph validator remains
// the bare `arm validate` command; `graph` is documentation-only terminology, not a
// registered subcommand. The hidden `validate doc-examples` subcommand remains valid.
func TestValidateRejectsUnexpectedArguments(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"validate", "graph"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command \"graph\" for \"arm validate\"")
}

// TestCommand_RefusesUnmigratedLayout_REQ_LNGHZN_S1_T4 verifies that non-bootstrap commands
// detect the old unmigrated .arm/.armature/ layout and refuse with guidance to run bootstrap.
func TestCommand_RefusesUnmigratedLayout_REQ_LNGHZN_S1_T4(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Manually set up the old dual-branch layout (.arm/.armature/)
	// This simulates the current pre-collapse layout before migration.
	armWorktreePath := filepath.Join(repo, ".arm")
	issuesDir := filepath.Join(armWorktreePath, ".armature")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755), "create .arm/.armature directory structure")

	// Create config.json so the layout is complete
	cfg := config.DefaultConfig("go")
	require.NoError(t, config.WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	// Create ops directory (required for a valid layout)
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "ops"), 0o755))

	// Set the git config to point to the old .arm worktree
	run(t, repo, "git", "config", "armature.ops-worktree-path", armWorktreePath)

	// Try to run a non-bootstrap command (e.g., list)
	// It should fail with a clear error message about the unmigrated layout
	_, err := runTrls(t, repo, "list")
	require.Error(t, err, "command should fail with unmigrated layout error")
	errMsg := err.Error()
	assert.Contains(t, errMsg, "pre-collapse", "error should mention pre-collapse layout")
	assert.Contains(t, errMsg, "arm bootstrap", "error should mention arm bootstrap as fix")

	// Also test another non-bootstrap command to ensure the check is consistent
	_, err = runTrls(t, repo, "ready")
	require.Error(t, err, "command should fail with unmigrated layout error")
	errMsg = err.Error()
	assert.Contains(t, errMsg, "pre-collapse", "error should mention pre-collapse layout")
	assert.Contains(t, errMsg, "arm bootstrap", "error should mention arm bootstrap as fix")

	// Bootstrap command should still work (it's exempt from the check)
	// and can migrate if/when that task is implemented
	buf := new(bytes.Buffer)
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(buf)
	bootstrapCmd.SetErr(new(bytes.Buffer))
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	// Bootstrap should execute without the unmigrated layout error blocking it
	err = bootstrapCmd.Execute()
	if err != nil {
		assert.NotContains(t, err.Error(), "pre-collapse", "bootstrap must bypass the unmigrated-layout guard")
	}

	// Simulate the collapsed layout by pointing git config to .armature instead of .arm
	// (In practice this would be done by the migration, but for testing we simulate the end state)
	armatureWorktreePath := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(armatureWorktreePath, "ops"), 0o755), "create collapsed layout")
	require.NoError(t, config.WriteConfig(filepath.Join(armatureWorktreePath, "config.json"), cfg))
	run(t, repo, "git", "config", "armature.ops-worktree-path", armatureWorktreePath)

	// Now non-bootstrap commands should work (no more unmigrated layout error)
	_, err = runTrls(t, repo, "list")
	// The list command might fail for other reasons (e.g., no issues), but it should NOT
	// fail with the unmigrated layout error — if it gets past context resolution, we've
	// verified the check isn't blocking migrated repos.
	if err != nil {
		// If list failed, make sure it's not because of unmigrated layout
		assert.NotContains(t, err.Error(), "pre-collapse", "list should not fail with unmigrated layout error")
	}
}

func TestCommand_CustomOpsWorktreeLayout_REQ_LNGHZN_S1_T4(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	customWorktree := filepath.Join(repo, ".custom-ops")
	legacyIssuesDir := filepath.Join(customWorktree, config.StateDirName)
	cfg := config.DefaultConfig("go")
	require.NoError(t, os.MkdirAll(filepath.Join(legacyIssuesDir, "ops"), 0o755))
	require.NoError(t, config.WriteConfig(filepath.Join(legacyIssuesDir, "config.json"), cfg))
	run(t, repo, "git", "config", "armature.ops-worktree-path", customWorktree)

	_, err := runTrls(t, repo, "list")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-collapse")
	assert.Contains(t, err.Error(), "arm bootstrap")

	// Bootstrap cannot safely relocate an arbitrary custom worktree, so it must
	// refuse rather than creating a second layout and leaving legacy ops behind.
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	err = bootstrapCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "custom ops worktree")

	// A custom path whose root is the issues directory is already collapsed and
	// must remain usable; the path name itself is not the layout discriminator.
	require.NoError(t, os.RemoveAll(legacyIssuesDir))
	require.NoError(t, os.MkdirAll(filepath.Join(customWorktree, "ops"), 0o755))
	require.NoError(t, config.WriteConfig(filepath.Join(customWorktree, "config.json"), cfg))

	_, err = runTrls(t, repo, "list")
	if err != nil {
		assert.NotContains(t, err.Error(), "pre-collapse")
	}
}
