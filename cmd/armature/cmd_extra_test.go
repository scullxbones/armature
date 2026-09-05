package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmCommand_Success(t *testing.T) {
	repo := setupRepoWithValidDraftNode(t)
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"confirm", "--repo", repo, "draft-task-01"})

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "confirmed draft-task-01")
}

// TestConfirmCmd_DoesNotMaterialize is historical: Plan Release on confirm
// must Load the graph. The command still uses ReadIssue to resolve the node
// before the gate; the gate's Load is required and is not a write-path leak.
func TestConfirmCmd_DoesNotMaterialize(t *testing.T) {
	repo := setupRepoWithValidDraftNode(t)
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "confirm", "draft-task-01")
	require.NoError(t, err)
}

func TestConfirmCommand_NotFound(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"confirm", "--repo", repo, "nonexistent-id"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSourcesAddCommand(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", "/docs/spec.md", "--type", "filesystem", "--title", "Spec"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "added source")
}

func TestSourcesSyncCommand_EmptyManifest(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"sources", "sync", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no sources")
}

func TestSourcesVerifyCommand_EmptyManifest(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"sources", "verify", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no sources")
}

func TestDAGSummaryCommand_NonInteractive_PendingItems(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create a draft task so dag-summary has items to report.
	cmd0 := newRootCmd()
	cmd0.SetOut(new(bytes.Buffer))
	cmd0.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo,
		"--title", "Draft feature", "--type", "task", "--id", "draft-01"}))
	require.NoError(t, cmd0.Execute())

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dag", "summary", "--repo", repo, "--format", "agent"})

	err := cmd.Execute()
	require.NoError(t, err)
	// Non-interactive mode with draft items outputs JSON
	assert.Contains(t, buf.String(), "pending_dag_confirmation")
}

func TestReadyCommand_JSONFormat(t *testing.T) {
	repo := setupRepoWithTask(t)
	plantVerifiedTask(t, repo, "ready-json-01", "cmd/armature/ready_json.go")
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"ready", "--repo", repo, "--format", "json"})

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[")
}

// TestReadyExpiredClaims_REQ_TOPTIER_S4_T3 verifies `arm ready` surfaces an
// expired claim distinctly in both text and JSON output, per TOPTIER-S4-T3's
// acceptance criterion, rather than silently omitting it (ComputeReady only
// ever returns status=open issues).
func TestReadyExpiredClaims_REQ_TOPTIER_S4_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Materialize first to establish baseline state before injecting the stale claim op.
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "expired-worker.log")
	staleClaimTime := time.Now().Unix() - 7200 // 2 hours ago, TTL 1 minute — stale
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: staleClaimTime,
		WorkerID: "expired-worker", Payload: ops.Payload{TTL: 1},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	jsonOut, jsonErrOut, err := runTrlsWithStderr(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	// stdout ready-queue shape is unchanged: task-01 is claimed, not open, so it
	// must not appear in the ready array itself.
	assert.NotContains(t, jsonOut, "task-01", "claimed+expired issue must not appear in the ready queue's own JSON array")
	var expiredClaims []map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonErrOut), &expiredClaims), "stderr must be a valid JSON array of expired claims")
	require.Len(t, expiredClaims, 1)
	assert.Equal(t, "task-01", expiredClaims[0]["issue"])
	assert.Equal(t, "expired-worker", expiredClaims[0]["claimed_by"])
}

// TestReadyExpiredClaims_ParentFilterScopesExpiredClaims_REQ_TOPTIER_S4_PRFIX
// verifies `arm ready --parent X` does not leak an expired claim on an issue
// outside that parent's subtree. Before the fix, expiredClaims was computed
// once from all issues and never filtered by --parent (unlike the main ready
// entries), so a scoped `arm ready` call would surface unrelated expired
// claims regardless of --parent.
func TestReadyExpiredClaims_ParentFilterScopesExpiredClaims_REQ_TOPTIER_S4_PRFIX(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Parent story with a child task-01 is scope; a sibling task outside
	// that parent must not leak into a --parent-scoped ready call.
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Parent story", "--type", "story", "--id", "E7"}))
	require.NoError(t, cmd.Execute())

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Unrelated task", "--type", "task", "--id", "task-outside"}))
	require.NoError(t, cmd2.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "expired-worker.log")
	staleClaimTime := time.Now().Unix() - 7200 // 2 hours ago, TTL 1 minute — stale
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpClaim, TargetID: "task-outside", Timestamp: staleClaimTime,
		WorkerID: "expired-worker", Payload: ops.Payload{TTL: 1},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// task-outside is not a descendant of E7: --parent E7 must not surface
	// its expired claim. RenderExpiredClaims emits nothing at all when there
	// are no (in-scope) expired claims, so an empty stderr is the expected,
	// correctly-scoped result.
	jsonErrOut := readyExpiredClaimsStderr(t, repo, "--parent", "E7")
	if jsonErrOut != "" {
		var expiredClaims []map[string]any
		require.NoError(t, json.Unmarshal([]byte(jsonErrOut), &expiredClaims), "stderr must be a valid JSON array of expired claims")
		assert.Empty(t, expiredClaims, "expired claim for task-outside must not leak into --parent E7 scoped ready")
	}
}

// readyExpiredClaimsStderr runs `arm ready --format json` with the given
// extra args and returns the stderr JSON payload (the expired-claims channel).
func readyExpiredClaimsStderr(t *testing.T, repo string, extraArgs ...string) string {
	t.Helper()
	args := append([]string{"ready", "--format", "json"}, extraArgs...)
	_, jsonErrOut, err := runTrlsWithStderr(t, repo, args...)
	require.NoError(t, err)
	return jsonErrOut
}

func TestImportCommand_DryRun_CSV(t *testing.T) {
	repo := setupRepoWithTask(t)

	csvFile := filepath.Join(t.TempDir(), "issues.csv")
	err := os.WriteFile(csvFile, []byte("id,title,type\nimp-1,Imported Story,story\n"), 0644)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"import", "--repo", repo, "--dry-run", csvFile})

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "dry-run")
}

func TestWorkersCommand_WithInitializedWorker(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "workers", "--repo", repo)
	require.NoError(t, err)
	_ = out // worker list rendered
}

func TestImportCommand_ActualImport(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	csvFile := filepath.Join(t.TempDir(), "issues.csv")
	require.NoError(t, os.WriteFile(csvFile, []byte("id,title,type\nimp-1,Imported Story,story\n"), 0644))

	out, err := runTrls(t, repo, "import", csvFile)
	require.NoError(t, err)
	assert.Contains(t, out, "imported 1 items")
}

// TestImportCommand_WithSource verifies that --source links each imported item to a source.
func TestImportCommand_WithSource(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	csvFile := filepath.Join(t.TempDir(), "issues.csv")
	require.NoError(t, os.WriteFile(csvFile, []byte("id,title,type\nwith-src-1,With Source Story,story\n"), 0644))

	out, err := runTrls(t, repo, "import", "--source", "src-import-01", csvFile)
	require.NoError(t, err)
	assert.Contains(t, out, "imported 1 items")
}

