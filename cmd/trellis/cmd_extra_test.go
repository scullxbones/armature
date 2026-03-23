package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmCommand_Success(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Confirm an existing issue
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"confirm", "--repo", repo, "task-01"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "confirmed task-01")
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
	cmd0.SetArgs([]string{"create", "--repo", repo,
		"--title", "Draft feature", "--type", "task", "--id", "draft-01",
		"--confidence", "draft"})
	require.NoError(t, cmd0.Execute())

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dag-summary", "--repo", repo, "--format", "agent"})

	err := cmd.Execute()
	require.NoError(t, err)
	// Non-interactive mode with draft items outputs JSON
	assert.Contains(t, buf.String(), "pending_dag_confirmation")
}

func TestReadyCommand_JSONFormat(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"ready", "--repo", repo, "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[")
}

func TestImportCommand_DryRun_CSV(t *testing.T) {
	repo := setupRepoWithTask(t)

	csvFile := filepath.Join(t.TempDir(), "issues.csv")
	err := os.WriteFile(csvFile, []byte("id,title,type\nimp-1,Imported Task,task\n"), 0644)
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
	require.NoError(t, os.WriteFile(csvFile, []byte("id,title,type\nimp-1,Imported Task,task\n"), 0644))

	out, err := runTrls(t, repo, "import", csvFile)
	require.NoError(t, err)
	assert.Contains(t, out, "imported 1 items")
}

func TestStaleReviewCommand_NoStale(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "stale-review", "--format", "agent")
	require.NoError(t, err)
	assert.Contains(t, out, "No stale sources detected")
}

func TestDecomposeRevertCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Test Plan","issues":[{"id":"REV-001","title":"Revertable","type":"task"}]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	_, err = runTrls(t, repo, "decompose-apply", "--plan", planFile)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "decompose-revert", "--plan", planFile)
	require.NoError(t, err)
	assert.Contains(t, out, "Reverted")
}

// TestDecomposeApply_DraftConfidence verifies that nodes created by decompose-apply
// have confidence=draft, are hidden from trls ready, and become visible after
// dag-transition promotes them to verified.
func TestDecomposeApply_DraftConfidence(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Draft Test","issues":[` +
		`{"id":"DRF-001","title":"Draft task one","type":"task"},` +
		`{"id":"DRF-002","title":"Draft task two","type":"task"},` +
		`{"id":"DRF-003","title":"Draft task three","type":"task"}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	// Apply the plan — all nodes should be created as draft
	out, err := runTrls(t, repo, "decompose-apply", "--plan", planFile)
	require.NoError(t, err)
	assert.Contains(t, out, "Applied 3 issues")

	// trls ready should NOT list draft nodes
	readyOut, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, readyOut, "DRF-001")
	assert.NotContains(t, readyOut, "DRF-002")
	assert.NotContains(t, readyOut, "DRF-003")

	// Promote via dag-transition on each root node (they have no parent so promote each)
	_, err = runTrls(t, repo, "dag-transition", "--issue", "DRF-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "dag-transition", "--issue", "DRF-002")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "dag-transition", "--issue", "DRF-003")
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

	// Sync — triggers providerForType
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

	out, err := runTrls(t, repo, "validate", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, "{")
}

func TestImportCommand_DryRun_JSON(t *testing.T) {
	repo := setupRepoWithTask(t)

	csvFile := filepath.Join(t.TempDir(), "issues.csv")
	require.NoError(t, os.WriteFile(csvFile, []byte("id,title,type\nimp-1,Imported Task,task\n"), 0644))

	out, err := runTrls(t, repo, "import", "--dry-run", "--format", "json", csvFile)
	require.NoError(t, err)
	assert.Contains(t, out, "dry_run")
}
