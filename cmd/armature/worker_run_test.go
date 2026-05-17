package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/scullxbones/armature/internal/workerruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedReady struct {
	ids []string
	i   int
}

func (r *fixedReady) NextReady(_ context.Context) (string, bool, error) {
	if r.i >= len(r.ids) {
		return "", false, nil
	}
	id := r.ids[r.i]
	r.i++
	return id, true, nil
}

type fixedClaim struct{}

func (fixedClaim) Claim(_ context.Context, _ string) (bool, error) { return true, nil }

type fixedExec struct{}

func (fixedExec) Run(_ context.Context, _ string) error { return nil }

func TestWorkerRunMaxTasksOneExecutesSingleTask(t *testing.T) {
	prev := newWorkerRuntime
	newWorkerRuntime = func() *workerruntime.Runtime {
		return &workerruntime.Runtime{
			Ready: &fixedReady{ids: []string{"T1", "T2"}},
			Claim: fixedClaim{},
			Exec:  fixedExec{},
		}
	}
	t.Cleanup(func() { newWorkerRuntime = prev })

	root := newRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"worker", "run", "--max-tasks", "1"})
	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "\"tasks_completed\":1")
}

func TestWorkerRunCommandSupportsSingleTaskDogfood(t *testing.T) {
	prev := newWorkerRuntime
	newWorkerRuntime = func() *workerruntime.Runtime {
		return &workerruntime.Runtime{
			Ready: &fixedReady{ids: []string{"T1"}},
			Claim: fixedClaim{},
			Exec:  fixedExec{},
		}
	}
	t.Cleanup(func() { newWorkerRuntime = prev })

	root := newRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"worker", "run", "--max-tasks", "1", "--dry-run", "--format", "json"})
	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "\"tasks_completed\":1")
	assert.Contains(t, out.String(), "\"dry_run\":true")
}
