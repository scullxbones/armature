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

func TestIssueWorktreeHasViolations_FailsClosedOnUnreadableInventory_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()
	notARepo := t.TempDir()
	issue := materialize.Issue{ID: "task-01", Type: "task"}

	_, err := issueWorktreeHasViolations(notARepo, issue)
	require.Error(t, err, "unreadable inventory must fail closed, not report zero violations")
}

func TestRemoveWorktreeForIssueTracked_FailsClosedOnUnreadableInventory_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()
	notARepo := t.TempDir()
	issue := materialize.Issue{ID: "task-01", Type: "task"}

	outcome, err := removeWorktreeForIssueTracked(notARepo, issue, new(bytes.Buffer))
	require.Error(t, err, "unreadable inventory must surface an error, not a silent skip")
	assert.Equal(t, worktreeSkipped, outcome)
}

func TestIssueWorktreeHasViolations_FailsClosedOnAmbiguousMarkers_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	legacyPath := filepath.Join(t.TempDir(), "legacy-worktree")
	run(t, repo, "git", "worktree", "add", legacyPath, "-b", "legacy/task-01")
	legacyGitDir, err := worktree.ResolveGitDir(legacyPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(legacyGitDir, "armature-issue-id"),
		[]byte("task-01\n"), 0o600))

	issue := materialize.Issue{ID: "task-01", Type: "task"}
	_, gateErr := issueWorktreeHasViolations(repo, issue)
	require.Error(t, gateErr, "ambiguous binding-bound worktrees must fail closed, not report zero violations")
	assert.Contains(t, gateErr.Error(), "ambiguous", "error should name the ambiguity condition")
}

func TestMergedCmd_DoesNotMaterialize(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--skip-delivery-gate", "--outcome", "Completed", "--force")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	stateDir := getTestStateDir(t, repo)
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	stat, statErr := os.Stat(checkpointPath)
	require.NoError(t, statErr, "checkpoint.json should exist after materialize")
	mtimeBefore := stat.ModTime()

	_, err = runTrls(t, repo, "merged", "--issue", "task-01", "--force")
	require.NoError(t, err)

	statAfter, statErr := os.Stat(checkpointPath)
	require.NoError(t, statErr)
	assert.Equal(t, mtimeBefore, statAfter.ModTime(),
		"checkpoint.json must not be updated by arm merged: store.ReadIndex/ReadIssue must be used, not store.Load")
}

func TestMergedRemovesTaskWorktree(t *testing.T) {
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

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	require.NoError(t, mergedCmd.Execute())

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

func TestMergedRemovesBugWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test bug", "--type", "bug", "--id", "bug-01"}))
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(repo, ".worktrees", "bug-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "bug-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{
		"transition", "--repo", repo, "--issue", "bug-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", "Fixed", "--force",
	}))
	require.NoError(t, transitionCmd.Execute())

	_, err2 := runTrls(t, repo, "materialize")
	require.NoError(t, err2)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "bug-01"})
	require.NoError(t, mergedCmd.Execute())

	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

func TestMergedHandlesStoryWithNoActiveWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test story", "--type", "story", "--id", "story-01"}))
	require.NoError(t, cmd2.Execute())

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "story-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Delivered", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "story-01", "--force"})
	require.NoError(t, mergedCmd.Execute())
}

func TestMergedRemovesStoryWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test story", "--type", "story", "--id", "story-01"}))
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(repo, ".worktrees", "story-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "story-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "story-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Delivered", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "story-01"})
	require.NoError(t, mergedCmd.Execute())

	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

func TestMergedRemovesFeatureWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test feature", "--type", "feature", "--id", "feature-01"}))
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(repo, ".worktrees", "feature-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "feature-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "feature-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Shipped", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "feature-01", "--force"})
	require.NoError(t, mergedCmd.Execute())

	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

func TestMergedHandlesFeatureWithNoWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test feature", "--type", "feature", "--id", "feature-01"}))
	require.NoError(t, cmd2.Execute())

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "feature-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Shipped", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "feature-01", "--force"})
	require.NoError(t, mergedCmd.Execute())
}

