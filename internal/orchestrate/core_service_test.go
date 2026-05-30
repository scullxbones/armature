package orchestrate

import (
	"context"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	unix int64
}

func (f fakeClock) NowUnix() int64 { return f.unix }

type fakeServiceGit struct {
	head string
}

func (f *fakeServiceGit) HeadSHA() (string, error)              { return f.head, nil }
func (f *fakeServiceGit) DiffFrom(string) (string, error)       { return "", nil }
func (f *fakeServiceGit) DiffNameOnly(string) ([]string, error) { return nil, nil }
func (f *fakeServiceGit) ResetHard(string) error                { return nil }
func (f *fakeServiceGit) ApplyPatch([]byte) error               { return nil }
func (f *fakeServiceGit) AddAll() error                         { return nil }
func (f *fakeServiceGit) AddPaths([]string) error               { return nil }
func (f *fakeServiceGit) CommitWithMessage(string) error        { return nil }
func (f *fakeServiceGit) RemoveWorktree(string) error           { return nil }

type fakeServiceOpLog struct {
	ops         []ops.Op
	appendCalls int
	appended    []ops.Op
}

func (f *fakeServiceOpLog) ReadAll() ([]ops.Op, error) { return f.ops, nil }
func (f *fakeServiceOpLog) Append(op ops.Op) error {
	f.appendCalls++
	f.appended = append(f.appended, op)
	return nil
}

func TestPlanNextStepReturnsEffectsWithoutTouchingAdapters(t *testing.T) {
	state := OrchestrateState{Phase: "pending"}

	next, effects := PlanNextStep(state, DecisionInput{
		TaskID:      "TASK-001",
		WorkerID:    "worker-a",
		NowUnix:     1700000000,
		RetryBudget: 3,
		Scope:       []string{"internal/orchestrate/core.go"},
		ActiveScopes: map[string][]string{
			"TASK-999": {"internal/other/file.go"},
		},
		PreDispatchRef: "abc123",
		WorktreePath:   "/tmp/task-001",
	})

	assert.Equal(t, "dispatched", next.Phase)
	require.Len(t, effects, 1)
	assert.Equal(t, EffectAppendDispatchOp, effects[0].Kind)
	assert.Equal(t, "TASK-001", effects[0].TaskID)
	assert.Equal(t, "abc123", effects[0].Op.Payload.PreDispatchRef)
}

func TestServiceExecutesPlannedEffectsThroughPorts(t *testing.T) {
	opLog := &fakeServiceOpLog{}
	service := NewService(ServiceConfig{
		Git:   &fakeServiceGit{head: "abc123"},
		OpLog: opLog,
		Clock: fakeClock{unix: 1700000000},
	})

	state, err := service.Run(context.Background(), RunInput{
		TaskID:       "TASK-001",
		WorkerID:     "worker-a",
		RetryBudget:  3,
		ActiveScopes: map[string][]string{},
		Scope:        []string{"internal/orchestrate/core.go"},
		Opts:         RunOptions{WorkDir: "/tmp/task-001"},
	})

	require.NoError(t, err)
	assert.Equal(t, "dispatched", state.Phase)
	assert.Equal(t, 1, opLog.appendCalls)
}
