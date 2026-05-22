package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOrchestrateService struct {
	state    orchestrate.OrchestrateState
	err      error
	runCalls int
}

func (f *fakeOrchestrateService) Run(_ context.Context, _ orchestrate.RunInput) (orchestrate.OrchestrateState, error) {
	f.runCalls++
	if f.err != nil {
		return orchestrate.OrchestrateState{}, f.err
	}
	return f.state, nil
}

func TestOrchestrateCommandDelegatesToService(t *testing.T) {
	service := &fakeOrchestrateService{
		state: orchestrate.OrchestrateState{Phase: "complete", Run: 1},
	}
	cmd := newOrchestrateCmdForService(service)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--issue", "TASK-001", "--dry-run"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, 1, service.runCalls)
	assert.Contains(t, buf.String(), "\"phase\":\"complete\"")
}

func TestOrchestrateCommand_ShowsTimeoutSummary(t *testing.T) {
	service := &fakeOrchestrateService{
		err: &orchestrate.RunError{
			Cause: errors.New("context deadline exceeded"),
			Diagnostics: orchestrate.TimeoutDiagnostics{
				ElapsedMs: 42,
				LastPhase: "running",
				Harness:   "codex",
				Retries:   1,
				NextStep:  "retry",
			},
		},
	}
	cmd := newOrchestrateCmdForService(service)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"--issue", "TASK-001", "--dry-run"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Equal(t, 1, service.runCalls)
	assert.Contains(t, errOut.String(), "timeout/failure summary")
	assert.NotContains(t, out.String(), "\"phase\"")
}