func TestMergedWarnsOnPassThroughEntries(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

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

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.NoError(t, err)

	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "pass-through", "should warn about pass-through entries in stderr")
	assert.Contains(t, errOutput, "task-01", "warning should mention the issue ID")
}

func TestMergedNoWarningWithoutPassThroughEntries(t *testing.T) {
	repo := setupRepoWithTask(t)

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat2 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat2)

	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.NoError(t, err)

	errOutput := errBuf.String()
	assert.NotContains(t, errOutput, "pass-through", "should not warn about pass-through when none exist")
}

func TestMergedMissingWorktreeFailsClosed_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat3 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat3)

	run(t, repo, "git", "worktree", "remove", "--force", worktreePath)

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

func TestMergedDoesNotWarnWhenWorktreeAlreadyRemoved(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

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

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat4 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat4)

	run(t, repo, "git", "worktree", "remove", "--force", worktreePath)

	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.Error(t, err)

	assert.NotContains(t, errBuf.String(), "pass-through", "no warning expected when worktree is already gone")
}

func TestMergedRejectsNonDoneStatus(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"}))
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "in-progress"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.Error(t, err, "merged should reject in-progress status")
	assert.Contains(t, err.Error(), "status=done", "error message should indicate done status required")

	assert.DirExists(t, worktreePath, "worktree should NOT be removed when merged fails")
}

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

		_, errMat := runTrls(t, repo, "materialize")
		require.NoError(t, errMat)

		mergedCmd := newRootCmd()
		mergedCmd.SetOut(new(bytes.Buffer))
		mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
		require.NoError(t, mergedCmd.Execute())

		assert.NoDirExists(t, worktreePath, "worktree should be removed after successful merged")
	})

	t.Run("failure path: appendOp fails → worktree preserved", func(t *testing.T) {
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

		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)

		opsDir := filepath.Join(repo, ".armature", "ops")
		require.NoError(t, os.Chmod(opsDir, 0o444))
		defer func() {
			if chmodErr := os.Chmod(opsDir, 0o755); chmodErr != nil {
				t.Logf("warning: failed to restore ops dir permissions: %v", chmodErr)
			}
		}()

		mergedCmd := newRootCmd()
		mergedCmd.SetOut(new(bytes.Buffer))
		mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
		err = mergedCmd.Execute()
		require.Error(t, err, "merged should fail when appendOp cannot write")

		assert.DirExists(t, worktreePath, "worktree must NOT be removed when appendOp fails")
	})
}

func TestMergedRecordsPROnRetry(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

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

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd1 := newRootCmd()
	mergedCmd1.SetOut(new(bytes.Buffer))
	mergedCmd1.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "123"})
	require.NoError(t, mergedCmd1.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd2 := newRootCmd()
	mergedCmd2.SetOut(new(bytes.Buffer))
	mergedCmd2.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "456"})
	require.NoError(t, mergedCmd2.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	stateDir := getTestStateDir(t, repo)
	issue, err := materialize.LoadIssue(filepath.Join(stateDir, "issues", "task-01.json"))
	require.NoError(t, err)

	assert.Equal(t, "456", issue.PR, "issue PR field should equal the new PR number from the retry call")
}

func TestMergedSkipsUnboundWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)
	unboundWorktreePath := filepath.Join(t.TempDir(), "unbound-worktree")

	run(t, repo, "git", "worktree", "add", unboundWorktreePath, "-b", "task/task-01")

	assert.DirExists(t, unboundWorktreePath, "manually-created worktree should exist")

	gitDir, err := worktree.ResolveGitDir(unboundWorktreePath)
	require.NoError(t, err)
	bindingPath := filepath.Join(gitDir, "armature-issue-id")
	assert.NoFileExists(t, bindingPath, "unbound worktree should have no armature-issue-id file")

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--skip-delivery-gate",
		"--outcome", "Completed", "--force"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat6 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat6)

	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()

	require.NoError(t, err, "merged should succeed even with unbound worktree")

	assert.DirExists(t, unboundWorktreePath, "unbound worktree should NOT be removed (P2 bug fix)")

	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "Warning:", "should warn about unbound worktree")
	assert.Contains(t, errOutput, "task-01", "warning should mention the issue ID")
	assert.Contains(t, errOutput, "not bound", "warning should mention binding mismatch")
}

func TestMergedRemovesBoundWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	assert.DirExists(t, worktreePath, "claimed worktree should exist")
	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	bindingPath := filepath.Join(gitDir, "armature-issue-id")
	assert.FileExists(t, bindingPath, "claimed worktree should have armature-issue-id file")
	bindingBytes, err := os.ReadFile(bindingPath)
	require.NoError(t, err)
	binding := strings.TrimSpace(string(bindingBytes))
	assert.Equal(t, "task-01", binding, "binding should be task-01")

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat7 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat7)

	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()

	require.NoError(t, err, "merged should succeed for bound worktree")

	assert.NoDirExists(t, worktreePath, "bound worktree should be removed normally")

	errOutput := errBuf.String()
	assert.NotContains(t, errOutput, "not bound", "should not warn for properly bound worktree")
}

func TestMergedAllowsRetryAfterWorktreeRemovalFails(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

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

	mergedCmd1 := newRootCmd()
	mergedCmd1.SetOut(new(bytes.Buffer))
	mergedCmd1.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "42"})
	require.NoError(t, mergedCmd1.Execute())
	assert.NoDirExists(t, worktreePath, "worktree should be removed after first merged call")

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd2 := newRootCmd()
	outBuf2 := new(bytes.Buffer)
	mergedCmd2.SetOut(outBuf2)
	mergedCmd2.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "42"})
	err = mergedCmd2.Execute()
	require.NoError(t, err, "merged must succeed on retry when status is already merged in dual-branch mode (P2 bug fix)")
}

func TestMergedFailsOnViolations_REQ_HOOKBIND_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

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

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.Error(t, err, "merged should exit non-zero when violations are present")

	assert.DirExists(t, worktreePath, "worktree should NOT be removed when violations are present")

	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "violation", "error should mention violations")
}

func TestMergedForceOverridesViolations_REQ_HOOKBIND_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

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

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--force"})
	err = mergedCmd.Execute()
	require.NoError(t, err, "merged with --force should succeed despite violations")

	assert.NoDirExists(t, worktreePath, "worktree should be removed when --force is used despite violations")
}

func TestMergedWarnsOnPassThrough_REQ_HOOKBIND_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

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

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs(enrichTestCLIArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force", "--skip-delivery-gate"}))
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.NoError(t, err, "merged should succeed with pass-through entries (warnings only)")

	assert.NoDirExists(t, worktreePath, "worktree should be removed even with pass-through entries")

	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "pass-through", "should warn about pass-through entries in stderr")
}

func TestHookViolationBlocksMerged_EndToEnd_REQ_HOOKBIND_T4(t *testing.T) {
	repo := setupRepoWithTask(t)

	worktreePath := filepath.Join(t.TempDir(), "unbound-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "-b", "task/task-01")

	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(gitDir, "armature-issue-id"))

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

	assert.DirExists(t, worktreePath)
}

func TestMergedClearsStaleParentBranchMetadata_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithTask(t)

	defaultBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

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

	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	_, getErr := getCmd.Output()
	assert.Error(t, getErr, "parent-branch config should be unset after arm merged")

	gitClient := adapters.New(repo)
	require.NoError(t, writeParentBranchConfigIfAbsent(gitClient, "task/task-01", "other-parent-branch"))
	out2 := runGitOutput(t, repo, "config", "--get", "branch.task/task-01.armature-parent")
	assert.Equal(t, "other-parent-branch", strings.TrimSpace(out2))
}

func TestMergedClearsParentBranchMetadataKeyedOnClaimedBranch_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	defaultBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	out := runGitOutput(t, repo, "config", "--get", "branch.task/task-01.armature-parent")
	require.Equal(t, defaultBranch, strings.TrimSpace(out))

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

	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	_, getErr := getCmd.Output()
	assert.Error(t, getErr, "parent-branch config for the claimed branch should be unset after arm merged")
}