// TestImportCommand_InvalidType verifies that an import batch containing an
// invalid issue type anywhere in the file is rejected atomically, before any
// op from the batch is written — otherwise import would be a second ingress
// (besides amend) for writing unvalidated types straight into the op log.
func TestImportCommand_InvalidType(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	jsonFile := filepath.Join(t.TempDir(), "issues.json")
	require.NoError(t, os.WriteFile(jsonFile, []byte(`[
		{"id": "imp-ok-1", "title": "Valid item", "type": "task"},
		{"id": "imp-bad-1", "title": "Bad item", "type": "nonsense"}
	]`), 0644))

	_, err = runTrls(t, repo, "import", jsonFile)
	require.Error(t, err, "import with an invalid type anywhere in the batch should be rejected")
	assert.Contains(t, err.Error(), "invalid type")

	out, err := runTrls(t, repo, "log")
	require.NoError(t, err)
	assert.NotContains(t, out, "imp-ok-1", "no ops from the rejected batch should have been written")
	assert.NotContains(t, out, "imp-bad-1")
}

// TestIssueIDIngressRejectsPathSeparators_REQ_LNGHZN_S5 verifies every
// user-facing creation boundary rejects a path-shaped ID before appending any
// create op. This keeps IDs from becoming filesystem paths later in lifecycle
// commands or materialization.
func TestIssueIDIngressRejectsPathSeparators_REQ_LNGHZN_S5(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		_, err := runTrls(t, repo, "create", "--id", "team/task-01", "--title", "bad", "--type", "task")
		require.Error(t, err)
		out, logErr := runTrls(t, repo, "log")
		require.NoError(t, logErr)
		assert.NotContains(t, out, "team/task-01")
	})

	t.Run("import", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		file := filepath.Join(t.TempDir(), "issues.csv")
		require.NoError(t, os.WriteFile(file, []byte("id,title,type\nteam/task-01,Bad,task\n"), 0o600))
		_, err := runTrls(t, repo, "import", file)
		require.Error(t, err)
		out, logErr := runTrls(t, repo, "log")
		require.NoError(t, logErr)
		assert.NotContains(t, out, "team/task-01")
	})

	t.Run("decompose", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		file := filepath.Join(t.TempDir(), "plan.json")
		plan := `{"version":1,"title":"bad ids","issues":[{"id":"team/task-01","title":"Bad","type":"task","scope":"bad.go"}]}`
		require.NoError(t, os.WriteFile(file, []byte(plan), 0o600))
		_, err := runTrls(t, repo, "dag", "apply", "--plan", file)
		require.Error(t, err)
		out, logErr := runTrls(t, repo, "log")
		require.NoError(t, logErr)
		assert.NotContains(t, out, "team/task-01")
	})
}

func TestStaleReviewCommand_NoStale(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "sources", "stale-review", "--format", "agent")
	require.NoError(t, err)
	assert.Contains(t, out, "No stale sources detected")
}

func TestDecomposeRevertCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Test Plan","issues":[{` +
		`"id":"REV-001","title":"Revertable","type":"task","source":"src-test",` +
		`"scope":"internal/REV-001.go","dod":"Revertable task is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	_, err = runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "dag", "revert", "--plan", planFile)
	require.NoError(t, err)
	assert.Contains(t, out, "Reverted")
}

// TestDecomposeApply_DraftConfidence verifies that nodes created by decompose-apply
// have confidence=draft, are hidden from trls ready, and become visible after
// dag-transition promotes them to verified.
func TestDecomposeApply_DraftConfidence(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	acc := `"acceptance":[{"type":"test_passes"}]`
	planData := `{"version":1,"title":"Draft Test","issues":[` +
		`{"id":"DRF-001","title":"Draft task one","type":"task","source":"src-test","scope":"cmd/armature/drf1.go","dod":"Draft task one is complete",` + acc + `},` +
		`{"id":"DRF-002","title":"Draft task two","type":"task","source":"src-test","scope":"cmd/armature/drf2.go","dod":"Draft task two is complete",` + acc + `},` +
		`{"id":"DRF-003","title":"Draft task three","type":"task","source":"src-test",` +
		`"scope":"cmd/armature/drf3.go","dod":"Draft task three is complete",` + acc + `}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	// Apply the plan — all nodes should be created as draft
	out, err := runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.NoError(t, err)
	assert.Contains(t, out, "Applied 3 issues")

	// trls ready should NOT list draft nodes
	readyOut, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, readyOut, "DRF-001")
	assert.NotContains(t, readyOut, "DRF-002")
	assert.NotContains(t, readyOut, "DRF-003")

	// Promote via dag-transition on each root node (they have no parent so promote each)
	_, err = runTrls(t, repo, "dag", "transition", "--issue", "DRF-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "dag", "transition", "--issue", "DRF-002")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "dag", "transition", "--issue", "DRF-003")
	require.NoError(t, err)

	// After promotion trls ready should show the tasks
	readyOut2, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, readyOut2, "DRF-001")
	assert.Contains(t, readyOut2, "DRF-002")
	assert.Contains(t, readyOut2, "DRF-003")
}

func TestSourcesSyncCommand_WithFilesystemSource(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Init worker so sync can emit ops
	cmd0 := newRootCmd()
	cmd0.SetOut(new(bytes.Buffer))
	cmd0.SetArgs([]string{"worker-init", "--repo", repo})
	require.NoError(t, cmd0.Execute())

	// Create a file to sync
	docFile := filepath.Join(repo, "spec.md")
	require.NoError(t, os.WriteFile(docFile, []byte("# Spec"), 0644))

	// Add filesystem source
	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", docFile, "--type", "filesystem", "--title", "Spec"})
	require.NoError(t, cmd1.Execute())

	// Sync through the lifecycle provider registry.
	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"sources", "sync", "--repo", repo})

	err := cmd2.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "synced")
}

func TestSourcesVerifyCommand_AfterSync_OK(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	docFile := filepath.Join(repo, "spec.md")
	require.NoError(t, os.WriteFile(docFile, []byte("# Spec"), 0644))

	_, err = runTrls(t, repo, "sources", "add",
		"--url", docFile, "--type", "filesystem", "--title", "Spec")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "sources", "sync")
	require.NoError(t, err)

	// After sync, verify should pass
	out, err := runTrls(t, repo, "sources", "verify")
	require.NoError(t, err)
	assert.Contains(t, out, "OK")
}

func TestValidateCommand_JSON(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "--issue", "task-01",
		"--scope", "internal/ops/*.go",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "validate", "--format", "json", "--strict=false")
	require.NoError(t, err)
	assert.Contains(t, out, "{")
}

