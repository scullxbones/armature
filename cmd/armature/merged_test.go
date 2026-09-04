package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/deliverygate"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIssueWorktreeHasViolations_FailsClosedOnUnreadableInventory_REQ_LNGHZN_S5
// verifies that a worktree-inventory read error propagates out of the merged
// violation gate instead of being swallowed into "no violations". Returning
// ok=false on an inventory error would let `arm merged` append the merged op and
// exit 0 while the target worktree is UNREADABLE and may contain violations —
// a fail-OPEN gate violating I5 (deterministic gates fail closed) and I6
// (done ≠ merged).
func TestIssueWorktreeHasViolations_FailsClosedOnUnreadableInventory_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()
	// A tempdir with no git repository makes `git worktree list` fail, so the
	// inventory cannot be read.
	notARepo := t.TempDir()
	issue := materialize.Issue{ID: "task-01", Type: "task"}

	_, err := issueWorktreeHasViolations(notARepo, issue)
	require.Error(t, err, "unreadable inventory must fail closed, not report zero violations")
}

// TestRemoveWorktreeForIssueTracked_FailsClosedOnUnreadableInventory_REQ_LNGHZN_S5
// verifies the removal path also propagates the inventory read error (outcome
// stays worktreeSkipped) rather than silently skipping teardown.
func TestRemoveWorktreeForIssueTracked_FailsClosedOnUnreadableInventory_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()
	notARepo := t.TempDir()
	issue := materialize.Issue{ID: "task-01", Type: "task"}

	outcome, err := removeWorktreeForIssueTracked(notARepo, issue, new(bytes.Buffer))
	require.Error(t, err, "unreadable inventory must surface an error, not a silent skip")
	assert.Equal(t, worktreeSkipped, outcome)
}

// TestIssueWorktreeHasViolations_FailsClosedOnAmbiguousMarkers_REQ_LNGHZN_S5
// verifies that when more than one worktree carries the issue's marker and none
// uniquely matches the recorded WorktreePath (a legacy explicit-path worktree
// alongside the canonical .worktrees/<id> one, with an empty WorktreePath), the
// merged violation gate FAILS CLOSED with an error rather than returning
// ok=false ("nothing to gate"). Returning ok=false would let `arm merged`
// append the merged op and exit 0 without inspecting either candidate's hook
// log — a fail-OPEN gate violating I5 (deterministic gates fail closed) and I6
// (done ≠ merged). This mirrors how gc treats duplicate markers as GCAmbiguous
// and exits non-zero (TestReconcile_GCAmbiguousDuplicateMarkersRemovesNothing).
func TestIssueWorktreeHasViolations_FailsClosedOnAmbiguousMarkers_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Claim to create the canonical .worktrees/task-01 worktree bound to task-01.
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Create a SECOND, legacy explicit-path worktree and bind it to the SAME
	// issue marker, producing a duplicate-marker set.
	legacyPath := filepath.Join(t.TempDir(), "legacy-worktree")
	run(t, repo, "git", "worktree", "add", legacyPath, "-b", "legacy/task-01")
	legacyGitDir, err := worktree.ResolveGitDir(legacyPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(legacyGitDir, "armature-issue-id"),
		[]byte("task-01\n"), 0o600))

	// With an empty recorded WorktreePath, SelectByIssue cannot disambiguate the
	// two binding-bound worktrees. The gate must surface an error, not (false,nil).
	issue := materialize.Issue{ID: "task-01", Type: "task"}
	_, gateErr := issueWorktreeHasViolations(repo, issue)
	require.Error(t, gateErr, "ambiguous binding-bound worktrees must fail closed, not report zero violations")
	assert.Contains(t, gateErr.Error(), "ambiguous", "error should name the ambiguity condition")
}

// TestMergedCmd_DoesNotMaterialize verifies that arm merged reads the index and issue from
// disk via store.ReadIndex()/store.ReadIssue() without triggering full rematerialization.
//
// RED with store.Load(): Load calls MaterializeAndReturnQuiet which rewrites checkpoint.json,
// advancing its mtime → mtime assertion fails.
// GREEN with store.ReadIndex()+store.ReadIssue(): no materialization → checkpoint.json mtime unchanged.
func TestMergedCmd_DoesNotMaterialize(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Materialize after creation.
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Transition task-01 to done.
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--outcome", "Completed", "--force")
	require.NoError(t, err)

	// Materialize again to update index.json (done→merged in single-branch mode).
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Capture checkpoint.json mtime before running merged.
	stateDir := getTestStateDir(t, repo)
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	stat, statErr := os.Stat(checkpointPath)
	require.NoError(t, statErr, "checkpoint.json should exist after materialize")
	mtimeBefore := stat.ModTime()

	// Run merged — must use ReadIndex+ReadIssue, not Load.
	_, err = runTrls(t, repo, "merged", "--issue", "task-01", "--force")
	require.NoError(t, err)

	// Verify checkpoint.json was NOT rewritten (no rematerialization occurred).
	statAfter, statErr := os.Stat(checkpointPath)
	require.NoError(t, statErr)
	assert.Equal(t, mtimeBefore, statAfter.ModTime(),
		"checkpoint.json must not be updated by arm merged: store.ReadIndex/ReadIssue must be used, not store.Load")
}

