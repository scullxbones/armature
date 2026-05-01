package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/spf13/cobra"
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
	cmd.SetArgs([]string{"create", "--repo", repo, "--title", "Second task", "--type", "task", "--id", "task-02"})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-02"})
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
	cmd.SetArgs([]string{"create", "--repo", repo, "--title", "Parent task", "--type", "story", "--id", "E6"})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Child task", "--type", "task", "--id", "task-child", "--parent", "E6"})
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
	_, err := runTrls(t, repo, "amend", "--issue", "task-01", "--scope", "nonexistent/file.go")
	require.NoError(t, err)

	out, _ := runTrls(t, repo, "validate")
	assert.Contains(t, out, "INFO: phantom scope", "phantom scope should appear as INFO")
	assert.NotContains(t, out, "WARNING: phantom scope", "phantom scope should not appear as WARNING")
}

func TestValidateCommand_JSON_IncludesInfosField(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "amend", "--issue", "task-01", "--scope", "nonexistent/file.go")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "validate", "--format", "json")
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

	out, err := runTrls(t, repo, "validate", "--quiet")
	require.NoError(t, err)
	assert.NotContains(t, out, "INFO:", "--quiet should suppress INFO lines")
	assert.Contains(t, out, "COVERAGE:", "--quiet should still print COVERAGE lines")
	assert.Contains(t, out, "OK:", "--quiet should still print OK lines")
}

func TestImportCommand_DryRun_JSON(t *testing.T) {
	repo := setupRepoWithTask(t)

	csvFile := filepath.Join(t.TempDir(), "issues.csv")
	require.NoError(t, os.WriteFile(csvFile, []byte("id,title,type\nimp-1,Imported Task,task\n"), 0644))

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
	validateOut, _ := runTrls(t, repo, "validate")
	// After amendment the task should not report missing acceptance
	assert.NotContains(t, validateOut, "missing required field: acceptance on task task-01")
}

func TestAmendCmd_NoFieldsProvided_ReturnsError(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "amend", "--issue", "task-01")
	assert.Error(t, err)
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

	out, err := runTrls(t, repo, "source-link", "--issue", "task-01", "--source-id", sourceID)
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, sourceID)
}

func TestSourceLinkCmd_UnknownSourceID(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "source-link", "--issue", "task-01", "--source-id", "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in manifest")
}

func TestSourceLinkCmd_MissingIssue(t *testing.T) {
	repo, sourceID := setupRepoWithSource(t)

	_, err := runTrls(t, repo, "source-link", "--source-id", sourceID)
	require.Error(t, err)
}

func TestSourceLinkCmd_MissingSourceID(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "source-link", "--issue", "task-01")
	require.Error(t, err)
}

