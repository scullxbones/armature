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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryRecordedOnDone_REQ_LNGHZN_S11_T1(t *testing.T) {
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	run(t, repo, "git", "branch", "-M", "main")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "deliv-01", "--title", "Delivery snapshot", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "deliv-01")
	_, err = runTrls(t, repo, "claim", "deliv-01", "--worktree")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "feat(deliv-01): add foo")
	tip := strings.TrimSpace(runGitOutput(t, wt, "rev-parse", "HEAD"))

	_, err = runTrls(t, wt, "transition", "--issue", "deliv-01", "--to", "done",
		"--outcome", testIntroductionOutcome, "--force")
	require.NoError(t, err)

	ref := strings.TrimSpace(runGitOutput(t, wt, "rev-parse", "refs/armature/deliveries/deliv-01"))
	assert.Equal(t, tip, ref, "delivery ref must point at the worktree tip")

	opsFor := transitionOpsForIssue(t, repo, "deliv-01")
	require.NotEmpty(t, opsFor)
	last := opsFor[len(opsFor)-1]
	assert.Equal(t, "done", last.Payload.To)
	assert.Equal(t, "task/deliv-01", last.Payload.Branch)
	assert.Equal(t, tip, last.Payload.Tip)
	assert.NotEmpty(t, last.Payload.Base)
	assert.NotEqual(t, last.Payload.Base, last.Payload.Tip)
	assert.Equal(t, "main", last.Payload.IntegrationBranch)

	out, err := runTrls(t, repo, "show", "--format", "json", "deliv-01")
	require.NoError(t, err)
	shown := decodeShowIssue(t, out)
	assert.Equal(t, "task/deliv-01", shown["branch"])
	assert.Equal(t, tip, shown["tip"])
	assert.Equal(t, last.Payload.Base, shown["base"])
}

func TestDoneWithoutWorktreeNamesDeliveryRecord_REQ_LNGHZN_S11_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--format", "agent", "--outcome", testIntroductionOutcome)
	assert.Equal(t, 1, code)

	cf := assertAgentFailureEnvelope(t, stdout.String())
	assert.Equal(t, "TRANSITION-1", cf.Code)
	require.NotEmpty(t, cf.NextActions)
	assert.Contains(t, cf.NextActions[0], "arm delivery record --issue task-01")
	assert.Contains(t, cf.NextActions[0], "--base <sha>")
	assert.Contains(t, cf.NextActions[0], "--tip <sha>")
}

func TestDeliveryRecordRejectsMissingObjects_REQ_LNGHZN_S11_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"delivery", "record", "--repo", repo, "--issue", "task-01",
		"--base", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		"--tip", "cafebabecafebabecafebabecafebabecafebabe",
		"--format", "agent")
	assert.Equal(t, 1, code)

	cf := assertAgentFailureEnvelope(t, stdout.String())
	assert.Equal(t, "DELIVERY-1", cf.Code)
	joined := strings.Join(cf.NextActions, "\n")
	assert.NotContains(t, joined, "arm delivery record")

	out, err := runTrls(t, repo, "show", "--field", "status", "task-01")
	require.NoError(t, err)
	assert.Equal(t, "open", strings.TrimSpace(out), "missing objects must not mark done")
}

func TestTransitionDoneRejectsUnknownIssue_REQ_LNGHZN_S11_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	base := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "extra.txt"), []byte("x\n"), 0o644))
	run(t, repo, "git", "add", "extra.txt")
	run(t, repo, "git", "commit", "-m", "feat: tip")
	tip := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"transition", "--repo", repo, "--issue", "missing-issue", "--to", "done",
		"--base", base, "--tip", tip, "--outcome", "oops", "--force", "--skip-delivery-gate",
		"--format", "agent")
	assert.Equal(t, 1, code, "unknown issue must fail closed before writing delivery/transition")
	assert.Contains(t, stdout.String(), "not found")

	_, err := runTrls(t, repo, "show", "missing-issue")
	require.Error(t, err)
	_, refErr := adapters.New(repo).ResolveRevision("refs/armature/deliveries/missing-issue")
	require.Error(t, refErr, "must not write a delivery ref for an unknown issue")
}

func TestExtractFieldsDeliveryAndDerived_REQ_LNGHZN_S11_T1(t *testing.T) {
	issue := &materialize.Issue{
		ID:                 "TASK-01",
		Branch:             "task/TASK-01",
		Base:               "aaa",
		Tip:                "bbb",
		PR:                 "7",
		RollupStatusBefore: "open",
	}
	got := extractFieldsFromIssue(issue, "branch,base,tip,pr,derived")
	assert.Equal(t, []string{"task/TASK-01", "aaa", "bbb", "7", "true"}, got)
}