// TestMergedRemovesTaskWorktree verifies that merged removes a worktree for task-type issues.
func TestMergedRemovesTaskWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Call merged command
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	require.NoError(t, mergedCmd.Execute())

	// Verify worktree is removed
	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

func TestMergedRemovesArmatureOwnedCustomExclusion_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "custom-merged")
	excludePath := filepath.Join(repo, ".git", "info", "exclude")

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", destination)
	require.NoError(t, err)
	exclude, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.NotContains(t, string(exclude), "custom-merged", "external custom destinations must not change shared exclusions")

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "merged", "--issue", "task-01", "--force")
	require.NoError(t, err)

	exclude, err = os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.NotContains(t, string(exclude), "custom-merged", "external custom destinations must not change shared exclusions")
	assert.NoDirExists(t, destination, "merged teardown must remove the custom worktree")
}

// TestMergedPreservesDirtyWorktree_REQ_LNGHZN_S5 verifies the public teardown
// boundary refuses to discard either tracked or untracked work. `--force` is
// an override for the merged gate, not authorization to delete a worker's
// files.
func TestMergedPreservesDirtyWorktree_REQ_LNGHZN_S5(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, repo, worktreePath string)
	}{
		{
			name: "tracked changes",
			prepare: func(t *testing.T, repo, worktreePath string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("before\n"), 0o600))
				run(t, repo, "git", "add", "tracked.txt")
				run(t, repo, "git", "commit", "-m", "test: add tracked fixture")
				require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "tracked.txt"), []byte("after\n"), 0o600))
			},
		},
		{
			name: "untracked changes",
			prepare: func(t *testing.T, repo, worktreePath string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "untracked.txt"), []byte("preserve me\n"), 0o600))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupRepoWithTask(t)
			worktreePath := filepath.Join(repo, ".worktrees", "task-01")

			claim := newRootCmd()
			claim.SetOut(new(bytes.Buffer))
			claim.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
			require.NoError(t, claim.Execute())
			recordedBranch, recorded, recordErr := deliverygate.RecordedClaimedBranch(worktreePath)
			require.NoError(t, recordErr)
			require.True(t, recorded)
			recordedBase, baseErr := deliverygate.RecordedBaseCommit(worktreePath)
			require.NoError(t, baseErr)
			parentBefore := strings.TrimSpace(runGitOutput(t, repo, "config", "--get", deliverygate.ParentBranchConfigKey(recordedBranch)))
			require.NotEmpty(t, parentBefore)

			tc.prepare(t, repo, worktreePath)
			transition := newRootCmd()
			transition.SetOut(new(bytes.Buffer))
			transition.SetArgs(enrichTestCLIArgs([]string{
				"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
				"--skip-delivery-gate", "--force", "--outcome", "complete",
			}))
			require.NoError(t, transition.Execute())
			_, err := runTrls(t, repo, "materialize")
			require.NoError(t, err)

			merged := newRootCmd()
			merged.SetOut(new(bytes.Buffer))
			merged.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--force"})
			err = merged.Execute()
			require.Error(t, err, "dirty worktree teardown must fail even with --force")
			assert.DirExists(t, worktreePath)
			branchAfter, stillRecorded, recordErr := deliverygate.RecordedClaimedBranch(worktreePath)
			require.NoError(t, recordErr)
			assert.True(t, stillRecorded)
			assert.Equal(t, recordedBranch, branchAfter)
			baseAfter, baseErr := deliverygate.RecordedBaseCommit(worktreePath)
			require.NoError(t, baseErr)
			assert.Equal(t, recordedBase, baseAfter)
			assert.Equal(t, parentBefore, strings.TrimSpace(runGitOutput(t, repo, "config", "--get", deliverygate.ParentBranchConfigKey(recordedBranch))))
			if tc.name == "tracked changes" {
				contents, readErr := os.ReadFile(filepath.Join(worktreePath, "tracked.txt"))
				require.NoError(t, readErr)
				assert.Equal(t, "after\n", string(contents))
			} else {
				contents, readErr := os.ReadFile(filepath.Join(worktreePath, "untracked.txt"))
				require.NoError(t, readErr)
				assert.Equal(t, "preserve me\n", string(contents))
			}
		})
	}
}