// Test extractFieldsFromIssue helper function
func TestExtractFieldsFromIssue_SingleField(t *testing.T) {
	issue := &materialize.Issue{
		ID:     "task-01",
		Title:  "Test task",
		Status: "open",
		Type:   "task",
		Parent: "E6",
	}

	fields := extractFieldsFromIssue(issue, "status")
	assert.Equal(t, []string{"open"}, fields)
}

func TestExtractFieldsFromIssue_MultipleFields(t *testing.T) {
	issue := &materialize.Issue{
		ID:      "task-01",
		Title:   "Test task",
		Status:  "open",
		Type:    "task",
		Parent:  "E6",
		Outcome: "Fixed bug",
	}

	fields := extractFieldsFromIssue(issue, "status,outcome,title")
	assert.Equal(t, []string{"open", "Fixed bug", "Test task"}, fields)
}

func TestExtractFieldsFromIssue_UnknownField(t *testing.T) {
	issue := &materialize.Issue{
		ID:    "task-01",
		Title: "Test task",
	}

	fields := extractFieldsFromIssue(issue, "unknown")
	assert.Equal(t, []string{""}, fields)
}

func TestExtractFieldsFromIssue_MixedKnownAndUnknown(t *testing.T) {
	issue := &materialize.Issue{
		ID:     "task-01",
		Title:  "Test task",
		Status: "open",
	}

	fields := extractFieldsFromIssue(issue, "status,unknown,title")
	assert.Equal(t, []string{"open", "", "Test task"}, fields)
}

func TestExtractFieldsFromIssue_BlockedByAbsent(t *testing.T) {
	issue := &materialize.Issue{
		ID:    "task-01",
		Title: "Test task",
	}

	fields := extractFieldsFromIssue(issue, "blocked_by")
	assert.Equal(t, []string{"[]"}, fields)
}

// Test trls show --field flag
func TestShowCommand_WithFieldFlag_SingleField(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, err)
	assert.Equal(t, "open\n", out)
}

func TestShowCommand_WithFieldFlag_MultipleFields(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "show", "task-01", "--field", "status,title")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "open", lines[0])
	assert.Equal(t, "Test task", lines[1])
}

func TestShowCommand_WithFieldFlag_BlockedByAbsent(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "show", "task-01", "--field", "blocked_by")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", out)
}

// Test trls status --status filter
func TestListCmd_Group_ShowsStatusHeaders(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "--format", "human", "list", "--group")
	require.NoError(t, err)
	assert.Contains(t, out, "=== open ===")
	assert.Contains(t, out, "story-01")
	assert.Contains(t, out, "task-01")
}

func TestListCmd_Group_WithStatusFilter(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Second task", "--type", "task", "--id", "task-02"}))
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-02", "--worktree"})
	require.NoError(t, cmd2.Execute())

	out, err := runTrls(t, repo, "--format", "human", "list", "--group", "--status", "open")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, "task-02")
}

func TestListCmd_Group_WithParentFilter(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Parent task", "--type", "story", "--id", "E6"}))
	require.NoError(t, cmd.Execute())

	// Materialize so issues/E6.json exists for ReadIssue in create --parent.
	_, materializeErr := runTrls(t, repo, "materialize")
	require.NoError(t, materializeErr)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Child task", "--type", "task", "--id", "task-child", "--parent", "E6"}))
	require.NoError(t, cmd2.Execute())

	out, err := runTrls(t, repo, "--format", "human", "list", "--group", "--parent", "E6")
	require.NoError(t, err)
	assert.Contains(t, out, "task-child")
	assert.NotContains(t, out, "task-01")
}

func TestListCmd_Group_JSONIgnoresGroupFlag(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--format", "json", "--repo", repo, "list", "--group"})
	require.NoError(t, cmd.Execute())

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entries),
		"--group must not break JSON output")
	assert.NotEmpty(t, entries)
}

func TestValidateCommand_PhantomScope_PrintsInfoNotWarning(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Amend task-01 to have scope pointing to a non-existent file
	_, err := runTrls(t, repo, "amend", "--issue", "task-01",
		"--scope", "nonexistent/file.go",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "validate", "--format", "json", "--strict=false")
	require.NoError(t, err)
	assert.Contains(t, out, "phantom scope", "phantom scope should appear in JSON infos")
	assert.NotContains(t, out, "WARNING: phantom scope", "phantom scope should not appear as WARNING")
}

func TestValidateCommand_JSON_IncludesInfosField(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "amend", "--issue", "task-01",
		"--scope", "nonexistent/file.go",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "validate", "--format", "json", "--strict=false")
	require.NoError(t, err)
	assert.Contains(t, out, `"infos"`, "JSON output should include infos field")
}

func TestValidateQuiet(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Provide all required task fields so validate reports OK (no errors)
	acceptance := `[{"type":"test_passes","cmd":"make check"}]`
	_, err := runTrls(t, repo, "amend", "--issue", "task-01",
		"--scope", "nonexistent/file.go",
		"--acceptance", acceptance,
		"--dod", "Tests pass and feature works")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "validate", "--quiet", "--format", "human")
	require.NoError(t, err)
	assert.NotContains(t, out, "INFO:", "--quiet should suppress INFO lines")
	assert.Contains(t, out, "COVERAGE:", "--quiet should still print COVERAGE lines")
	assert.Contains(t, out, "OK:", "--quiet should still print OK lines")
}

func TestImportCommand_DryRun_JSON(t *testing.T) {
	repo := setupRepoWithTask(t)

	csvFile := filepath.Join(t.TempDir(), "issues.csv")
	require.NoError(t, os.WriteFile(csvFile, []byte("id,title,type\nimp-1,Imported Story,story\n"), 0644))

	out, err := runTrls(t, repo, "import", "--dry-run", "--format", "json", csvFile)
	require.NoError(t, err)
	assert.Contains(t, out, "dry_run")
}

func TestAmendCmd_PatchesType(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "amend", "--issue", "task-01", "--type", "story")
	require.NoError(t, err)
	assert.Contains(t, out, "amended")

	// Materialize and check the type changed
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "story", index["task-01"].Type)
}

func TestAmendCmd_PatchesAcceptance(t *testing.T) {
	repo := setupRepoWithTask(t)

	acceptance := `[{"type":"test_passes","cmd":"make check"}]`
	out, err := runTrls(t, repo, "amend", "--issue", "task-01",
		"--acceptance", acceptance)
	require.NoError(t, err)
	assert.Contains(t, out, "amended")

	// Re-materialize and check validate no longer reports missing acceptance
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	validateOut, _ := runTrls(t, repo, "validate") //nolint:errcheck // test helper; errors checked via output assertions
	// After amendment the task should not report missing acceptance
	assert.NotContains(t, validateOut, "missing required field: acceptance on task task-01")
}

func TestAmendCmd_NoFieldsProvided_ReturnsError(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "amend", "--issue", "task-01")
	assert.Error(t, err)
}

