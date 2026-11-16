package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/scullxbones/armature/internal/externalref"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIssueCreates struct {
	mu    sync.Mutex
	calls []createdCall
}

type createdCall struct {
	owner, repo, title, body string
	labels                   []string
}

func (f *fakeIssueCreates) create(_ context.Context, owner, repo, title, body string, labels []string) (externalref.CreatedIssue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.calls) + 1
	f.calls = append(f.calls, createdCall{
		owner: owner, repo: repo, title: title, body: body,
		labels: append([]string(nil), labels...),
	})
	num := 9000 + n
	return externalref.CreatedIssue{
		Number: num,
		URL:    fmt.Sprintf("https://github.com/scullxbones/armature/issues/%d", num),
	}, nil
}

func runLandWithAndon(t *testing.T, stdout, stderr *bytes.Buffer, create issueCreator, args ...string) int {
	t.Helper()
	return Run(context.Background(), Invocation{
		Args:             args,
		Env:              OverlayEnv(OSEnv(), cliEnvFor(t)),
		Stdout:           stdout,
		Stderr:           stderr,
		TempDir:          testTempDir(t),
		createAndonIssue: create,
	})
}

func TestJidokaDriftTreeStopOneAndon_REQ_482(t *testing.T) {
	t.Parallel()
	fx := setupLandFixture(t)
	repo := fx.Dir

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "story-01", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{
			Title: "story", NodeType: "story", DefinitionOfDone: "story done",
			Acceptance: []byte(testAcceptance), Confidence: "verified",
		},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpReparent, TargetID: "task-01", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{Parent: "story-01"},
	}))
	plantVerifiedTask(t, repo, "unrelated-01", "other.go")
	_, err = runTrls(t, repo, "amend", "--issue", "unrelated-01", "--acceptance", testAcceptance, "--dod", "unrelated done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
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
	require.Equal(t, "story-01", issue.Parent)

	run(t, repo, "git", "checkout", "main")
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "merge", "--squash", issue.Tip)
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", "feat(task-01): squash (#77)")
	run(t, repo, "git", "push", "origin", issue.Tip+":refs/pull/77/head")
	run(t, repo, "git", "worktree", "remove", "--force", wt)
	run(t, repo, "git", "branch", "-D", "task/task-01")

	fake := &fakeIssueCreates{}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := runLandWithAndon(t, stdout, stderr, fake.create,
		"land", "--repo", repo, "--pr", "77", "--format", "agent")
	assert.Equal(t, 1, code, "drift land must fail; out=%s err=%s", stdout.String(), stderr.String())
	assert.Contains(t, stderr.String(), "jidoka: armed stop")

	fake.mu.Lock()
	require.Len(t, fake.calls, 1, "exactly one GitHub issue")
	assert.Contains(t, fake.calls[0].title, "andon:")
	assert.Contains(t, fake.calls[0].title, "77")
	assert.Contains(t, fake.calls[0].title, "task-01")
	assert.Contains(t, fake.calls[0].body, "Failing step:")
	assert.Contains(t, fake.calls[0].body, "land:missing-assessment")
	firstAndon := fake.calls[0].title
	fake.mu.Unlock()

	stops, err := ops.ReadAllPromotionStops(filepath.Join(ctx.IssuesDir, "ops"))
	require.NoError(t, err)
	active := promotion.ActiveStops(stops)
	require.Len(t, active, 1)
	assert.Equal(t, "story-01", active[0].TreeRoot)

	plantCIGateEvidence(t, repo, issue.Tip)
	_, err = runTrls(t, repo, "promotion-ready",
		"--issue", "task-01", "--base", issue.Base, "--head", issue.Tip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jidoka stop")

	g := promotionGraphFromIssues(map[string]*materialize.Issue{
		"story-01":     {ID: "story-01", Children: []string{"task-01"}},
		"task-01":      {ID: "task-01", Parent: "story-01"},
		"unrelated-01": {ID: "unrelated-01"},
	})
	assert.Nil(t, promotion.StopCovering(active, g, []string{"unrelated-01"}),
		"stop must not cover unrelated tree")

	_, err = runTrls(t, repo, "promotion-stop", "clear", "--issue", "task-01")
	require.Error(t, err, "clear without root-cause note must refuse")
	_, err = runTrls(t, repo, "promotion-stop", "clear",
		"--issue", "task-01", "--msg", "squash landed without attestation")
	require.NoError(t, err)

	stops, err = ops.ReadAllPromotionStops(filepath.Join(ctx.IssuesDir, "ops"))
	require.NoError(t, err)
	assert.Empty(t, promotion.ActiveStops(stops))

	history, err := ops.ReadAllPromotionStops(filepath.Join(ctx.IssuesDir, "ops"))
	require.NoError(t, err)
	plan := promotion.PlanTrip(g, history, "task-01", promotion.KindMissingAssessment)
	assert.True(t, plan.OpenDogfood)
	assert.False(t, plan.OpenAndon)

	stdout2 := new(bytes.Buffer)
	stderr2 := new(bytes.Buffer)
	code = runLandWithAndon(t, stdout2, stderr2, fake.create,
		"land", "--repo", repo, "--pr", "77", "--format", "agent")
	assert.Equal(t, 1, code, "second drift land must still fail; out=%s err=%s", stdout2.String(), stderr2.String())

	fake.mu.Lock()
	require.Len(t, fake.calls, 2, "second call is dogfood finding")
	assert.Equal(t, firstAndon, fake.calls[0].title)
	assert.Contains(t, fake.calls[1].title, "dogfood:")
	assert.Contains(t, fake.calls[1].labels, "dogfood-finding")
	andonTitles := 0
	for _, c := range fake.calls {
		if len(c.title) >= 6 && c.title[:6] == "andon:" {
			andonTitles++
		}
	}
	assert.Equal(t, 1, andonTitles, "exactly one andon issue across both failures")
	fake.mu.Unlock()
}

func TestLandDryRunNeverTripsJidoka_REQ_482(t *testing.T) {
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
	// On-target without assessment → missing-assessment; dry-run must not trip.
	run(t, repo, "git", "checkout", "main")
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "merge", "--squash", issue.Tip)
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", "feat(task-01): squash (#66)")
	run(t, repo, "git", "push", "origin", issue.Tip+":refs/pull/66/head")

	ctx := getTestContext(t, repo)
	opsBefore, err := ops.ReadAllPromotionStops(filepath.Join(ctx.IssuesDir, "ops"))
	require.NoError(t, err)

	fake := &fakeIssueCreates{}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := runLandWithAndon(t, stdout, stderr, fake.create,
		"land", "--repo", repo, "--pr", "66", "--dry-run", "--format", "agent")
	assert.Equal(t, 1, code, "dry-run missing-assessment still exits 1; out=%s err=%s",
		stdout.String(), stderr.String())
	assert.NotContains(t, stderr.String(), "jidoka: armed stop")

	fake.mu.Lock()
	assert.Empty(t, fake.calls, "dry-run must open zero GitHub issues")
	fake.mu.Unlock()

	opsAfter, err := ops.ReadAllPromotionStops(filepath.Join(ctx.IssuesDir, "ops"))
	require.NoError(t, err)
	assert.Equal(t, len(opsBefore), len(opsAfter), "dry-run must append zero promotion-stop ops")
	assert.Empty(t, promotion.ActiveStops(opsAfter))
}