// TestMergedClearsParentBranchMetadataFromRecordedClaim_REQ_LNGHZN_S5_T9
// verifies teardown uses immutable claim-time provenance rather than the
// branch currently checked out in the worktree.
func TestMergedClearsParentBranchMetadataFromRecordedClaim_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	claim, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err, claim)

	parentBefore := strings.TrimSpace(runGitOutput(t, repo, "config", "--get", "branch.task/task-01.armature-parent"))
	require.NotEmpty(t, parentBefore)
	run(t, worktreePath, "git", "checkout", "-b", "scratch/parked")
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "complete")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "merged", "--issue", "task-01")
	require.NoError(t, err)
	_, configErr := exec.CommandContext(context.Background(), "git", "-C", repo, "config", "--get", "branch.task/task-01.armature-parent").Output()
	assert.Error(t, configErr, "recorded task branch config must be cleared after successful scratch-branch teardown")
}

// TestMergedRemovesBugWorktree verifies that merged removes a worktree for bug-type issues.
func TestMergedRemovesBugWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test bug", "--type", "bug", "--id", "bug-01"}))
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(repo, ".worktrees", "bug-01")

	// Claim the bug to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "bug-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition bug to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{
		"transition", "--repo", repo, "--issue", "bug-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", "Fixed", "--force",
	}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, err2 := runTrls(t, repo, "materialize")
	require.NoError(t, err2)

	// Call merged command
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "bug-01"})
	require.NoError(t, mergedCmd.Execute())

	// Verify worktree is removed
	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

// TestMergedHandlesStoryWithNoActiveWorktree verifies that merged handles gracefully
// a story-type issue when no worktree was created for it (e.g. no --worktree used at claim time).
// Stories now map to feat/<id> via deriveBranchName, so merged will attempt worktree removal,
// but must not fail when no matching worktree exists.
func TestMergedHandlesStoryWithNoActiveWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test story", "--type", "story", "--id", "story-01"}))
	require.NoError(t, cmd2.Execute())

	// Transition story to done before calling merged
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "story-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Delivered", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json and issues/story-01.json exist before calling merged.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command (no worktree was created for this story; command must handle gracefully)
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "story-01", "--force"})
	require.NoError(t, mergedCmd.Execute())
}

// TestMergedRemovesStoryWorktree verifies that merged removes the worktree for a story-type
// issue when one was created via claim. Stories map to feat/<id> via deriveBranchName, so
// merged must tear down the story worktree just like task/bug/feature worktrees.
func TestMergedRemovesStoryWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test story", "--type", "story", "--id", "story-01"}))
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(repo, ".worktrees", "story-01")

	// Claim the story to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "story-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition story to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "story-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Delivered", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "story-01"})
	require.NoError(t, mergedCmd.Execute())

	// Verify worktree is removed
	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

// TestMergedRemovesFeatureWorktree verifies that merged removes a worktree for feature-type issues
// (F2: deriveBranchName maps feature → feat/, and merged must tear down every type that claim creates).
func TestMergedRemovesFeatureWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test feature", "--type", "feature", "--id", "feature-01"}))
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(repo, ".worktrees", "feature-01")

	// Claim the feature to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "feature-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition feature to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "feature-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Shipped", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "feature-01", "--force"})
	require.NoError(t, mergedCmd.Execute())

	// Verify worktree is removed
	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

// TestMergedHandlesFeatureWithNoWorktree verifies that merged gracefully handles
// a feature-type issue with no associated worktree (e.g., not created via --worktree).
func TestMergedHandlesFeatureWithNoWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test feature", "--type", "feature", "--id", "feature-01"}))
	require.NoError(t, cmd2.Execute())

	// Transition feature to done before calling merged
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "feature-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Shipped", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json and issues/feature-01.json exist before calling merged.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command (no worktree was created; should handle gracefully)
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "feature-01", "--force"})
	require.NoError(t, mergedCmd.Execute())
}

// TestMergedWarnsOnPassThroughEntries verifies that merged warns to stderr when armature-hook.log contains pass-through entries.
func TestMergedWarnsOnPassThroughEntries(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Create armature-hook.log with pass-through entries
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	hookLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	hookLogContent := "2026-07-04T00:00:00Z pass-through: no task binding found\n2026-07-04T00:00:01Z pass-through: stale binding\n"
	err = os.WriteFile(hookLogPath, []byte(hookLogContent), 0o600) //nolint:gosec // test path under temp directory
	require.NoError(t, err)

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command and capture stderr
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.NoError(t, err)

	// Verify warning is printed to stderr
	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "pass-through", "should warn about pass-through entries in stderr")
	assert.Contains(t, errOutput, "task-01", "warning should mention the issue ID")
}

