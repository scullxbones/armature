package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLandPR_SquashApplyThenNoOp_REQ_481(t *testing.T) {
	t.Parallel()
	fx := setupLandFixture(t)
	repo := fx.Dir

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	wt := filepath.Join(repo, ".worktrees", "task-01")
	scopeFile := filepath.Join(wt, "cmd", "armature", "task_01.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(scopeFile), 0o755))
	require.NoError(t, os.WriteFile(scopeFile, []byte("package armature\n"), 0o644))
	run(t, wt, "git", "add", "cmd/armature/task_01.go")
	run(t, wt, "git", "commit", "-m", "feat(task-01): implement")

	_, err = runTrls(t, wt, "transition", "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.NotEmpty(t, issue.Base)
	require.NotEmpty(t, issue.Tip)
	tip := issue.Tip
	attestDeliveryForTest(t, repo, "task-01", issue.Base, tip)

	run(t, repo, "git", "checkout", "main")
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "merge", "--squash", tip)
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", "feat(task-01): squash (#99)")
	squashSHA := gitRevParse(t, repo)

	// R1: publish tip as refs/pull/N/head (land fetches this after squash).
	run(t, repo, "git", "push", "origin", tip+":refs/pull/99/head")
	run(t, repo, "git", "worktree", "remove", "--force", wt)
	run(t, repo, "git", "branch", "-D", "task/task-01")

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"land", "--repo", repo, "--pr", "99", "--format", "agent")
	assert.Equal(t, 0, code, "out=%s", stdout.String())
	_, rows := decodeSyncIssues(t, stdout.String())
	require.Len(t, rows, 1)
	assert.Equal(t, "promote", rows[0].Kind)
	assert.Equal(t, "merged", rows[0].Status)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	status, err := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, err)
	assert.Equal(t, "merged\n", status)
	matched, err := runTrls(t, repo, "show", "task-01", "--field", "matched_commit")
	require.NoError(t, err)
	assert.Equal(t, squashSHA+"\n", matched, "matched_commit must be the squash SHA")

	opsBefore := countMergedTransitions(t, repo, "task-01")
	require.GreaterOrEqual(t, opsBefore, 1)

	stdout2 := new(bytes.Buffer)
	code = executeThenHandleRootError(t, stdout2, new(bytes.Buffer),
		"land", "--repo", repo, "--pr", "99", "--format", "agent")
	assert.Equal(t, 0, code, "second land must be a no-op; out=%s", stdout2.String())
	assert.Equal(t, opsBefore, countMergedTransitions(t, repo, "task-01"),
		"second land must append no merged transition")
}

func TestLandPR_DryRunWritesNothing_REQ_481(t *testing.T) {
	t.Parallel()
	fx := setupLandFixture(t)
	repo := fx.Dir

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	wt := filepath.Join(repo, ".worktrees", "task-01")
	scopeFile := filepath.Join(wt, "cmd", "armature", "task_01.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(scopeFile), 0o755))
	require.NoError(t, os.WriteFile(scopeFile, []byte("package armature\n"), 0o644))
	run(t, wt, "git", "add", "cmd/armature/task_01.go")
	run(t, wt, "git", "commit", "-m", "feat(task-01): implement")
	_, err = runTrls(t, wt, "transition", "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	attestDeliveryForTest(t, repo, "task-01", issue.Base, issue.Tip)
	run(t, repo, "git", "push", "origin", issue.Tip+":refs/pull/7/head")

	opsBefore := countAllOps(t, repo)
	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"land", "--repo", repo, "--pr", "7", "--dry-run", "--format", "agent")
	assert.Equal(t, 0, code, "out=%s", stdout.String())
	_, rows := decodeSyncIssues(t, stdout.String())
	require.Len(t, rows, 1)
	assert.Equal(t, "not-on-target", rows[0].Kind)
	assert.Equal(t, opsBefore, countAllOps(t, repo), "dry-run must write no ops")

	status, err := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, err)
	assert.Equal(t, "done\n", status)
}

func TestLandPR_RequiresPR_REQ_481(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "land")
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "required flag") || strings.Contains(err.Error(), "--pr"),
		"got %v", err)
}

