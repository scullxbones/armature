package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func agentFailureFromStdout(t *testing.T, stdout string) *armerrors.CommandFailure {
	t.Helper()
	raw := strings.TrimSpace(stdout)
	require.True(t, json.Valid([]byte(raw)), "stdout must be one JSON object, got %q", stdout)
	var envelope commandFailureEnvelope
	require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
	require.NotNil(t, envelope.Error, "stdout must be {error:{...}}, got %s", raw)
	assert.NotEqual(t, armerrors.CodeGeneral1, envelope.Error.Code, "high-traffic paths must not wrap as GENERAL-1")
	return envelope.Error
}

func TestClaimErrorsCarryNextActions_REQ_LNGHZN_S6_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, stderr,
		"claim", "--repo", repo, "--issue", "task-01", "--format", "agent")
	assert.Equal(t, 2, code)
	assert.Empty(t, stderr.String(), "Command Failure must not land on stderr")

	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, armerrors.CodeUSAGE, cf.Code)
	assert.Contains(t, cf.Cause, "worktree")
	require.NotEmpty(t, cf.NextActions, "claim errors must carry next_actions")
	assert.Contains(t, strings.Join(cf.NextActions, "\n"), "arm claim")

	missing := new(bytes.Buffer)
	code = executeThenHandleRootError(t, missing, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "no-such-issue", "--worktree", "--format", "agent")
	assert.Equal(t, 1, code)
	notFound := agentFailureFromStdout(t, missing.String())
	assert.Equal(t, "CLAIM-1", notFound.Code)
	assert.Contains(t, notFound.Cause, "not found")
	require.NotEmpty(t, notFound.NextActions)
	joined := strings.Join(notFound.NextActions, "\n")
	assert.True(t, strings.Contains(joined, "arm ready") || strings.Contains(joined, "arm list"),
		"next_actions=%v", notFound.NextActions)
}

func TestReviewBundleErrorRemediation_REQ_LNGHZN_S6_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"review", "record",
		"--repo", repo,
		"--issue", "task-01",
		"--assessment", "assessment.json",
		"--bundle", `{"bundle_id":"not-a-path"}`,
		"--format", "agent")
	assert.Equal(t, 1, code)

	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "REVIEW-1", cf.Code)
	assert.Contains(t, strings.ToLower(cf.Cause), "bundle")
	require.NotEmpty(t, cf.NextActions, "bundle misuse must carry remediation")
	joined := strings.Join(cf.NextActions, "\n")
	assert.Contains(t, joined, "arm review")
	assert.True(t,
		strings.Contains(joined, "--output") || strings.Contains(joined, "--bundle"),
		"next_actions should point at a file path, got %v", cf.NextActions)
}

func TestTransitionErrorsCarryStructuredCode_REQ_LNGHZN_S6_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"transition", "--repo", repo, "--issue", "task-01", "--to", "nope", "--format", "agent")
	assert.Equal(t, 1, code)

	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "TRANSITION-1", cf.Code)
	assert.NotEqual(t, armerrors.CodeGeneral1, cf.Code)
	assert.Contains(t, cf.Cause, "invalid status")
	require.NotEmpty(t, cf.NextActions)
	assert.Contains(t, strings.Join(cf.NextActions, "\n"), "arm transition")

	var round armerrors.CommandFailure
	raw, err := json.Marshal(cf)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &round))
	assert.Equal(t, cf.Code, round.Code)
	assert.Equal(t, cf.Cause, round.Cause)
	assert.Equal(t, cf.NextActions, round.NextActions)
	assert.Equal(t, cf.ExitCode, round.ExitCode)
}

func TestReadyAndRenderContextErrorsMapped_REQ_LNGHZN_S6_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	rcOut := new(bytes.Buffer)
	code := executeThenHandleRootError(t, rcOut, new(bytes.Buffer),
		"render-context", "--repo", repo, "--issue", "missing-issue", "--format", "agent")
	assert.Equal(t, 1, code)
	rc := agentFailureFromStdout(t, rcOut.String())
	assert.Equal(t, "RENDER-CONTEXT-1", rc.Code)
	assert.Contains(t, rc.Cause, "not found")
	require.NotEmpty(t, rc.NextActions)

	usageOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, usageOut, new(bytes.Buffer),
		"render-context", "--repo", repo, "--format", "agent")
	assert.Equal(t, 2, code)
	usage := agentFailureFromStdout(t, usageOut.String())
	assert.Equal(t, armerrors.CodeUSAGE, usage.Code)

	readyOut := new(bytes.Buffer)
	readyErr := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(readyOut)
	root.SetErr(readyErr)
	root.SetArgs([]string{"ready", "--repo", repo, "--format", "agent"})
	err := root.Execute()
	if err != nil {
		var cf *armerrors.CommandFailure
		require.True(t, errors.As(err, &cf), "ready RunE error must be a Command Failure, got %T %v", err, err)
		assert.Equal(t, "READY-1", cf.Code)
		assert.NotEqual(t, armerrors.CodeGeneral1, cf.Code)
	}
}