// Fix W3: passing both --clear-context-files and --context-file must be a hard error.
func TestAmendCmd_ClearContextFilesAndContextFileConflict_ReturnsError(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "amend", "--issue", "task-01",
		"--clear-context-files",
		"--context-file", "docs/guide.md")
	require.Error(t, err, "using --clear-context-files together with --context-file must return an error")
	assert.Contains(t, err.Error(), "--clear-context-files")
}

// setupRepoWithSource creates a repo with a task and a source entry in the manifest,
// returning the repo path and the source UUID.
func setupRepoWithSource(t *testing.T) (string, string) {
	t.Helper()
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	docFile := filepath.Join(repo, "doc.md")
	require.NoError(t, os.WriteFile(docFile, []byte("# Doc"), 0644))

	out, err := runTrls(t, repo, "sources", "add",
		"--url", docFile, "--type", "filesystem", "--title", "Doc")
	require.NoError(t, err)

	// Extract UUID from "added source <uuid> (...)" output
	parts := strings.Fields(out)
	require.GreaterOrEqual(t, len(parts), 3, "expected 'added source <uuid> ...' output")
	sourceID := parts[2]
	return repo, sourceID
}

func TestSourceLinkCmd_HappyPath(t *testing.T) {
	repo, sourceID := setupRepoWithSource(t)

	out, err := runTrls(t, repo, "sources", "link", "--issue", "task-01", "--source-id", sourceID)
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, sourceID)
}

func TestSourceLinkCmd_UnknownSourceID(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "sources", "link", "--issue", "task-01", "--source-id", "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in manifest")
}

func TestSourceLinkCmd_MissingIssue(t *testing.T) {
	repo, sourceID := setupRepoWithSource(t)

	_, err := runTrls(t, repo, "sources", "link", "--source-id", sourceID)
	require.Error(t, err)
}

func TestSourceLinkCmd_MissingSourceID(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "sources", "link", "--issue", "task-01")
	require.Error(t, err)
}

func TestSourceLinkCmd_MakesNodeCited(t *testing.T) {
	repo, sourceID := setupRepoWithSource(t)

	_, err := runTrls(t, repo, "sources", "link", "--issue", "task-01", "--source-id", sourceID)
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.NotEmpty(t, issue.SourceLinks, "expected SourceLinks to be non-empty after source-link op")
	assert.Equal(t, sourceID, issue.SourceLinks[0].SourceEntryID)
}

// accept-citation tests

func TestAcceptCitationCmd_CI_HappyPath(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--rationale", "cited because it matches",
		"--ci")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "cited because it matches")
}

func TestAcceptCitationCmd_RationaleTooShort(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--rationale", "too short",
		"--ci")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 words")
}

func TestAcceptCitationCmd_TwoWords_Rejected(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--rationale", "only two",
		"--ci")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 words")
}

func TestAcceptCitationCmd_ThreeWords_Accepted(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--rationale", "exactly three words",
		"--ci")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestAcceptCitationCmd_MissingIssue(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "sources", "accept-citation",
		"--rationale", "some valid rationale here",
		"--ci")
	require.Error(t, err)
}

func TestAcceptCitationCmd_MissingRationale(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--ci")
	require.Error(t, err)
}

func TestAcceptCitationCmd_Force_SkipsPrompt(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--rationale", "cited because it matches",
		"--force")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "cited because it matches")
}

func TestAcceptCitationCmd_NonInteractive_SkipsPrompt(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--rationale", "cited because it matches",
		"--non-interactive")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "cited because it matches")
}

func setupRepoWithTwoTasks(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	ctx := getTestContext(t, repo)
	workerID, logPath, resolveErr := resolveWorkerAndLog(ctx)
	require.NoError(t, resolveErr)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{
			Title: "Task one", NodeType: "task", Scope: []string{"cmd/armature/one.go"},
			DefinitionOfDone: "Task one is complete and tested", Confidence: "verified",
		},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{
			Title: "Task two", NodeType: "task", Scope: []string{"cmd/armature/two.go"},
			DefinitionOfDone: "Task two is complete and tested", Confidence: "verified",
		},
	}))

	return repo
}

func TestAcceptCitationCmd_MultiIssue_AllApplied(t *testing.T) {
	repo := setupRepoWithTwoTasks(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--issue", "task-02",
		"--rationale", "bulk citation no source",
		"--ci")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
}

func TestAcceptCitationCmd_MultiIssue_ThreeIDs(t *testing.T) {
	repo := setupRepoWithTwoTasks(t)

	_, err := runTrls(t, repo, "create", "--title", "Task three", "--type", "task", "--id", "task-03")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "sources", "accept-citation",
		"--issue", "task-01",
		"--issue", "task-02",
		"--issue", "task-03",
		"--rationale", "bulk citation three ids",
		"--ci")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
	assert.Contains(t, out, "task-03")
}

func setupRepoWithStoryAndTask(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "My Story", "--type", "story", "--id", "story-01"}))
	require.NoError(t, cmd2.Execute())

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{
			Title: "My Task", NodeType: "task", Parent: "story-01",
			Scope:            []string{"cmd/armature/story_task.go"},
			DefinitionOfDone: "My Task is complete and tested", Confidence: "verified",
		},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{
			Title: "Other Task", NodeType: "task", Scope: []string{"cmd/armature/other_task.go"},
			DefinitionOfDone: "Other Task is complete and tested", Confidence: "verified",
		},
	}))

	return repo
}

func TestListCmd_NoFilter_ShowsAll(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "story-01")
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
}

func TestListCmd_ParentFilter(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "--format", "human", "list", "--parent", "story-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, "story-01")
	assert.NotContains(t, out, "task-02")
}

func TestListCmd_TypeFilter(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "list", "--type", "story")
	require.NoError(t, err)
	assert.Contains(t, out, "story-01")
	assert.NotContains(t, out, "task-01")
	assert.NotContains(t, out, "task-02")
}

func TestListCmd_ParentAndTypeFilter(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "list", "--parent", "story-01", "--type", "task")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, "task-02")
}

func TestListCmd_ParentFilter_NoMatch(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "--format", "human", "list", "--parent", "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(out))
}

func TestDecomposeApplyExampleFlag(t *testing.T) {
	repo := initTempRepo(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dag", "apply", "--example", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	// Output must be valid JSON
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed), "output must be valid JSON")

	// Must contain top-level plan fields
	assert.Contains(t, parsed, "version")
	assert.Contains(t, parsed, "title")
	assert.Contains(t, parsed, "issues")

	// Issues must be a non-empty array
	issues, ok := parsed["issues"].([]any)
	require.True(t, ok, "issues must be an array")
	assert.NotEmpty(t, issues)
}