// TestMergedNoWarningWithoutPassThroughEntries verifies that merged does not warn when hook.log has no pass-through entries.
func TestMergedNoWarningWithoutPassThroughEntries(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Optionally create armature-hook.log without pass-through entries (or don't create it at all)
	// Either way, no warning should be printed

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, errMat2 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat2)

	// Call merged command and capture stderr
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.NoError(t, err)

	// Verify no warning is printed (or only success message)
	errOutput := errBuf.String()
	assert.NotContains(t, errOutput, "pass-through", "should not warn about pass-through when none exist")
}

// TestMergedMissingWorktreeFailsClosed_REQ_LNGHZN_S5 verifies that a done
// issue cannot become merged when its required hook-log target disappeared:
// done is not merged, and absence is not proof that no violation occurred.
func TestMergedMissingWorktreeFailsClosed_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition.
	_, errMat3 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat3)

	// Manually remove the worktree (simulating it being deleted before merged is called)
	run(t, repo, "git", "worktree", "remove", "--force", worktreePath)

	// A done issue has lost the evidence target, so merged must not append a
	// merged op or silently treat the missing worktree as clean.
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no worktree or hook-log target")
	status, showErr := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, showErr)
	assert.Equal(t, "done\n", status, "missing target must leave the issue done, not merged")
}

// TestMergedUnreadableHookLogFailsClosed_REQ_LNGHZN_S5 verifies that a hook
// log which exists but cannot be read is not collapsed into "no violations".
func TestMergedUnreadableHookLogFailsClosed_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	claim, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err, claim)

	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(gitDir, "armature-hook.log"), 0o700), "a directory at the log path is unreadable as a log")

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--force", "--outcome", "complete")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "merged", "--issue", "task-01")
	require.Error(t, err)
	status, showErr := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, showErr)
	assert.Equal(t, "done\n", status, "unreadable hook evidence must leave the issue done")
}

// TestMergedDoesNotWarnWhenWorktreeAlreadyRemoved verifies that when the worktree has
// already been deleted before `arm merged` is called, no pass-through warning is emitted
// because the gate fails closed before it can inspect a hook log.
func TestMergedDoesNotWarnWhenWorktreeAlreadyRemoved(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Read the actual git dir from the worktree so we can write the hook log.
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	hookLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	err = os.WriteFile(hookLogPath, []byte("pass-through: no task binding found\n"), 0o600) //nolint:gosec // test path
	require.NoError(t, err)

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition.
	_, errMat4 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat4)

	// Remove the worktree before calling merged
	run(t, repo, "git", "worktree", "remove", "--force", worktreePath)

	// Call merged command and capture stderr
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.Error(t, err)

	// The implementation can only check hook log entries when the worktree is still
	// present (because it needs to find it via git worktree list). After removal
	// it is unavailable, so no pass-through warning can be emitted.
	assert.NotContains(t, errBuf.String(), "pass-through", "no warning expected when worktree is already gone")
}

// TestMergedRejectsNonDoneStatus verifies that merged requires status=done or status=merged.
// It rejects issues in other statuses (e.g., in-progress, open) to prevent accidental
// worktree cleanup of incomplete work.
func TestMergedRejectsNonDoneStatus(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap in dual-branch mode
	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"}))
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition to in-progress (NOT to done)
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "in-progress"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so the in-progress status is reflected in index.json before merged reads it.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command — should fail because status is not done/merged
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.Error(t, err, "merged should reject in-progress status")
	assert.Contains(t, err.Error(), "status=done", "error message should indicate done status required")

	// Verify worktree still exists (should not be deleted on error)
	assert.DirExists(t, worktreePath, "worktree should NOT be removed when merged fails")
}