func setupLandFixture(t *testing.T) gittest.Fixture {
	t.Helper()
	require.NotEmpty(t, fixtures.taskFixture.Dir, "TestMain must prepare task fixture")
	fx := gittest.CloneFixture(t, fixtures.taskFixture)
	require.NoError(t, uniquifyClonedWorker(fx.Dir))
	_, err := runTrls(t, fx.Dir, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, fx.Dir, "amend", "--issue", "task-01", "--acceptance", testAcceptance)
	require.NoError(t, err)
	return fx
}

func countMergedTransitions(t *testing.T, repo, issueID string) int {
	t.Helper()
	n := 0
	for _, op := range transitionOpsForIssue(t, repo, issueID) {
		if op.Payload.To == ops.StatusMerged {
			n++
		}
	}
	return n
}

func countAllOps(t *testing.T, repo string) int {
	t.Helper()
	ctx := getTestContext(t, repo)
	all, err := readAllOpsFromDir(filepath.Join(ctx.IssuesDir, "ops"))
	require.NoError(t, err)
	return len(all)
}

func TestFetchPullHead_R1_REQ_481(t *testing.T) {
	t.Parallel()
	fx := gittest.InitWithOrigin(t)
	run(t, fx.Dir, "git", "commit", "--allow-empty", "-m", "init")
	run(t, fx.Dir, "git", "checkout", "-b", "delivery")
	run(t, fx.Dir, "git", "commit", "--allow-empty", "-m", "feat: tip")
	tip := gitRevParse(t, fx.Dir)
	run(t, fx.Dir, "git", "push", "origin", "HEAD:refs/pull/42/head")
	run(t, fx.Dir, "git", "checkout", "main")

	// Fresh clone has heads only — must fetch refs/pull/42/head (R1).
	fresh := t.TempDir()
	run(t, "", "git", "clone", fx.Origin, fresh)
	git := adapters.New(fresh)
	got, err := git.FetchPullHead("42")
	require.NoError(t, err)
	assert.Equal(t, tip, got)
}

func TestLandPR_LeavesOriginURLTokenFree_REQ_481(t *testing.T) {
	// GITHUB_TOKEN must auth via ephemeral networkCmd header, never remote URL.
	probe := "ghs_" + "LAND_ORIGIN_PROBE_481"
	t.Setenv("GITHUB_TOKEN", probe)

	fx := setupLandFixture(t)
	repo := fx.Dir
	beforeURL := originRemoteURL(t, repo)
	require.NotContains(t, beforeURL, probe)
	require.NotContains(t, beforeURL, "x-access-token")

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	wt := filepath.Join(repo, ".worktrees", "task-01")
	scopeFile := filepath.Join(wt, "cmd", "armature", "task_01.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(scopeFile), 0o755))
	require.NoError(t, os.WriteFile(scopeFile, []byte("package armature\n"), 0o644))
	run(t, wt, "git", "add", "cmd/armature/task_01.go")
	run(t, wt, "git", "commit", "-m", "feat(task-01): implement")
	_, err = runTrls(t, wt, "transition", "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	attestDeliveryForTest(t, repo, "task-01", issue.Base, issue.Tip)

	run(t, repo, "git", "checkout", "main")
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "merge", "--squash", issue.Tip)
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", "feat(task-01): squash (#88)")
	run(t, repo, "git", "push", "origin", issue.Tip+":refs/pull/88/head")
	run(t, repo, "git", "worktree", "remove", "--force", wt)
	run(t, repo, "git", "branch", "-D", "task/task-01")

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"land", "--repo", repo, "--pr", "88", "--format", "agent")
	assert.Equal(t, 0, code, "out=%s", stdout.String())

	afterURL := originRemoteURL(t, repo)
	assert.Equal(t, beforeURL, afterURL, "arm land must not rewrite remote.origin.url")
	assert.NotContains(t, afterURL, probe)
	assert.NotContains(t, afterURL, "x-access-token")
}

func originRemoteURL(t *testing.T, repo string) string {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(repo, "config", "--get", "remote.origin.url")
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
