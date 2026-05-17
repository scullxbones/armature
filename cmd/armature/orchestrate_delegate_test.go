package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOrchestrateService struct {
	state    orchestrate.OrchestrateState
	runCalls int
}

func (f *fakeOrchestrateService) Run(_ context.Context, _ orchestrate.RunInput) (orchestrate.OrchestrateState, error) {
	f.runCalls++
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