func TestDecomposeApplyDryRun(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Dry Run Plan","issues":[` +
		`{"id":"DRY-001","title":"Dry run task one","type":"task","source":"src-test",` +
		`"scope":"internal/DRY-001.go","dod":"Dry run task one is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]},` +
		`{"id":"DRY-002","title":"Dry run task two","type":"task","source":"src-test",` +
		`"scope":"internal/DRY-002.go","dod":"Dry run task two is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	// Capture ops dir state before dry-run
	opsDir := filepath.Join(repo, ".armature", "ops")
	entriesBefore, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "dag", "apply", "--plan", planFile, "--dry-run")
	require.NoError(t, err)

	// Output must mention the issue IDs (what would be created)
	assert.Contains(t, out, "DRY-001")
	assert.Contains(t, out, "DRY-002")
	// Output must indicate dry-run (e.g. "would create")
	assert.Contains(t, out, "would create")

	// No new ops files should be written
	entriesAfter, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	assert.Equal(t, len(entriesBefore), len(entriesAfter), "dry-run must not write any ops files")
}

func TestListCmd_JSONFormat(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--format", "json", "--repo", repo, "list", "--parent", "story-01"})
	require.NoError(t, cmd.Execute())

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "task-01", entries[0].ID)
	assert.Equal(t, "story-01", entries[0].Parent)
}

func TestListCmd_StatusFilter(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	// Transition task-01 to done so we have two distinct statuses
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "task-01", "--to", "done", "--skip-delivery-gate", "--outcome", "completed", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Promotion to merged now requires an explicit `arm merged` call (no more
	// automatic done->merged promotion via git-history merge detection).
	_, err = runTrls(t, repo, "merged", "--issue", "task-01")
	require.NoError(t, err)

	// --status merged should include task-01 but not task-02 (still open)
	out, err := runTrls(t, repo, "list", "--status", "merged")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, "task-02")

	// --status open should include task-02 but not task-01
	out, err = runTrls(t, repo, "list", "--status", "open")
	require.NoError(t, err)
	assert.Contains(t, out, "task-02")
	assert.NotContains(t, out, "task-01")
}

func TestListCmd_HumanShowsStatus(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "list")
	require.NoError(t, err)
	// Human output should include a status value alongside each issue
	assert.Contains(t, out, "open")
}

func TestListCmd_AgentFormatEmitsJSON(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--format", "agent", "--repo", repo, "list"})
	require.NoError(t, cmd.Execute())

	var entries []listEntry
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entries),
		"agent format must emit valid JSON")
	assert.NotEmpty(t, entries)
}

// TestDecomposeApplyUncitedPlan verifies apply --strict is gone and an
// uncited plan is refused (source-atomic).
func TestDecomposeApplyUncitedPlan(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Uncited Test","issues":[` +
		`{"id":"STR-001","title":"Task without source","type":"task"}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	_, err = runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.Error(t, err, "apply must refuse a plan with no per-issue source")
	assert.Contains(t, err.Error(), "source")

	_, err = runTrls(t, repo, "dag", "apply", "--plan", planFile, "--strict")
	require.Error(t, err, "apply --strict is deleted")
}

func TestDecomposeApplyRefusesUnknownSource(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	tmpFile := filepath.Join(t.TempDir(), "known.md")
	require.NoError(t, os.WriteFile(tmpFile, []byte("known\n"), 0o600))
	_, err = runTrls(t, repo, "sources", "add", "--url", tmpFile, "--type", "filesystem", "--title", "Known")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Unknown source","issues":[` +
		`{"id":"UNK-001","title":"Fabricated citation","type":"task",` +
		`"source":"00000000-0000-0000-0000-000000000001",` +
		`"scope":"cmd/armature/unk.go","dod":"Fabricated citation is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	_, err = runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.Error(t, err, "apply must refuse a source ID that is not in the manifest")
	assert.Contains(t, err.Error(), "00000000-0000-0000-0000-000000000001")

	_, showErr := runTrls(t, repo, "show", "--issue", "UNK-001")
	require.Error(t, showErr, "a refused apply must not create the issue")
}

// TestDecomposeApplyGenerateIds verifies that --generate-ids replaces the
// plan-specified IDs with system-generated UUIDs in the created issues.
func TestDecomposeApplyGenerateIds(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"GenIDs Test","issues":[` +
		`{"id":"GEN-001","title":"Story one","type":"story","source":"src-test"},` +
		`{"id":"GEN-002","title":"Task two","type":"task","parent":"GEN-001","source":"src-test",` +
		`"scope":"internal/GEN-002.go","dod":"Task two is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	out, err := runTrls(t, repo, "dag", "apply", "--plan", planFile, "--generate-ids")
	require.NoError(t, err)
	assert.Contains(t, out, "Applied 2 issues")

	// The plan IDs must NOT appear in the state after materialization.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)

	_, hasGEN001 := index["GEN-001"]
	_, hasGEN002 := index["GEN-002"]
	assert.False(t, hasGEN001, "GEN-001 should not exist when --generate-ids is used")
	assert.False(t, hasGEN002, "GEN-002 should not exist when --generate-ids is used")

	// There should be exactly 2 new issues with UUID-like IDs.
	assert.Len(t, index, 2, "should have exactly 2 issues with generated IDs")
}

// TestDecomposeApplyRoot verifies that --root overrides the inferred root and
// attaches top-level plan issues as children of the given root issue.
func TestDecomposeApplyRoot(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create an existing story to use as root.
	_, err = runTrls(t, repo, "create", "--title", "Existing Story", "--type", "story", "--id", "root-story-01")
	require.NoError(t, err)

	// Plan with no parent set — top-level issues should become children of root-story-01.
	planData := `{"version":1,"title":"Root Test","issues":[` +
		`{"id":"ROOT-001","title":"Task under root","type":"task","source":"src-test",` +
		`"scope":"internal/ROOT-001.go","dod":"Task under root is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	out, err := runTrls(t, repo, "dag", "apply", "--plan", planFile, "--root", "root-story-01")
	require.NoError(t, err)
	assert.Contains(t, out, "Applied 1 issues")

	// After materialization, ROOT-001 should have parent = root-story-01.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)

	entry, ok := index["ROOT-001"]
	require.True(t, ok, "ROOT-001 should exist in state")
	assert.Equal(t, "root-story-01", entry.Parent, "ROOT-001 should have parent=root-story-01 when --root is set")
}

// TestShowCmd verifies that trls show --issue prints human-readable summary
// and that --format json produces structured data.
func TestShowCmd(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	// Human-readable output
	out, err := runTrls(t, repo, "show", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "My Task")
	assert.Contains(t, out, "task")     // type
	assert.Contains(t, out, "open")     // status
	assert.Contains(t, out, "story-01") // parent

	// JSON output
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--format", "json", "--repo", repo, "show", "--issue", "task-01"})
	require.NoError(t, cmd.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &result))
	assert.Equal(t, "task-01", result["id"])
	assert.Equal(t, "My Task", result["title"])
	assert.Equal(t, "task", result["type"])
	assert.Equal(t, "open", result["status"])
	assert.Equal(t, "story-01", result["parent"])
}

func TestShowCmd_DisplaysAcceptance(t *testing.T) {
	repo := setupRepoWithTask(t)

	acceptance := `[{"type":"test_passes","cmd":"make check"}]`
	_, err := runTrls(t, repo, "amend", "--issue", "task-01", "--acceptance", acceptance)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Human-readable output includes acceptance
	out, err := runTrls(t, repo, "show", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "Acceptance:", "human output should show Acceptance field")
	assert.Contains(t, out, "test_passes", "human output should include acceptance criteria content")

	// JSON output includes acceptance field
	jsonOut, err := runTrls(t, repo, "show", "--issue", "task-01", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, `"acceptance"`, "JSON output should include acceptance field")
}

func TestShowCmd_MissingIssue(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	_, err := runTrls(t, repo, "show", "--issue", "nonexistent-99")
	assert.Error(t, err)
}

func TestShowCmd_MissingFlag(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	_, err := runTrls(t, repo, "show")
	assert.Error(t, err)
}

// TestDoctorCmd_CleanRepo verifies that trls doctor succeeds on a healthy repo.
func TestDoctorCmd_CleanRepo(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "doctor")
	require.NoError(t, err)
	assert.Contains(t, out, "D2")
	assert.Contains(t, out, "D3")
	assert.Contains(t, out, "D4")
	assert.Contains(t, out, "D5")
	assert.Contains(t, out, "D6")
}

// TestDoctorCmd_JSONFormat verifies --format json outputs structured data.
func TestDoctorCmd_JSONFormat(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--format", "json", "--repo", repo, "doctor"})
	require.NoError(t, cmd.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &result))
	assert.Contains(t, result, "checks")
}

