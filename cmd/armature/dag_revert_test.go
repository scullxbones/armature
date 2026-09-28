package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/clock"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDagRevertPublishesCancelOps_REQ_DECOMPOSE(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	planFile := writePlanIssue(t, "REV-PUB")
	_, err = runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.NoError(t, err)

	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	out, err := runTrls(t, repo, "dag", "revert", "--plan", planFile)
	require.NoError(t, err)
	assert.Contains(t, out, "Reverted")

	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.NotEqual(t, headBefore, headAfter, "revert must commit cancel ops in the ops worktree")
	require.True(t, originArmatureContains(t, bareDir, `"to":"cancelled"`),
		"revert must publish cancel ops to origin/_armature")
	require.True(t, originArmatureContains(t, bareDir, "REV-PUB"))
}

func TestDagRevertPublishFailureKeepsLocalCommit_REQ_DECOMPOSE(t *testing.T) {
	_, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	planFile := writePlanIssue(t, "REV-PUB-FAIL")
	_, err = runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.NoError(t, err)
	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	breakOrigin(t, repo)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"dag", "revert", "--repo", repo, "--plan", planFile, "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "DAG-1", cf.Code)
	assert.Contains(t, cf.Cause, "publish _armature")
	assert.Contains(t, cf.Cause, "class=", "publish failure must use the high-stakes error class, got %v", cf.Cause)

	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.NotEqual(t, headBefore, headAfter, "local revert commit must remain after publish failure")
	require.True(t, localOpsContainCancel(t, repo, "REV-PUB-FAIL"))
}

func TestDagRevertSucceedsWhenIntroductionWouldReject_REQ_DECOMPOSE(t *testing.T) {
	_, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	planFile := writePlanIssue(t, "REV-DIRTY")
	_, err := runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.NoError(t, err)

	state := lowStakesState(t, repo, worktree, 5)
	workerID := strings.TrimSpace(runOutput(t, repo, "config", "--get", "armature.worker-id"))
	logPath := filepath.Join(worktree, "ops", workerID+".log")
	overlapping := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "REV-OVERLAP",
		Timestamp: clock.System() + 1,
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:            "Overlapping dirty sibling",
			NodeType:         "task",
			Scope:            []string{"internal/REV-DIRTY.go"},
			DefinitionOfDone: "Overlapping dirty sibling is complete and tested",
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			Confidence:       "draft",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, overlapping))

	dirtyCreate := overlapping
	dirtyCreate.TargetID = "REV-DIRTY-CREATE"
	require.Error(t, refuseIntroduction(state.ctx, []ops.Op{dirtyCreate}),
		"graph must be dirty enough that refuseIntroduction rejects a validity-affecting write")

	normalCancel := ops.Op{
		Type:      ops.OpTransition,
		TargetID:  "REV-DIRTY",
		Timestamp: clock.System() + 2,
		WorkerID:  workerID,
		Payload:   ops.Payload{To: ops.StatusCancelled, Outcome: "done"},
	}
	require.Error(t, refuseIntroduction(state.ctx, []ops.Op{normalCancel}),
		"a normal cancel through refuseIntroduction must be refused on this graph (W11 outcome)")

	out, err := runTrls(t, repo, "dag", "revert", "--plan", planFile)
	require.NoError(t, err, "revert must skip refuseIntroduction so the recovery cancel can land")
	assert.Contains(t, out, "Reverted")
	require.True(t, localOpsContainCancel(t, repo, "REV-DIRTY"))
}

