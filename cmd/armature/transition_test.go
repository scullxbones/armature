package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

const (
	idempotentOutcomeWaiting   = "Waiting on the upstream review to finish"
	idempotentOutcomeRicher    = "Waiting on the upstream review, now with more detail"
	idempotentIssueID          = "aoc-s4-t1-noop"
	idempotentAmendmentIssueID = "aoc-s4-t1-amend"
)

func setupTransitionIdempotencyRepo(t *testing.T, issueID string) string {
	t.Helper()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", issueID, "--title", "Payload-keyed idempotency", "--type", "task")
	require.NoError(t, err)
	return repo
}

func transitionOpsForIssue(t *testing.T, repo, issueID string) []ops.Op {
	t.Helper()
	allOps, err := readAllOpsFromDir(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	var out []ops.Op
	for _, op := range allOps {
		if op.Type == ops.OpTransition && op.TargetID == issueID {
			out = append(out, op)
		}
	}
	return out
}

func TestIdenticalPayloadIsNoOpExitZero_REQ_AOC_S4_T1(t *testing.T) {
	repo := setupTransitionIdempotencyRepo(t, idempotentIssueID)

	out, err := runTrls(t, repo, "transition", "--issue", idempotentIssueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err)
	assert.Contains(t, out, idempotentIssueID)

	out, err = runTrls(t, repo, "transition", "--issue", idempotentIssueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err, "identical payload must exit 0, not error")
	assert.NotContains(t, strings.ToLower(out), `"error"`)
	assert.Contains(t, out, `"noop":true`)

	human, err := runTrls(t, repo, "--format", "human", "transition", "--issue", idempotentIssueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err)
	assert.Contains(t, human, "no-op")
}

func TestChangedPayloadAppendsAmendment_REQ_AOC_S4_T1(t *testing.T) {
	repo := setupTransitionIdempotencyRepo(t, idempotentAmendmentIssueID)

	_, err := runTrls(t, repo, "transition", "--issue", idempotentAmendmentIssueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err)
	require.Len(t, transitionOpsForIssue(t, repo, idempotentAmendmentIssueID), 1)

	out, err := runTrls(t, repo, "transition", "--issue", idempotentAmendmentIssueID, "--to", "blocked", "--outcome", idempotentOutcomeRicher)
	require.NoError(t, err)
	assert.Contains(t, out, `"amendment":true`)

	got := transitionOpsForIssue(t, repo, idempotentAmendmentIssueID)
	require.Len(t, got, 2, "changed payload must append a second transition op")
	assert.Equal(t, "blocked", got[1].Payload.To)
	assert.Equal(t, idempotentOutcomeRicher, got[1].Payload.Outcome)
}

func TestNoOpAppendsNothingToOpsLog_REQ_AOC_S4_T1(t *testing.T) {
	repo := setupTransitionIdempotencyRepo(t, idempotentIssueID)

	_, err := runTrls(t, repo, "transition", "--issue", idempotentIssueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err)
	before := transitionOpsForIssue(t, repo, idempotentIssueID)
	require.Len(t, before, 1)

	_, err = runTrls(t, repo, "transition", "--issue", idempotentIssueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err)

	after := transitionOpsForIssue(t, repo, idempotentIssueID)
	assert.Equal(t, before, after, "no-op must leave the ops log unchanged")
	assert.Len(t, after, 1)
}

func runTransitionUnlocked(t *testing.T, repo string, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs(append(injectDoneDelivery(repo, enrichTestCLIArgs(args)), "--repo", repo))
	err := root.Execute()
	return buf.String(), err
}

func TestIdenticalDoneRetryOnMainIsNoOp_REQ_AOC_S4_T1(t *testing.T) {
	issueID := "aoc-s4-t1-done-main"
	repo := setupTransitionIdempotencyRepo(t, issueID)
	run(t, repo, "git", "branch", "-M", "main")

	_, err := runTrls(t, repo, "transition", "--issue", issueID, "--to", "done",
		"--skip-delivery-gate", "--outcome", idempotentOutcomeWaiting)
	require.Error(t, err, "first-time done on main without --force must still fail")
	assert.Contains(t, err.Error(), "cannot transition to done")
	require.Empty(t, transitionOpsForIssue(t, repo, issueID))

	_, err = runTrls(t, repo, "transition", "--issue", issueID, "--to", "done",
		"--force", "--skip-delivery-gate", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err)
	require.Len(t, transitionOpsForIssue(t, repo, issueID), 1)

	out, err := runTrls(t, repo, "transition", "--issue", issueID, "--to", "done",
		"--skip-delivery-gate", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err, "identical done retry on main must exit 0")
	assert.Contains(t, out, `"noop":true`)
	assert.Len(t, transitionOpsForIssue(t, repo, issueID), 1)
}

func TestOverlappingIdenticalTransitionsAppendOnce_REQ_AOC_S4_T1(t *testing.T) {
	issueID := "aoc-s4-t1-race"
	repo := setupTransitionIdempotencyRepo(t, issueID)

	var barrier sync.WaitGroup
	barrier.Add(2)
	testBarrierAfterIdempotencyCheck = func() {
		barrier.Done()
		barrier.Wait()
	}
	t.Cleanup(func() { testBarrierAfterIdempotencyCheck = nil })

	args := []string{"transition", "--issue", issueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() {
			_, errs[i] = runTransitionUnlocked(t, repo, args...)
		})
	}
	wg.Wait()
	ok, collision := 0, 0
	for i, err := range errs {
		if err == nil {
			ok++
			continue
		}
		if strings.Contains(err.Error(), "LOG-SLOT-COLLISION") {
			collision++
			continue
		}
		require.NoError(t, err, "overlapping transition %d: unexpected error", i)
	}
	assert.Equal(t, 1, ok, "exactly one overlapping transition must append")
	assert.Equal(t, 1, collision, "the other writer must fail LOG-SLOT-COLLISION (common-dir flock)")
	assert.Len(t, transitionOpsForIssue(t, repo, issueID), 1, "overlapping identical retries must append once")
}