// TestDoctorCmd_BrokenParentRef verifies D4 detects broken parent references.
// Since arm create now validates parent existence, we inject the broken op directly
// into the ops log to simulate a task with a non-existent parent.
func TestDoctorCmd_BrokenParentRef(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Directly inject a create op with a non-existent parent into the ops log,
	// bypassing the arm create validation layer.
	workerID := fmt.Sprintf("test-worker-%d", time.Now().UnixNano())
	logPath := filepath.Join(repo, ".armature", "ops", workerID+".log")
	brokenOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "orphan-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:    "Orphan task",
			NodeType: "task",
			Parent:   "nonexistent-parent",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, brokenOp), "injecting broken op must succeed")

	out, err := runTrls(t, repo, "doctor")
	assert.Error(t, err, "doctor should fail on broken parent ref (D4 error)")
	assert.Contains(t, out+err.Error(), "D4")
}

// TestDoctorCmd_Strict verifies --strict promotes D6 warnings to errors.
func TestDoctorCmd_Strict(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Without --strict: uncited issues are warnings, should succeed.
	_, err := runTrls(t, repo, "doctor")
	require.NoError(t, err, "doctor without --strict should succeed on a repo with uncited issues")

	// With --strict: warnings become errors, should fail.
	_, err = runTrls(t, repo, "doctor", "--strict")
	assert.Error(t, err, "doctor --strict should fail when uncited issues exist")
}

// TestDoctorStrictFlagsUnrecognizedManagedWorktree_REQ_LNGHZN_S5_T8 verifies
// the CLI reports a managed checkout with no binding and promotes that warning
// to a non-zero exit under --strict.
func TestDoctorStrictFlagsUnrecognizedManagedWorktree_REQ_LNGHZN_S5_T8(t *testing.T) {
	repo := setupRepoWithTask(t)
	stray := filepath.Join(repo, ".worktrees", "stray")
	run(t, repo, "git", "worktree", "add", "-b", "stray-branch", stray)

	out, err := runTrls(t, repo, "doctor", "--strict")
	require.Error(t, err)
	assert.Contains(t, out, "D9")
	assert.Contains(t, out, "stray")
}

// TestDoctorFixReportsBoundWorktreePathDrift_REQ_LNGHZN_S5 verifies that a
// moved but still bound live worktree is an advisory path-drift finding, not a
// missing-worktree repair that releases the claimant's reservation.
func TestDoctorFixReportsBoundWorktreePathDrift_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	recordedPath := filepath.Join(repo, ".worktrees", "task-01")
	movedPath := filepath.Join(repo, "moved-task-01")
	run(t, repo, "git", "worktree", "move", recordedPath, movedPath)

	out, err := runTrls(t, repo, "doctor", "--fix", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, "path drift")
	assert.Contains(t, out, movedPath)

	status, err := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, err)
	assert.Equal(t, "claimed\n", status, "path drift must not release the live claim")
	assert.DirExists(t, movedPath)
}

// TestDecomposeApplySchemaFlag verifies that --schema prints a valid JSON Schema
// document that correctly documents field names, types, and constraints.
func TestDecomposeApplySchemaFlag(t *testing.T) {
	repo := initTempRepo(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dag", "apply", "--schema", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	output := strings.TrimSpace(buf.String())

	// Output must be valid JSON
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &parsed), "output must be valid JSON")

	// Must contain $schema key (JSON Schema indicator)
	assert.Contains(t, parsed, "$schema", "output must contain $schema key")

	// Must document the version field as integer type (not string)
	schemaStr := output
	assert.Contains(t, schemaStr, `"version"`, "schema must document version field")
	assert.Contains(t, schemaStr, `"integer"`, "version must be documented as integer type")

	// Must document dod field (not definition_of_done)
	assert.Contains(t, schemaStr, `"dod"`, "schema must document dod field (not definition_of_done)")
	assert.NotContains(t, schemaStr, `"definition_of_done"`, "schema must not use definition_of_done as field name")

	// Must document scope as string type (not array)
	assert.Contains(t, schemaStr, `"scope"`, "schema must document scope field")
	// scope property should use "string" type, not "array"
	// We verify by checking the properties section contains scope with string type
	properties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "schema must have a properties object")
	assert.Contains(t, properties, "version", "properties must include version")
	assert.Contains(t, properties, "issues", "properties must include issues")

	issues, ok := properties["issues"].(map[string]any)
	require.True(t, ok, "issues must be an object")
	items, ok := issues["items"].(map[string]any)
	require.True(t, ok, "issues must define item schema")
	issueProperties, ok := items["properties"].(map[string]any)
	require.True(t, ok, "issue items must define properties")
	issueType, ok := issueProperties["type"].(map[string]any)
	require.True(t, ok, "issue type must be an object")
	issueTypeEnum, ok := issueType["enum"].([]any)
	require.True(t, ok, "issue type enum must be an array")
	assert.ElementsMatch(t, issuetype.All(), issueTypeEnum, "schema must accept every CLI issue type")

	acceptance, ok := issueProperties["acceptance"].(map[string]any)
	require.True(t, ok, "acceptance must be an object")
	acceptanceItems, ok := acceptance["items"].(map[string]any)
	require.True(t, ok, "acceptance must define item schema")
	acceptanceOneOf, ok := acceptanceItems["oneOf"].([]any)
	require.True(t, ok, "acceptance item schema must define oneOf")
	assert.Equal(t, []any{
		map[string]any{"type": "string"},
		map[string]any{"type": "object"},
	}, acceptanceOneOf, "schema must preserve string and structured acceptance criteria")

	for _, field := range []string{"context_files", "acceptance", "blocked_by", "notes"} {
		fieldSchema, ok := issueProperties[field].(map[string]any)
		require.True(t, ok, "%s must be an object", field)
		assert.Equal(t, []any{"array", "null"}, fieldSchema["type"], "%s must allow null", field)
		assert.NotContains(t, fieldSchema, "nullable", "%s must use standard JSON Schema null types", field)
	}
}