func TestDagRevertForeignChildRefusesApplyAndDryRun_REQ_DECOMPOSE(t *testing.T) {
	repo := plantDagApplyLogSlotRepo(t)
	planFile := writePlanIssue(t, "REV-PARENT")
	_, err := runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.NoError(t, err)

	workerID := strings.TrimSpace(runOutput(t, repo, "config", "--get", "armature.worker-id"))
	logPath := filepath.Join(repo, ".armature", "ops", workerID+".log")
	statBefore, err := os.Stat(logPath)
	require.NoError(t, err)

	foreign := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "FOREIGN-CHILD",
		Timestamp: clock.System() + 1,
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:            "Foreign child",
			NodeType:         "task",
			Parent:           "REV-PARENT",
			Scope:            []string{"internal/FOREIGN-CHILD.go"},
			DefinitionOfDone: "Foreign child is complete and tested",
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			Confidence:       "draft",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, foreign))
	statPlanted, err := os.Stat(logPath)
	require.NoError(t, err)
	require.Greater(t, statPlanted.Size(), statBefore.Size())

	_, dryErr := runTrls(t, repo, "dag", "revert", "--plan", planFile, "--dry-run")
	require.Error(t, dryErr)
	assert.Contains(t, dryErr.Error(), "FOREIGN-CHILD")
	assert.Contains(t, dryErr.Error(), "REV-PARENT")

	statAfterDry, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, statPlanted.Size(), statAfterDry.Size(), "dry-run refusal must write zero ops")

	_, applyErr := runTrls(t, repo, "dag", "revert", "--plan", planFile)
	require.Error(t, applyErr)
	assert.Contains(t, applyErr.Error(), "FOREIGN-CHILD")
	assert.Contains(t, applyErr.Error(), "REV-PARENT")

	statAfterApply, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, statPlanted.Size(), statAfterApply.Size(), "apply refusal must write zero ops")
	assert.Empty(t, logFilesContaining(t, repo, `"to":"cancelled"`))
}

func TestDagRevertHelpPromisesForeignChildGuard(t *testing.T) {
	cmd := newDecomposeRevertCmd()
	assert.Contains(t, cmd.Long, "no new children")
	assert.Contains(t, cmd.Long, "parent")
}

func TestAppendHighStakesOpsExemptIntroductionPublishes_REQ_DECOMPOSE(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	state := lowStakesState(t, repo, worktree, 5)
	logPath := filepath.Join(worktree, "ops", "revert-exempt-ok.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	op := ops.Op{
		Type: ops.OpTransition, TargetID: "T-REV-EX", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{To: ops.StatusCancelled},
	}
	require.NoError(t, appendHighStakesOpsExemptIntroduction(state, logPath, []ops.Op{op}))

	tracker, ok := state.tracker.(*fakePendingPushTracker)
	require.True(t, ok)
	assert.Equal(t, 1, tracker.resetCalls)
	require.True(t, originArmatureContains(t, bareDir, "T-REV-EX"))
}

func TestDagRevertDoesNotCallRefuseIntroduction_REQ_DECOMPOSE(t *testing.T) {
	decomposeSrc, err := os.ReadFile("decompose.go")
	require.NoError(t, err)
	src := string(decomposeSrc)
	assert.Contains(t, src, "appendHighStakesOpsExemptIntroduction",
		"dag revert must write through the named Introduction exemption")
	assert.NotContains(t, src, "refuseIntroduction(",
		"dag revert must not be routed through refuseIntroduction")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "helpers.go", nil, 0)
	require.NoError(t, err)

	var fn *ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		decl, ok := n.(*ast.FuncDecl)
		if !ok || decl.Name == nil || decl.Name.Name != "appendHighStakesOpsExemptIntroduction" {
			return true
		}
		fn = decl
		return false
	})
	require.NotNil(t, fn, "named exemption appendHighStakesOpsExemptIntroduction must exist")

	calledRefuse := false
	calledLock := false
	calledPublish := false
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			switch fun.Name {
			case "refuseIntroduction":
				calledRefuse = true
			case "withWorkerLogLock":
				calledLock = true
			case "pushOpsBranchAfter":
				calledPublish = true
			}
		}
		return true
	})
	assert.False(t, calledRefuse, "exemption must not call refuseIntroduction")
	assert.True(t, calledLock, "exemption must take the per-worker log lock")
	assert.True(t, calledPublish, "exemption must publish like other high-stakes writes")

	helpersSrc, err := os.ReadFile("helpers.go")
	require.NoError(t, err)
	assert.Contains(t, string(helpersSrc), "introductionExemptionDagRevert",
		"the skip must remain a named exemption, not an accidental omitted call")
}

func localOpsContainCancel(t *testing.T, repo, issueID string) bool {
	t.Helper()
	workerID := strings.TrimSpace(runOutput(t, repo, "config", "--get", "armature.worker-id"))
	require.NotEmpty(t, workerID)
	logged, err := ops.ReadLog(filepath.Join(repo, ".armature", "ops", workerID+".log"))
	require.NoError(t, err)
	for _, op := range logged {
		if op.Type == ops.OpTransition && op.TargetID == issueID && op.Payload.To == ops.StatusCancelled {
			return true
		}
	}
	return false
}