// TestMergedRecordsOpBeforeRemovingWorktree verifies the P2 bug fix.
// The bug (pre-fix): removeWorktreeForIssue is called BEFORE appendOp (lines 152-154 in buggy code),
// so if appendOp fails, the worktree is already deleted and recovery is impossible.
// The fix: move removeWorktreeForIssue to AFTER appendOp succeeds, so on failure the worktree is preserved.
//
// Happy path: merged command executes successfully → worktree is removed.
// Failure path: ops dir made read-only so appendOp fails → worktree is NOT removed (recovery possible).
func TestMergedRecordsOpBeforeRemovingWorktree(t *testing.T) {
	t.Run("happy path: op recorded and worktree removed", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		worktreePath := filepath.Join(repo, ".worktrees", "task-01")

		claimCmd := newRootCmd()
		claimCmd.SetOut(new(bytes.Buffer))
		claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
		require.NoError(t, claimCmd.Execute())
		assert.DirExists(t, worktreePath, "worktree should exist after claim")

		transitionCmd := newRootCmd()
		transitionCmd.SetOut(new(bytes.Buffer))
		transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
			"--outcome", "Completed", "--force"}))
		require.NoError(t, transitionCmd.Execute())

		// Materialize so index.json reflects the done→merged transition before calling merged.
		_, errMat := runTrls(t, repo, "materialize")
		require.NoError(t, errMat)

		mergedCmd := newRootCmd()
		mergedCmd.SetOut(new(bytes.Buffer))
		mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
		require.NoError(t, mergedCmd.Execute())

		// Op succeeded → worktree should be removed
		assert.NoDirExists(t, worktreePath, "worktree should be removed after successful merged")
	})

	t.Run("failure path: appendOp fails → worktree preserved", func(t *testing.T) {
		// Use dual-branch mode so status=done after transition (not auto-advanced to merged),
		// ensuring appendOp is called and the read-only test exercises the actual P2 invariant:
		// removeWorktreeForIssue must not be called when appendOp fails.
		repo := initTempRepo(t)
		run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

		bootstrapCmd := newRootCmd()
		bootstrapCmd.SetOut(new(bytes.Buffer))
		bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
		require.NoError(t, bootstrapCmd.Execute())

		_, err := runTrls(t, repo, "worker-init")
		require.NoError(t, err)

		createCmd := newRootCmd()
		createCmd.SetOut(new(bytes.Buffer))
		createCmd.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"}))
		require.NoError(t, createCmd.Execute())

		worktreePath := filepath.Join(repo, ".worktrees", "task-01")
		claimCmd := newRootCmd()
		claimCmd.SetOut(new(bytes.Buffer))
		claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
		require.NoError(t, claimCmd.Execute())
		assert.DirExists(t, worktreePath, "worktree should exist after claim")

		transitionCmd := newRootCmd()
		transitionCmd.SetOut(new(bytes.Buffer))
		transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
			"--outcome", "Completed", "--force"}))
		require.NoError(t, transitionCmd.Execute())

		// Materialize so index.json reflects status=done (dual-branch: done is NOT auto-advanced to merged).
		// merged will read status=done → alreadyMerged=false → try appendOp → fails (read-only).
		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)

		// Make the ops log directory read-only so appendOp cannot write a new log entry.
		// This simulates a disk-full or permission error during the op write.
		// In dual-branch mode, ops live in the .arm worktree: <repo>/.arm/.armature/ops/.
		opsDir := filepath.Join(repo, ".armature", "ops")
		require.NoError(t, os.Chmod(opsDir, 0o444))
		defer func() {
			if chmodErr := os.Chmod(opsDir, 0o755); chmodErr != nil {
				t.Logf("warning: failed to restore ops dir permissions: %v", chmodErr)
			}
		}() // restore so cleanup works

		mergedCmd := newRootCmd()
		mergedCmd.SetOut(new(bytes.Buffer))
		mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
		err = mergedCmd.Execute()
		require.Error(t, err, "merged should fail when appendOp cannot write")

		// The critical invariant: because the op write failed BEFORE removeWorktreeForIssue
		// was called, the worktree must still be present and recovery is possible.
		assert.DirExists(t, worktreePath, "worktree must NOT be removed when appendOp fails")
	})
}

// TestMergedRecordsPROnRetry tests the P2 bug fix: when a new --pr flag is provided
// with a different PR number, the merge op must be recorded even if the issue is
// already in 'merged' status. This ensures merged captures the PR reference without
// silently discarding it.
//
// The bug: the idempotent skip (entry.Status == ops.StatusMerged) unconditionally skips
// re-recording, so the PR field stays empty even when --pr is provided.
//
// The fix: only skip op re-recording if there is no new PR to attach OR if the issue
// already has the same PR recorded.
func TestMergedRecordsPROnRetry(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap in dual-branch mode
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(new(bytes.Buffer))
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	workerCmd := newRootCmd()
	workerCmd.SetOut(new(bytes.Buffer))
	workerCmd.SetArgs([]string{"worker-init", "--repo", repo})
	require.NoError(t, workerCmd.Execute())

	// Create a task
	createCmd := newRootCmd()
	createCmd.SetOut(new(bytes.Buffer))
	createCmd.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"}))
	require.NoError(t, createCmd.Execute())

	// Materialize to initialize state
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Claim the task
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Materialize to update status
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Transition to done (stays as done in dual-branch mode, not auto-advanced to merged)
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize to finalize the transition
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// First merged call with PR 123: transitions status to merged and records PR
	mergedCmd1 := newRootCmd()
	mergedCmd1.SetOut(new(bytes.Buffer))
	mergedCmd1.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "123"})
	require.NoError(t, mergedCmd1.Execute())

	// Materialize to apply the first PR op
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Now the issue is in 'merged' status with PR 123. Call merged again with a different PR.
	// The fix requires this to record the new PR, not skip it.
	mergedCmd2 := newRootCmd()
	mergedCmd2.SetOut(new(bytes.Buffer))
	mergedCmd2.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "456"})
	require.NoError(t, mergedCmd2.Execute())

	// Materialize to apply the new PR op
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Load the issue and verify the PR field was updated to the new PR
	stateDir := getTestStateDir(t, repo)
	issue, err := materialize.LoadIssue(filepath.Join(stateDir, "issues", "task-01.json"))
	require.NoError(t, err)

	assert.Equal(t, "456", issue.PR, "issue PR field should equal the new PR number from the retry call")
}