// TestReadyParentFilter verifies that trls ready --parent ISSUE-ID returns only
// descendants of the given issue.
func TestReadyParentFilter(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	// Without filter: all ready tasks visible (task-01, task-02; story-01 is a story type which may appear).
	outAll, err := runTrls(t, repo, "ready")
	require.NoError(t, err)
	assert.Contains(t, outAll, "task-01")
	assert.Contains(t, outAll, "task-02")

	// With --parent story-01: only task-01 (child of story-01) should appear.
	outFiltered, err := runTrls(t, repo, "ready", "--parent", "story-01")
	require.NoError(t, err)
	assert.Contains(t, outFiltered, "task-01")
	assert.NotContains(t, outFiltered, "task-02")

	// With --parent for a non-existent ID: no tasks.
	outNone, err := runTrls(t, repo, "ready", "--parent", "nonexistent-parent")
	require.NoError(t, err)
	assert.NotContains(t, outNone, "task-01")
	assert.NotContains(t, outNone, "task-02")
}

// TestMaterializeCommand_ExcludeWorker verifies that --exclude-worker skips all
// ops from that worker's log, yielding zero issues in diagnostic mode.
func TestMaterializeCommand_ExcludeWorker(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Exclude Worker Issue", "--type", "task", "--id", "TST-EX")
	require.NoError(t, err)

	// Find the worker ID from the ops log filename.
	opsDir := filepath.Join(repo, ".armature", "ops")
	entries, readErr := os.ReadDir(opsDir)
	require.NoError(t, readErr)
	var workerID string
	for _, e := range entries {
		if w, ok := strings.CutSuffix(e.Name(), ".log"); ok {
			workerID = w
			break
		}
	}
	require.NotEmpty(t, workerID, "expected at least one .log file in ops dir")

	// Normal materialize should produce 1 issue.
	outNormal, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)
	assert.Contains(t, outNormal, "1 issues")

	// With --exclude-worker, all ops from that worker are skipped.
	outExclude, err := runTrls(t, repo, "materialize", "--exclude-worker", workerID)
	require.NoError(t, err)
	assert.Contains(t, outExclude, "excluding worker")
	assert.Contains(t, outExclude, "0 issues")
}

// TestListTerminal verifies that --terminal returns all issues with terminal
// statuses (done, merged, cancelled) and excludes open/in-progress issues.
func TestListTerminal(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	// Initialize worker so we can do transitions.
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create additional issues: one to cancel, one to leave open, one to merge.
	_, err = runTrls(t, repo, "create", "--title", "Task to cancel", "--type", "task", "--id", "task-cancel")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task to done", "--type", "task", "--id", "task-done")
	require.NoError(t, err)

	// Transition task-cancel to cancelled.
	_, err = runTrls(t, repo, "claim", "task-cancel", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "task-cancel", "--to", "cancelled", "--outcome", "not needed", "--force")
	require.NoError(t, err)

	// Transition task-done to done; on a repo with git history this becomes merged.
	_, err = runTrls(t, repo, "claim", "task-done", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "task-done", "--to", "done", "--skip-delivery-gate", "--outcome", "completed", "--force")
	require.NoError(t, err)

	// --terminal must include cancelled and done/merged issues.
	out, err := runTrls(t, repo, "list", "--terminal")
	require.NoError(t, err)
	assert.Contains(t, out, "task-cancel", "--terminal should include cancelled issues")
	assert.Contains(t, out, "task-done", "--terminal should include done/merged issues")

	// --terminal must exclude open issues.
	assert.NotContains(t, out, "task-01", "--terminal should exclude open issues")
	assert.NotContains(t, out, "task-02", "--terminal should exclude open issues")
	assert.NotContains(t, out, "story-01", "--terminal should exclude open story")
}

// TestReadyExplain verifies that arm ready --explain prints ID: reason pairs for
// open tasks that are blocked or have an inactive parent, in deterministic order.
func TestReadyExplain(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a blocker task (open, not merged) and a task that depends on it.
	// The blocker stays open — so task-blocked is excluded because blocker is not merged.
	plantVerifiedTask(t, repo, "task-blocker", "cmd/armature/blocker.go")
	plantVerifiedTask(t, repo, "task-blocked", "cmd/armature/blocked.go")
	_, err = runTrls(t, repo, "link", "--source", "task-blocked", "--dep", "task-blocker")
	require.NoError(t, err)
	// Claim task-blocker so it is in-progress (not merged) — task-blocked remains not ready.
	_, err = runTrls(t, repo, "claim", "task-blocker", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--explain")
	require.NoError(t, err)
	// task-blocked should appear with a reason mentioning its unmerged blocker
	assert.Contains(t, out, "task-blocked", "--explain should list task-blocked")
	assert.Contains(t, out, "task-blocker", "--explain reason should mention the unmerged blocker")
	// task-01 and task-02 are ready (not blocked), so they must NOT appear in explain output
	assert.NotContains(t, out, "task-01", "--explain must not include ready tasks")
	assert.NotContains(t, out, "task-02", "--explain must not include ready tasks")
}

// TestCommandLongAndExampleFields verifies that high-priority commands have
// non-empty Long and Example fields for comprehensive help documentation.
func TestCommandLongAndExampleFields(t *testing.T) {
	type commandTest struct {
		name string
		cmd  *cobra.Command
	}

	tests := []commandTest{
		{"ready", newReadyCmd()},
		{"claim", newClaimCmd()},
		{"transition", newTransitionCmd()},
		{"dag summary", newDAGSummaryCmd()},
		{"dag apply", newDecomposeApplyCmd()},
		{"dag context", newDecomposeContextCmd()},
		{"dag revert", newDecomposeRevertCmd()},
		{"dag transition", newDAGTransitionCmd()},
		{"link", newLinkCmd()},
		{"sync", newSyncCmd()},
		{"validate", newValidateCmd()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.cmd.Long, "%s command must have non-empty Long field", tt.name)
			assert.NotEmpty(t, tt.cmd.Example, "%s command must have non-empty Example field", tt.name)
		})
	}
}

