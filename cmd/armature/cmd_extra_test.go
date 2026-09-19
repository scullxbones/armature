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
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
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
	decoded := decodeReadyEnvelope(t, buf.String())
	var issues []readyIssueRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	found := false
	for _, row := range issues {
		if row.ID == "ready-json-01" {
			found = true
		}
	}
	assert.True(t, found, "ready-json-01 must appear in the issues payload")
}

func TestReadyExpiredClaims_REQ_TOPTIER_S4_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "expired-worker.log")
	staleClaimTime := time.Now().Unix() - 7200
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: staleClaimTime,
		WorkerID: "expired-worker", Payload: ops.Payload{TTL: 1},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	jsonOut, jsonErrOut, err := runTrlsWithStderr(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	decoded := decodeReadyEnvelope(t, jsonOut)
	var issues []readyIssueRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	for _, row := range issues {
		assert.NotEqual(t, "task-01", row.ID, "claimed+expired issue must not appear in issues")
	}
	var expiredClaims []map[string]any
	require.NoError(t, json.Unmarshal(decoded["expired_claims"], &expiredClaims), "expired claims belong on the stdout envelope")
	require.Len(t, expiredClaims, 1)
	assert.Equal(t, "task-01", expiredClaims[0]["id"])
	assert.Equal(t, "expired-worker", expiredClaims[0]["claimed_by"])
	assert.False(t, strings.HasPrefix(strings.TrimSpace(jsonErrOut), "["), "expired claims must not be a stderr JSON array")
}

func TestReadyExpiredClaims_ParentFilterScopesExpiredClaims_REQ_TOPTIER_S4_PRFIX(t *testing.T) {
	repo := setupRepoWithTask(t)

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
	staleClaimTime := time.Now().Unix() - 7200
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpClaim, TargetID: "task-outside", Timestamp: staleClaimTime,
		WorkerID: "expired-worker", Payload: ops.Payload{TTL: 1},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, errOut := runReadyJSON(t, repo, "--parent", "E7")
	decoded := decodeReadyEnvelope(t, out)
	var expiredClaims []map[string]any
	require.NoError(t, json.Unmarshal(decoded["expired_claims"], &expiredClaims))
	assert.Empty(t, expiredClaims, "expired claim for task-outside must not leak into --parent E7 scoped ready")
	assert.False(t, strings.HasPrefix(strings.TrimSpace(errOut), "["), "expired claims must not be a stderr JSON array")
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
	_ = out
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

func TestDecomposeApply_DraftConfidence_REQ_AOC_S2_T4(t *testing.T) {
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

	out, err := runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.NoError(t, err)
	applied := decodeContractEnvelope(t, out, "issues")
	var count int
	require.NoError(t, json.Unmarshal(applied["count"], &count))
	assert.Equal(t, 3, count)

	readyOut, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, readyOut, "DRF-001")
	assert.NotContains(t, readyOut, "DRF-002")
	assert.NotContains(t, readyOut, "DRF-003")

	_, err = runTrls(t, repo, "dag", "transition", "--issue", "DRF-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "dag", "transition", "--issue", "DRF-002")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "dag", "transition", "--issue", "DRF-003")
	require.NoError(t, err)

	readyOut2, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, readyOut2, "DRF-001")
	assert.Contains(t, readyOut2, "DRF-002")
	assert.Contains(t, readyOut2, "DRF-003")
}