// TestMergedSkipsUnboundWorktree tests the P2 bug fix: if a worktree exists on the
// correct branch but has no (or wrong) armature-issue-id binding, the worktree must
// NOT be removed. This prevents arm merged from deleting user-created worktrees
// that happen to match the branch name.
//
// Scenario: user manually runs `git worktree add /tmp/foo -b task/task-01` for their
// own purposes (not via `arm claim`). Then `arm merged --issue task-01` must:
// 1. Find the worktree by branch name
// 2. Check the binding in <gitDir>/armature-issue-id
// 3. Skip removal and log a warning if binding is missing or wrong
// 4. Still mark the issue as merged (don't fail on binding mismatch)
//
// The test:
// 1. Set up repo with task-01
// 2. Manually create a git worktree on task/task-01 WITHOUT running `arm claim`
// 3. Transition task to done
// 4. Run `arm merged --issue task-01`
// 5. Assert command succeeds (issue marked merged)
// 6. Assert worktree still exists (was not removed)
// 7. Assert warning was logged about the unbound worktree
func TestMergedSkipsUnboundWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)
	unboundWorktreePath := filepath.Join(t.TempDir(), "unbound-worktree")

	// Manually create a git worktree on the task branch WITHOUT running `arm claim`.
	// This simulates a user worktree that happens to match the expected branch name.
	run(t, repo, "git", "worktree", "add", unboundWorktreePath, "-b", "task/task-01")

	// Verify the worktree exists
	assert.DirExists(t, unboundWorktreePath, "manually-created worktree should exist")

	// Verify there is NO armature-issue-id binding file in the git dir.
	// This is the key difference: a real claimed worktree would have this file.
	gitDir, err := worktree.ResolveGitDir(unboundWorktreePath)
	require.NoError(t, err)
	bindingPath := filepath.Join(gitDir, "armature-issue-id")
	assert.NoFileExists(t, bindingPath, "unbound worktree should have no armature-issue-id file")

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json and issues/task-01.json exist before calling merged.
	_, errMat6 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat6)

	// Call merged command and capture stderr to check for warning
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()

	// Command must succeed (issue is marked merged)
	require.NoError(t, err, "merged should succeed even with unbound worktree")

	// The unbound worktree must NOT be removed
	assert.DirExists(t, unboundWorktreePath, "unbound worktree should NOT be removed (P2 bug fix)")

	// A warning should be logged to stderr about the binding mismatch
	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "Warning:", "should warn about unbound worktree")
	assert.Contains(t, errOutput, "task-01", "warning should mention the issue ID")
	assert.Contains(t, errOutput, "not bound", "warning should mention binding mismatch")
}

// TestMergedRemovesBoundWorktree verifies that if a worktree IS properly bound
// (has the correct armature-issue-id file), it will be removed during merged.
// This ensures the binding check doesn't break the normal flow of removing
// properly claimed worktrees.
func TestMergedRemovesBoundWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a properly bound worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists and IS bound
	assert.DirExists(t, worktreePath, "claimed worktree should exist")
	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	bindingPath := filepath.Join(gitDir, "armature-issue-id")
	assert.FileExists(t, bindingPath, "claimed worktree should have armature-issue-id file")
	bindingBytes, err := os.ReadFile(bindingPath)
	require.NoError(t, err)
	binding := strings.TrimSpace(string(bindingBytes))
	assert.Equal(t, "task-01", binding, "binding should be task-01")

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, errMat7 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat7)

	// Call merged command
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()

	// Command must succeed
	require.NoError(t, err, "merged should succeed for bound worktree")

	// The bound worktree MUST be removed
	assert.NoDirExists(t, worktreePath, "bound worktree should be removed normally")

	// No warning should be logged for a properly bound worktree
	errOutput := errBuf.String()
	assert.NotContains(t, errOutput, "not bound", "should not warn for properly bound worktree")
}