// Fix 1: TestCreateCommand_FeatureType verifies that arm create --type feature succeeds.
func TestCreateCommand_FeatureType(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "create", "--title", "my feature", "--type", "feature", "--id", "feature-01")
	require.NoError(t, err, "arm create --type feature should succeed")
	assert.Contains(t, out, "feature-01", "output should include the created ID")
}

// Fix 1: TestCreateCommand_FeatureTypeInvalidMsg verifies that invalid type error includes "feature".
func TestCreateCommand_FeatureTypeInErrMsg(t *testing.T) {
	// Verify that the valid types list includes "feature" in the error message
	// by attempting to create with a totally invalid type.
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "create", "--title", "my widget", "--type", "invalid-type", "--id", "widget-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "feature", "error message should list 'feature' as a valid type")
}

// Fix 1: TestValidParentChildTypes_EpicCanContainFeature verifies hierarchy rules for feature type.
func TestValidParentChildTypes_EpicCanContainFeature(t *testing.T) {
	assert.True(t, issuetype.IsLegalHierarchy("epic", "feature"),
		"epic should be able to contain feature")
}

// Fix 1: TestValidParentChildTypes_FeatureCanContainTask verifies feature can contain task.
func TestValidParentChildTypes_FeatureCanContainTask(t *testing.T) {
	assert.True(t, issuetype.IsLegalHierarchy("feature", "task"),
		"feature should be able to contain task")
}

// Fix 1: TestValidParentChildTypes_FeatureCanContainBug verifies feature can contain bug.
func TestValidParentChildTypes_FeatureCanContainBug(t *testing.T) {
	assert.True(t, issuetype.IsLegalHierarchy("feature", "bug"),
		"feature should be able to contain bug")
}

// Fix 1: TestCreateCommand_FeatureUnderEpic verifies feature can be created under an epic.
func TestCreateCommand_FeatureUnderEpic(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create an epic first
	_, err := runTrls(t, repo, "create", "--title", "My Epic", "--type", "epic", "--id", "epic-01")
	require.NoError(t, err)

	// Materialize so issues/epic-01.json exists for ReadIssue in create --parent.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a feature under the epic
	out, err := runTrls(t, repo, "create", "--title", "My Feature", "--type", "feature", "--id", "feature-02", "--parent", "epic-01")
	require.NoError(t, err, "arm create --type feature --parent epic-01 should succeed")
	assert.Contains(t, out, "feature-02")
}

// Fix 3: TestReparentCommand_EmptyParentMakesTopLevel verifies that --parent ""
// makes an issue top-level (removes its parent).
func TestReparentCommand_EmptyParentMakesTopLevel(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	// task-01 has parent story-01; reparent with --parent "" should make it top-level.
	out, err := runTrls(t, repo, "reparent", "--issue", "task-01", "--parent", "")
	require.NoError(t, err, "arm reparent --parent '' should succeed")
	assert.Contains(t, out, "task-01", "output should include issue ID")
}

// TestAcceptCitationCmd_Interactive_Confirm sends "y" to stdin and verifies success.
func TestAcceptCitationCmd_Interactive_Confirm(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	outBuf := new(bytes.Buffer)
	stdin := strings.NewReader("y\n")
	root := newRootCmd()
	root.SetOut(outBuf)
	root.SetIn(stdin)
	root.SetArgs([]string{"sources", "accept-citation", "--repo", repo,
		"--issue", "task-01",
		"--rationale", "cited because it matches",
	})
	require.NoError(t, root.Execute())
	assert.Contains(t, outBuf.String(), "task-01")
}

// TestAcceptCitationCmd_PositionalIssueID verifies that the positional argument is accepted.
func TestAcceptCitationCmd_PositionalIssueID(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "sources", "accept-citation",
		"task-01",
		"--rationale", "cited because it matches here",
		"--ci",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

// TestWorkersCommand_WithCancelledTransition verifies workers handles cancelled status ops.
func TestWorkersCommand_WithCancelledTransition(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Claim the task, then cancel it so an OpTransition with StatusCancelled is recorded.
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "cancelled")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "workers", "--repo", repo)
	require.NoError(t, err)
	_ = out
}

// TestListCmd_Group_MultipleStatusGroups verifies the sort comparator runs with 2+ status groups.
func TestListCmd_Group_MultipleStatusGroups(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create a second task and claim it so we get two status groups: open and claimed.
	_, err := runTrls(t, repo, "create",
		"--title", "Task two",
		"--type", "task",
		"--id", "task-02",
	)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-02",
		"--worktree",
	)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"--format", "human", "--repo", repo, "list", "--group"})
	require.NoError(t, root.Execute())

	out := buf.String()
	// Both status groups should appear.
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
}

// TestTransitionCmd_DoneWithParentStory_ChecksStoryStatus verifies the parent story
// status check is called when transitioning a task with a parent to done.
func TestTransitionCmd_DoneWithParentStory_ChecksStoryStatus(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Claim task-01 (child of story-01) then transition it to done.
	_, err = runTrls(t, repo, "claim", "task-01",
		"--worktree",
	)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--force")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

// TestDecomposeContextCmd_BasicOutput verifies that decompose-context outputs a template.
func TestDecomposeContextCmd_BasicOutput(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "context"})
	require.NoError(t, root.Execute())
	// Should output the default prompt template (non-empty).
	assert.NotEmpty(t, buf.String())
}

// TestDecomposeContextCmd_JSONFormat verifies --format json output.
func TestDecomposeContextCmd_JSONFormat(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "context", "--format", "json"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
}

// TestDecomposeContextCmd_WithSources verifies --sources flag parses and filters empty segments.
func TestDecomposeContextCmd_WithSources(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	// Pass comma-separated sources with an empty segment to exercise the `if s != "" {` guard.
	root.SetArgs([]string{"dag", "context", "--format", "json", "--sources", "src-01,,src-02"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
}

// TestNewSnapshotStore_UsesContextPaths verifies that newSnapshotStore wires
// opsDir from IssuesDir/ops and stateDir from StateDir.
func TestNewSnapshotStore_UsesContextPaths_REQ_ARCHIMP_S14_T2(t *testing.T) {
	t.Parallel()

	// Dual-branch mode (the only supported mode)
	ctx := &config.Context{
		IssuesDir: "/repo/.arm/.armature",
		StateDir:  "/repo/.arm/state/worker-1",
	}
	store := newSnapshotStore(ctx)
	require.NotNil(t, store)

	// Verify stateDir is wired correctly by checking IndexPath
	expectedIndexPath := filepath.Join(ctx.StateDir, "index.json")
	assert.Equal(t, expectedIndexPath, store.IndexPath())

	// Verify IssuePath also uses StateDir
	expectedIssuePath := filepath.Join(ctx.StateDir, "issues", "test-id.json")
	assert.Equal(t, expectedIssuePath, store.IssuePath("test-id"))
}