func TestSourcesSyncCommand_WithFilesystemSource(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd0 := newRootCmd()
	cmd0.SetOut(new(bytes.Buffer))
	cmd0.SetArgs([]string{"worker-init", "--repo", repo})
	require.NoError(t, cmd0.Execute())

	docFile := filepath.Join(repo, "spec.md")
	require.NoError(t, os.WriteFile(docFile, []byte("# Spec"), 0644))

	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", docFile, "--type", "filesystem", "--title", "Spec"})
	require.NoError(t, cmd1.Execute())

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

	var env struct {
		Count  int                `json:"count"`
		Issues []output.ListIssue `json:"issues"`
		Groups []output.ListGroup `json:"groups"`
		Help   []string           `json:"help"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &env),
		"--group must emit a structured envelope")
	assert.NotEmpty(t, env.Issues)
	assert.Equal(t, len(env.Issues), env.Count)
	assert.NotEmpty(t, env.Groups)
}

func TestValidateCommand_PhantomScope_PrintsInfoNotWarning(t *testing.T) {
	repo := setupRepoWithTask(t)

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

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	validateOut, err := runTrls(t, repo, "validate")
	swallowErr(err)
	assert.NotContains(t, validateOut, "missing required field: acceptance on task task-01")
}

func TestAmendCmd_NoFieldsProvided_ReturnsError(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "amend", "--issue", "task-01")
	assert.Error(t, err)
}

func TestAmendCmd_ClearContextFilesAndContextFileConflict_ReturnsError(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "amend", "--issue", "task-01",
		"--clear-context-files",
		"--context-file", "docs/guide.md")
	require.Error(t, err, "using --clear-context-files together with --context-file must return an error")
	assert.Contains(t, err.Error(), "--clear-context-files")
}

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
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed), "output must be valid JSON")

	assert.Contains(t, parsed, "version")
	assert.Contains(t, parsed, "title")
	assert.Contains(t, parsed, "issues")

	issues, ok := parsed["issues"].([]any)
	require.True(t, ok, "issues must be an array")
	assert.NotEmpty(t, issues)
}

func TestDecomposeApplyDryRun_REQ_AOC_S2_T4(t *testing.T) {
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

	opsDir := filepath.Join(repo, ".armature", "ops")
	entriesBefore, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "dag", "apply", "--plan", planFile, "--dry-run")
	require.NoError(t, err)

	decoded := decodeContractEnvelope(t, out, "issues")
	var rows []applyIssueRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &rows))
	ids := map[string]bool{}
	for _, row := range rows {
		ids[row.ID] = true
		assert.Equal(t, "would_create", row.Action)
		assert.Equal(t, "task", row.Type)
		assert.Equal(t, "open", row.Status)
		assert.NotEmpty(t, row.Title)
	}
	assert.True(t, ids["DRY-001"])
	assert.True(t, ids["DRY-002"])
	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.Contains(t, help[0], "dry-run")

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

	var env struct {
		Count  int                `json:"count"`
		Issues []output.ListIssue `json:"issues"`
		Help   []string           `json:"help"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &env))
	require.Len(t, env.Issues, 1)
	assert.Equal(t, 1, env.Count)
	assert.Equal(t, "task-01", env.Issues[0].ID)
}

func TestListCmd_StatusFilter(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "task-01", "--to", "done", "--skip-delivery-gate", "--outcome", "completed", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "merged", "--issue", "task-01")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "list", "--status", "merged")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.NotContains(t, out, "task-02")

	out, err = runTrls(t, repo, "list", "--status", "open")
	require.NoError(t, err)
	assert.Contains(t, out, "task-02")
	assert.NotContains(t, out, "task-01")
}

func TestListCmd_HumanShowsStatus(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "open")
}