// TestMergedAllowsRetryAfterWorktreeRemovalFails tests the P2 bug fix in dual-branch mode:
// If removeWorktreeForIssue fails after the merge op is recorded, the issue's status
// becomes 'merged'. On retry, the dual-branch guard (the `else` branch in merged.go)
// must accept 'merged' status (in addition to 'done'), and the command should skip
// re-recording the op and just attempt worktree cleanup. This test runs in dual-branch
// mode so that it actually exercises the dual-branch guard at merged.go:141 rather than
// the single-branch guard.
func TestMergedAllowsRetryAfterWorktreeRemovalFails(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap in dual-branch mode so the dual-branch guard (else branch) is exercised.
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(new(bytes.Buffer))
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	workerCmd := newRootCmd()
	workerCmd.SetOut(new(bytes.Buffer))
	workerCmd.SetArgs([]string{"worker-init", "--repo", repo})
	require.NoError(t, workerCmd.Execute())

	createCmd := newRootCmd()
	createCmd.SetOut(new(bytes.Buffer))
	createCmd.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"}))
	require.NoError(t, createCmd.Execute())

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// First merged call: records the op and removes the worktree (happy path).
	// In dual-branch mode the status check goes through the else branch at merged.go:141.
	mergedCmd1 := newRootCmd()
	mergedCmd1.SetOut(new(bytes.Buffer))
	mergedCmd1.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "42"})
	require.NoError(t, mergedCmd1.Execute())
	assert.NoDirExists(t, worktreePath, "worktree should be removed after first merged call")

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Second merged call: status is now 'merged'. Before the fix, the dual-branch guard
	// (entry.Status != ops.StatusDone) would reject this with "requires status=done".
	// After the fix, the guard accepts status=merged and skips re-recording the op,
	// then just attempts worktree cleanup (which is a no-op since it's already gone).
	mergedCmd2 := newRootCmd()
	outBuf2 := new(bytes.Buffer)
	mergedCmd2.SetOut(outBuf2)
	mergedCmd2.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "42"})
	err = mergedCmd2.Execute()
	require.NoError(t, err, "merged must succeed on retry when status is already merged in dual-branch mode (P2 bug fix)")
}

// TestMergedFailsOnViolations_REQ_HOOKBIND_T4 verifies that arm merged --issue exits
// non-zero when the worktree's armature-hook.log contains violation: entries,
// and does NOT tear down the worktree when violations are present.
func TestMergedFailsOnViolations_REQ_HOOKBIND_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Create armature-hook.log with violation entries
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	hookLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	hookLogContent := "2026-07-04T00:00:00Z violation: unbound file write to main.go\n2026-07-04T00:00:01Z violation: unbound file write to cmd/main.go\n"
	err = os.WriteFile(hookLogPath, []byte(hookLogContent), 0o600) //nolint:gosec // test path under temp directory
	require.NoError(t, err)

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command — should fail because violations are present
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.Error(t, err, "merged should exit non-zero when violations are present")

	// Verify worktree still exists (should NOT be removed when violations are found)
	assert.DirExists(t, worktreePath, "worktree should NOT be removed when violations are present")

	// Verify error message mentions violations
	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "violation", "error should mention violations")
}

// TestMergedForceOverridesViolations_REQ_HOOKBIND_T4 verifies that with --force flag,
// arm merged succeeds despite violation: entries in the hook log and proceeds to
// tear down the worktree.
func TestMergedForceOverridesViolations_REQ_HOOKBIND_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Create armature-hook.log with violation entries
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	hookLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	hookLogContent := "2026-07-04T00:00:00Z violation: unbound file write to main.go\n2026-07-04T00:00:01Z violation: unbound file write to cmd/main.go\n"
	err = os.WriteFile(hookLogPath, []byte(hookLogContent), 0o600) //nolint:gosec // test path under temp directory
	require.NoError(t, err)

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command with --force — should succeed despite violations
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--force"})
	err = mergedCmd.Execute()
	require.NoError(t, err, "merged with --force should succeed despite violations")

	// Verify worktree IS removed (when --force is used, violations are overridden)
	assert.NoDirExists(t, worktreePath, "worktree should be removed when --force is used despite violations")
}

// TestMergedWarnsOnPassThrough_REQ_HOOKBIND_T4 verifies that pass-through: entries
// produce warnings only and do NOT cause merged to fail.
func TestMergedWarnsOnPassThrough_REQ_HOOKBIND_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Create armature-hook.log with ONLY pass-through entries (no violations)
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	hookLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	hookLogContent := "2026-07-04T00:00:00Z pass-through: no task binding found\n2026-07-04T00:00:01Z pass-through: stale binding\n"
	err = os.WriteFile(hookLogPath, []byte(hookLogContent), 0o600) //nolint:gosec // test path under temp directory
	require.NoError(t, err)

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	// Materialize so index.json reflects the done→merged transition before calling merged.
	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	// Call merged command — should succeed with pass-through entries (they are warnings only)
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.NoError(t, err, "merged should succeed with pass-through entries (warnings only)")

	// Verify worktree IS removed (pass-through entries do not block removal)
	assert.NoDirExists(t, worktreePath, "worktree should be removed even with pass-through entries")

	// Verify warning is printed to stderr
	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "pass-through", "should warn about pass-through entries in stderr")
}