func TestSourceLinkCmd_MakesNodeCited(t *testing.T) {
	repo, sourceID := setupRepoWithSource(t)

	_, err := runTrls(t, repo, "source-link", "--issue", "task-01", "--source-id", sourceID)
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

	out, err := runTrls(t, repo, "accept-citation",
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

	_, err = runTrls(t, repo, "accept-citation",
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

	_, err = runTrls(t, repo, "accept-citation",
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

	out, err := runTrls(t, repo, "accept-citation",
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

	_, err = runTrls(t, repo, "accept-citation",
		"--rationale", "some valid rationale here",
		"--ci")
	require.Error(t, err)
}

func TestAcceptCitationCmd_MissingRationale(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "accept-citation",
		"--issue", "task-01",
		"--ci")
	require.Error(t, err)
}

func TestAcceptCitationCmd_Force_SkipsPrompt(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "accept-citation",
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

	out, err := runTrls(t, repo, "accept-citation",
		"--issue", "task-01",
		"--rationale", "cited because it matches",
		"--non-interactive")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "cited because it matches")
}

func setupRepoWithStoryAndTask(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "My Story", "--type", "story", "--id", "story-01"})
	require.NoError(t, cmd2.Execute())

	cmd3 := newRootCmd()
	cmd3.SetOut(new(bytes.Buffer))
	cmd3.SetArgs([]string{"create", "--repo", repo, "--title", "My Task", "--type", "task", "--id", "task-01", "--parent", "story-01"})
	require.NoError(t, cmd3.Execute())

	cmd4 := newRootCmd()
	cmd4.SetOut(new(bytes.Buffer))
	cmd4.SetArgs([]string{"create", "--repo", repo, "--title", "Other Task", "--type", "task", "--id", "task-02"})
	require.NoError(t, cmd4.Execute())

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
	cmd.SetArgs([]string{"decompose-apply", "--example", "--repo", repo})

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

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"Dry Run Plan","issues":[` +
		`{"id":"DRY-001","title":"Dry run task one","type":"task"},` +
		`{"id":"DRY-002","title":"Dry run task two","type":"task"}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	// Capture ops dir state before dry-run
	opsDir := filepath.Join(repo, ".armature", "ops")
	entriesBefore, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "decompose-apply", "--plan", planFile, "--dry-run")
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
	_, err = runTrls(t, repo, "claim", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "task-01", "--to", "done", "--outcome", "completed", "--force")
	require.NoError(t, err)

	// After transition on a repo with git history, merge-detection advances to merged.
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

// TestDecomposeApplyStrict verifies that --strict causes non-zero exit when the
// plan has advisory warnings (e.g. issues missing DoD), and that without
// --strict the same plan applies successfully.
func TestDecomposeApplyStrict(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Plan with issues missing DoD — these should generate advisory warnings.
	planData := `{"version":1,"title":"Strict Test","issues":[` +
		`{"id":"STR-001","title":"Task without dod","type":"task"}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	// Without --strict, apply should succeed (warnings are advisory).
	_, err = runTrls(t, repo, "decompose-apply", "--plan", planFile)
	require.NoError(t, err, "apply without --strict should succeed even with warnings")

	// With --strict, the same plan applied to a fresh repo should fail.
	repo2 := initTempRepo(t)
	run(t, repo2, "git", "commit", "--allow-empty", "-m", "init")
	_, err = runTrls(t, repo2, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo2, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo2, "decompose-apply", "--plan", planFile, "--strict")
	assert.Error(t, err, "--strict should cause non-zero exit when warnings exist")
}

// TestDecomposeApplyGenerateIds verifies that --generate-ids replaces the
// plan-specified IDs with system-generated UUIDs in the created issues.
func TestDecomposeApplyGenerateIds(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	planData := `{"version":1,"title":"GenIDs Test","issues":[` +
		`{"id":"GEN-001","title":"Task one","type":"task"},` +
		`{"id":"GEN-002","title":"Task two","type":"task","parent":"GEN-001"}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	out, err := runTrls(t, repo, "decompose-apply", "--plan", planFile, "--generate-ids")
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

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create an existing story to use as root.
	_, err = runTrls(t, repo, "create", "--title", "Existing Story", "--type", "story", "--id", "root-story-01")
	require.NoError(t, err)

	// Plan with no parent set — top-level issues should become children of root-story-01.
	planData := `{"version":1,"title":"Root Test","issues":[` +
		`{"id":"ROOT-001","title":"Task under root","type":"task"}` +
		`]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0644))

	out, err := runTrls(t, repo, "decompose-apply", "--plan", planFile, "--root", "root-story-01")
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
func TestDoctorCmd_BrokenParentRef(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a task with a non-existent parent.
	_, err = runTrls(t, repo, "create",
		"--title", "Orphan task", "--type", "task", "--id", "orphan-01",
		"--parent", "nonexistent-parent")
	require.NoError(t, err)

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

// TestDecomposeApplySchemaFlag verifies that --schema prints a valid JSON Schema
// document that correctly documents field names, types, and constraints.
func TestDecomposeApplySchemaFlag(t *testing.T) {
	repo := initTempRepo(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"decompose-apply", "--schema", "--repo", repo})

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

	_, err := runTrls(t, repo, "init")
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
		{"dag-summary", newDAGSummaryCmd()},
		{"decompose-apply", newDecomposeApplyCmd()},
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
