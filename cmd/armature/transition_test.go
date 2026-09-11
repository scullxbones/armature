package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	repo := initTempRepo(t)
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
	allOps, _, err := readAllOpsFromDirWithOffsets(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	var out []ops.Op
	for _, op := range allOps {
		if op.Type == ops.OpTransition && op.TargetID == issueID {
			out = append(out, op)
		}
	}
	return out
}

// TestIdenticalPayloadIsNoOpExitZero_REQ_AOC_S4_T1 verifies that repeating a
// transition whose payload matches the issue's current recorded state exits 0
// and says it is a no-op rather than an error.
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

// TestChangedPayloadAppendsAmendment_REQ_AOC_S4_T1 verifies that a same-status
// transition with a changed payload appends as an amendment at exit 0.
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

// TestNoOpAppendsNothingToOpsLog_REQ_AOC_S4_T1 verifies that an identical-payload
// retry does not append another op.
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