// TestHookViolationBlocksMerged_EndToEnd_REQ_HOOKBIND_T4 verifies the full
// enforcement loop (finding 1): the harness hook itself (not a hand-written
// fixture) logs a violation for an unbound file write inside the issue's
// worktree, and `arm merged --issue` subsequently blocks on that violation.
func TestHookViolationBlocksMerged_EndToEnd_REQ_HOOKBIND_T4(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create an UNBOUND worktree on the issue's branch (simulating a worker
	// dispatched without `claim --worktree`, so no armature-issue-id exists).
	worktreePath := filepath.Join(t.TempDir(), "unbound-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "-b", "task/task-01")

	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(gitDir, "armature-issue-id"))

	// Drive the hook with a file-write event targeting a file inside the
	// unbound worktree. Path-based resolution finds the worktree's git dir,
	// sees no binding, and must log a violation THERE.
	t.Setenv("ARMATURE_ISSUE_ID", "")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	targetPath := filepath.Join(worktreePath, "internal", "somefile.go")
	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(fmt.Sprintf(
		`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"%s"}]}}`, targetPath)))
	hookCmd.SetOut(new(bytes.Buffer))
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})
	require.NoError(t, hookCmd.Execute(), "hook must fail open (exit 0) on unbound write")

	logData, err := os.ReadFile(filepath.Join(gitDir, "armature-hook.log"))
	require.NoError(t, err, "hook must write the violation into the worktree's own git dir")
	assert.Contains(t, string(logData), "violation:")

	// Transition to done and materialize, then verify the merged gate fires.
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd := newRootCmd()
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.Error(t, err, "merged must block on hook-written violations")
	assert.Contains(t, err.Error(), "violations")

	// Worktree preserved as evidence.
	assert.DirExists(t, worktreePath)
}

// TestMergedClearsStaleParentBranchMetadata_REQ_LNGHZN_S4 verifies the P2 fix: `arm
// merged` must clear the persisted branch.<name>.armature-parent git-config
// key (and base-commit file) when it removes a task's worktree. Without this,
// if the branch is later deleted and the same branch name is reused for a
// genuinely different parent, writeParentBranchConfigIfAbsent's "if absent"
// guard sees the stale leftover value and never records the fresh, correct
// parent — silently corrupting the delivery gate's merge-base computation
// for whatever unrelated branch reuses that name.
func TestMergedClearsStaleParentBranchMetadata_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithTask(t)

	defaultBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Sanity check: claiming from the default branch should have recorded it
	// as the parent-branch config for task/task-01.
	out := runGitOutput(t, repo, "config", "--get", "branch.task/task-01.armature-parent")
	require.Equal(t, defaultBranch, strings.TrimSpace(out))

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	require.NoError(t, mergedCmd.Execute())

	// The stale parent-branch config must be cleared after merged.
	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	_, getErr := getCmd.Output()
	assert.Error(t, getErr, "parent-branch config should be unset after arm merged")

	// A fresh write for the same branch name (simulating branch deletion and
	// reuse with a genuinely different parent) must now succeed, proving the
	// idempotency guard isn't blocked by a stale leftover value.
	gitClient := adapters.New(repo)
	require.NoError(t, writeParentBranchConfigIfAbsent(gitClient, "task/task-01", "other-parent-branch"))
	out2 := runGitOutput(t, repo, "config", "--get", "branch.task/task-01.armature-parent")
	assert.Equal(t, "other-parent-branch", strings.TrimSpace(out2))
}

// TestMergedClearsParentBranchMetadataKeyedOnClaimedBranch_REQ_LNGHZN_S5_T9
// verifies that worktree teardown clears branch-point provenance keyed on the
// branch the issue was actually CLAIMED under (read from the immutable
// armature-claimed-branch marker), not the branch the worktree happens to be
// parked on at removal time. A worktree checked out onto a scratch branch
// before `arm merged` must still leave no stale branch.task/task-01.armature-parent
// entry behind — the previous behaviour keyed off the current worktree branch
// (the scratch branch) and cleared the wrong key.
func TestMergedClearsParentBranchMetadataKeyedOnClaimedBranch_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	defaultBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Claiming from the default branch records it as the parent for task/task-01.
	out := runGitOutput(t, repo, "config", "--get", "branch.task/task-01.armature-parent")
	require.Equal(t, defaultBranch, strings.TrimSpace(out))

	// Park the worktree on an unrelated scratch branch. The current branch is
	// now NOT the branch the issue was claimed under.
	run(t, worktreePath, "git", "checkout", "-b", "scratch/parked")

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	require.NoError(t, mergedCmd.Execute())

	// The parent-branch config keyed on the CLAIMED branch (task/task-01) must
	// be cleared even though the worktree was parked on scratch/parked.
	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	_, getErr := getCmd.Output()
	assert.Error(t, getErr, "parent-branch config for the claimed branch should be unset after arm merged")
}