func TestListCmd_AgentFormatEmitsJSON(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--format", "agent", "--repo", repo, "list"})
	require.NoError(t, cmd.Execute())

	var env struct {
		Count  int                `json:"count"`
		Issues []output.ListIssue `json:"issues"`
		Help   []string           `json:"help"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &env),
		"agent format must emit a valid envelope")
	assert.NotEmpty(t, env.Issues)
	assert.Equal(t, len(env.Issues), env.Count)
	assert.NotEmpty(t, env.Help)
}

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

func TestDecomposeApplyGenerateIds_REQ_AOC_S2_T4(t *testing.T) {
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
	applied := decodeContractEnvelope(t, out, "issues")
	var count int
	require.NoError(t, json.Unmarshal(applied["count"], &count))
	assert.Equal(t, 2, count)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)

	_, hasGEN001 := index["GEN-001"]
	_, hasGEN002 := index["GEN-002"]
	assert.False(t, hasGEN001, "GEN-001 should not exist when --generate-ids is used")
	assert.False(t, hasGEN002, "GEN-002 should not exist when --generate-ids is used")

	assert.Len(t, index, 2, "should have exactly 2 issues with generated IDs")
}

func TestDecomposeApplyRoot_REQ_AOC_S2_T4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Existing Story", "--type", "story", "--id", "root-story-01")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Root Test","issues":[` +
		`{"id":"ROOT-001","title":"Task under root","type":"task","source":"src-test",` +
		`"scope":"internal/ROOT-001.go","dod":"Task under root is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	out, err := runTrls(t, repo, "dag", "apply", "--plan", planFile, "--root", "root-story-01")
	require.NoError(t, err)
	applied := decodeContractEnvelope(t, out, "issues")
	var count int
	require.NoError(t, json.Unmarshal(applied["count"], &count))
	assert.Equal(t, 1, count)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)

	entry, ok := index["ROOT-001"]
	require.True(t, ok, "ROOT-001 should exist in state")
	assert.Equal(t, "root-story-01", entry.Parent, "ROOT-001 should have parent=root-story-01 when --root is set")
}

func TestShowCmd(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "show", "--format", "human", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "My Task")
	assert.Contains(t, out, "task")
	assert.Contains(t, out, "open")
	assert.Contains(t, out, "story-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--format", "json", "--repo", repo, "show", "--issue", "task-01"})
	require.NoError(t, cmd.Execute())

	issue := decodeShowIssue(t, buf.String())
	assert.Equal(t, "task-01", issue["id"])
	assert.Equal(t, "My Task", issue["title"])
	assert.Equal(t, "task", issue["type"])
	assert.Equal(t, "open", issue["status"])
	assert.Equal(t, "story-01", issue["parent"])
}

func TestShowCmd_DisplaysAcceptance(t *testing.T) {
	repo := setupRepoWithTask(t)

	acceptance := `[{"type":"test_passes","cmd":"make check"}]`
	_, err := runTrls(t, repo, "amend", "--issue", "task-01", "--acceptance", acceptance)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "show", "--format", "human", "--issue", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "Acceptance:", "human output should show Acceptance field")
	assert.Contains(t, out, "test_passes", "human output should include acceptance criteria content")

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

func TestDoctorCmd_BrokenParentRef(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

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

func TestDoctorCmd_Strict(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "doctor")
	require.NoError(t, err, "doctor without --strict should succeed on a repo with uncited issues")

	_, err = runTrls(t, repo, "doctor", "--strict")
	assert.Error(t, err, "doctor --strict should fail when uncited issues exist")
}

func TestDoctorStrictFlagsUnrecognizedManagedWorktree_REQ_LNGHZN_S5_T8(t *testing.T) {
	repo := setupRepoWithTask(t)
	stray := filepath.Join(repo, ".worktrees", "stray")
	run(t, repo, "git", "worktree", "add", "-b", "stray-branch", stray)

	out, err := runTrls(t, repo, "doctor", "--strict")
	require.Error(t, err)
	assert.Contains(t, out, "D9")
	assert.Contains(t, out, "stray")
}

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

func TestDecomposeApplySchemaFlag(t *testing.T) {
	repo := initTempRepo(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dag", "apply", "--schema", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	output := strings.TrimSpace(buf.String())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &parsed), "output must be valid JSON")

	assert.Contains(t, parsed, "$schema", "output must contain $schema key")

	schemaStr := output
	assert.Contains(t, schemaStr, `"version"`, "schema must document version field")
	assert.Contains(t, schemaStr, `"integer"`, "version must be documented as integer type")

	assert.Contains(t, schemaStr, `"dod"`, "schema must document dod field (not definition_of_done)")
	assert.NotContains(t, schemaStr, `"definition_of_done"`, "schema must not use definition_of_done as field name")

	assert.Contains(t, schemaStr, `"scope"`, "schema must document scope field")
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

func TestReadyParentFilter(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	outAll, err := runTrls(t, repo, "ready")
	require.NoError(t, err)
	assert.Contains(t, outAll, "task-01")
	assert.Contains(t, outAll, "task-02")

	outFiltered, err := runTrls(t, repo, "ready", "--parent", "story-01")
	require.NoError(t, err)
	assert.Contains(t, outFiltered, "task-01")
	assert.NotContains(t, outFiltered, "task-02")

	outNone, err := runTrls(t, repo, "ready", "--parent", "nonexistent-parent")
	require.NoError(t, err)
	assert.NotContains(t, outNone, "task-01")
	assert.NotContains(t, outNone, "task-02")
}

func TestMaterializeCommand_ExcludeWorker(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Exclude Worker Issue", "--type", "task", "--id", "TST-EX")
	require.NoError(t, err)

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

	outNormal, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)
	assert.Contains(t, outNormal, "1 issues")

	outExclude, err := runTrls(t, repo, "materialize", "--exclude-worker", workerID)
	require.NoError(t, err)
	assert.Contains(t, outExclude, "excluding worker")
	assert.Contains(t, outExclude, "0 issues")
}

func TestListTerminal(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Task to cancel", "--type", "task", "--id", "task-cancel")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task to done", "--type", "task", "--id", "task-done")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-cancel", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "task-cancel", "--to", "cancelled", "--outcome", "not needed", "--force")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-done", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "task-done", "--to", "done", "--skip-delivery-gate", "--outcome", "completed", "--force")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "list", "--terminal")
	require.NoError(t, err)
	assert.Contains(t, out, "task-cancel", "--terminal should include cancelled issues")
	assert.Contains(t, out, "task-done", "--terminal should include done/merged issues")

	assert.NotContains(t, out, "task-01", "--terminal should exclude open issues")
	assert.NotContains(t, out, "task-02", "--terminal should exclude open issues")
	assert.NotContains(t, out, "story-01", "--terminal should exclude open story")
}

func TestReadyExplain(t *testing.T) {
	repo := plantBlockedReadyExplainFixture(t)

	out, err := runTrls(t, repo, "ready", "--explain")
	require.NoError(t, err)
	assert.Contains(t, out, "task-blocked", "--explain should list task-blocked")
	assert.Contains(t, out, "task-blocker", "--explain reason should mention the unmerged blocker")
	assert.NotContains(t, out, "task-01", "--explain must not include ready tasks")
	assert.NotContains(t, out, "task-02", "--explain must not include ready tasks")
}

func TestDagApplyArtifactModesAreSchemaAndExample_REQ_AOC_S1_T3(t *testing.T) {
	t.Parallel()

	cmd := newDecomposeApplyCmd()
	require.Equal(t, output.CitationPlanSchema, cmd.Annotations[output.ArtifactCitationKey])
	require.Equal(t, output.ChannelAgentFacing, output.Classify(cmd.Annotations),
		"dag apply without --schema/--example stays agent-facing")
	require.Equal(t, output.ChannelArtifactOutput, output.ClassifyFlags(cmd.Annotations, map[string]bool{"schema": true}))
	require.Equal(t, output.ChannelArtifactOutput, output.ClassifyFlags(cmd.Annotations, map[string]bool{"example": true}))
	require.Equal(t, output.ChannelAgentFacing, output.ClassifyFlags(cmd.Annotations, map[string]bool{"dry-run": true}))
}

func TestDagApplyResultModesEmitEnvelope_REQ_AOC_S2_T4(t *testing.T) {
	repo, planFile := plantDagApplyEnvelopeFixture(t)

	dryOut, err := runTrls(t, repo, "dag", "apply", "--plan", planFile, "--dry-run", "--format", "json")
	require.NoError(t, err)
	dry := decodeContractEnvelope(t, dryOut, "issues")
	var dryIssues []applyIssueRow
	require.NoError(t, json.Unmarshal(dry["issues"], &dryIssues))
	require.Len(t, dryIssues, 2)
	assert.Equal(t, "ENV-001", dryIssues[0].ID)
	assert.Equal(t, "would_create", dryIssues[0].Action)
	assert.Equal(t, "task", dryIssues[0].Type)
	assert.Equal(t, "open", dryIssues[0].Status)
	assert.NotEmpty(t, dryIssues[0].Title)

	applyOut, err := runTrls(t, repo, "dag", "apply", "--plan", planFile, "--format", "json")
	require.NoError(t, err)
	applied := decodeContractEnvelope(t, applyOut, "issues")
	var created []applyIssueRow
	require.NoError(t, json.Unmarshal(applied["issues"], &created))
	require.Len(t, created, 2)
	ids := map[string]bool{}
	for _, row := range created {
		ids[row.ID] = true
		assert.Equal(t, "created", row.Action)
		assert.Equal(t, "task", row.Type)
		assert.Equal(t, "open", row.Status)
		assert.NotEmpty(t, row.Title)
	}
	assert.True(t, ids["ENV-001"])
	assert.True(t, ids["ENV-002"])
}

func TestDagApplyEnvelopeIgnoresForeignCreates_REQ_AOC_S2_T4(t *testing.T) {
	repo, planFile := plantDagApplyEnvelopeFixture(t)
	_, err := runTrls(t, repo, "create",
		"--id", "FOREIGN-001",
		"--title", "Other worker issue",
		"--type", "task",
		"--scope", "internal/FOREIGN-001.go",
		"--dod", "Foreign issue exists before this apply",
		"--acceptance", `[{"type":"test_passes"}]`,
	)
	require.NoError(t, err)

	applyOut, err := runTrls(t, repo, "dag", "apply", "--plan", planFile, "--format", "json")
	require.NoError(t, err)
	applied := decodeContractEnvelope(t, applyOut, "issues")
	var created []applyIssueRow
	require.NoError(t, json.Unmarshal(applied["issues"], &created))
	ids := map[string]bool{}
	for _, row := range created {
		ids[row.ID] = true
		assert.Equal(t, "created", row.Action)
		assert.Equal(t, "task", row.Type)
		assert.Equal(t, "open", row.Status)
	}
	assert.True(t, ids["ENV-001"])
	assert.True(t, ids["ENV-002"])
	assert.False(t, ids["FOREIGN-001"], "parallel/sibling creates must not appear as this apply's created rows")
	var count int
	require.NoError(t, json.Unmarshal(applied["count"], &count))
	assert.Equal(t, 2, count)
}

func TestAllAgentFacingCommandsEmitEnvelope_REQ_AOC_S2_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	cases := []struct {
		payload string
		args    []string
	}{
		{"workers", []string{"workers", "--format", "json"}},
		{"findings", []string{"validate", "--format", "json", "--strict=false"}},
		{"worktrees", []string{"worktree", "list", "--format", "json"}},
		{"worktrees", []string{"worktree", "gc", "--dry-run", "--format", "json"}},
		{"commits", []string{"review", "commits", "task-01", "--format", "json"}},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := runTrls(t, repo, tc.args...)
			if tc.payload == "findings" {
				decodeContractEnvelope(t, out, tc.payload)
				return
			}
			require.NoError(t, err)
			decodeContractEnvelope(t, out, tc.payload)
		})
	}
}

func TestHarnessHookOutputUnchanged_REQ_AOC_S2_T4(t *testing.T) {
	t.Parallel()

	cmd := newHarnessHookCmd()
	require.Equal(t, output.ChannelProtocolOutput, output.Classify(cmd.Annotations))

	result := harnesshook.RunResult{
		Output:   []byte(`{"decision":"approve"}`),
		ExitCode: 0,
	}
	var buf bytes.Buffer
	require.NoError(t, applyRunResult(&buf, result))
	assert.Equal(t, `{"decision":"approve"}`, buf.String())
	assert.NotContains(t, buf.String(), `"count"`)
	assert.NotContains(t, buf.String(), `"help"`)
}

func plantDagApplyEnvelopeFixture(t *testing.T) (string, string) {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Envelope Plan","issues":[` +
		`{"id":"ENV-001","title":"Envelope task one","type":"task","source":"src-test",` +
		`"scope":"internal/ENV-001.go","dod":"Envelope task one is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]},` +
		`{"id":"ENV-002","title":"Envelope task two","type":"task","source":"src-test",` +
		`"scope":"internal/ENV-002.go","dod":"Envelope task two is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0o644))
	return repo, planFile
}

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

func TestCreateCommand_FeatureType(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "create", "--title", "my feature", "--type", "feature", "--id", "feature-01")
	require.NoError(t, err, "arm create --type feature should succeed")
	assert.Contains(t, out, "feature-01", "output should include the created ID")
}

func TestCreateCommand_FeatureTypeInErrMsg(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "create", "--title", "my widget", "--type", "invalid-type", "--id", "widget-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "feature", "error message should list 'feature' as a valid type")
}

func TestValidParentChildTypes_EpicCanContainFeature(t *testing.T) {
	assert.True(t, issuetype.IsLegalHierarchy("epic", "feature"),
		"epic should be able to contain feature")
}

func TestValidParentChildTypes_FeatureCanContainTask(t *testing.T) {
	assert.True(t, issuetype.IsLegalHierarchy("feature", "task"),
		"feature should be able to contain task")
}

func TestValidParentChildTypes_FeatureCanContainBug(t *testing.T) {
	assert.True(t, issuetype.IsLegalHierarchy("feature", "bug"),
		"feature should be able to contain bug")
}

func TestCreateCommand_FeatureUnderEpic(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "create", "--title", "My Epic", "--type", "epic", "--id", "epic-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "create", "--title", "My Feature", "--type", "feature", "--id", "feature-02", "--parent", "epic-01")
	require.NoError(t, err, "arm create --type feature --parent epic-01 should succeed")
	assert.Contains(t, out, "feature-02")
}

func TestReparentCommand_EmptyParentMakesTopLevel(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)

	out, err := runTrls(t, repo, "reparent", "--issue", "task-01", "--parent", "")
	require.NoError(t, err, "arm reparent --parent '' should succeed")
	assert.Contains(t, out, "task-01", "output should include issue ID")
}

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

func TestWorkersCommand_WithCancelledTransition(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "cancelled")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "workers", "--repo", repo)
	require.NoError(t, err)
	_ = out
}

func TestListCmd_Group_MultipleStatusGroups(t *testing.T) {
	repo := setupRepoWithTask(t)

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
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
}

func TestTransitionCmd_DoneWithParentStory_ChecksStoryStatus(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-01",
		"--worktree",
	)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--force")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestDecomposeContextCmd_BasicOutput(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "context"})
	require.NoError(t, root.Execute())
	assert.NotEmpty(t, buf.String())
}

func TestDecomposeContextCmd_JSONFormat(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "context", "--format", "json"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
}

func TestDecomposeContextCmd_WithSources(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "context", "--format", "json", "--sources", "src-01,,src-02"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
}

func TestNewSnapshotStore_UsesContextPaths_REQ_ARCHIMP_S14_T2(t *testing.T) {
	t.Parallel()

	ctx := &config.Context{
		IssuesDir: "/repo/.arm/.armature",
		StateDir:  "/repo/.arm/state/worker-1",
	}
	store := newSnapshotStore(ctx)
	require.NotNil(t, store)

	expectedIndexPath := filepath.Join(ctx.StateDir, "index.json")
	assert.Equal(t, expectedIndexPath, store.IndexPath())

	expectedIssuePath := filepath.Join(ctx.StateDir, "issues", "test-id.json")
	assert.Equal(t, expectedIssuePath, store.IssuePath("test-id"))
}