func TestDoneAmendmentPreservesIntegrationBranch_REQ_LNGHZN_S11_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	wt := filepath.Join(repo, ".worktrees", "task-01")
	_, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	scoped := filepath.Join(wt, "cmd/armature/task_01.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(scoped), 0o755))
	require.NoError(t, os.WriteFile(scoped, []byte("package main\n"), 0o644))
	run(t, wt, "git", "add", "cmd/armature/task_01.go")
	run(t, wt, "git", "commit", "-m", "feat(task-01): add scoped file")

	_, err = runTrls(t, wt, "transition", "--issue", "task-01", "--to", "done",
		"--outcome", "first delivery", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	before, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, "main", before.IntegrationBranch)
	require.NotEmpty(t, before.Base)
	require.NotEmpty(t, before.Tip)

	cfgPath := filepath.Join(getTestContext(t, repo).IssuesDir, "config.json")
	cfg, err := config.LoadConfig(cfgPath)
	require.NoError(t, err)
	cfg.IntegrationBranch = "develop"
	require.NoError(t, config.WriteConfig(cfgPath, cfg))

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done",
		"--outcome", "amended outcome", "--force", "--skip-delivery-gate")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	after, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, "main", after.IntegrationBranch,
		"same-status done amendment must keep the recorded integration branch")
	assert.Equal(t, before.Base, after.Base)
	assert.Equal(t, before.Tip, after.Tip)

	opsFor := transitionOpsForIssue(t, repo, "task-01")
	require.GreaterOrEqual(t, len(opsFor), 2)
	last := opsFor[len(opsFor)-1]
	assert.Equal(t, "main", last.Payload.IntegrationBranch)
}
