package orchestrate

import (
	"context"
	"fmt"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

type engineSpyHarness struct {
	t *testing.T
}

type contextStubGit struct {
	diffOut   string
	diffFiles []string
	commitErr error
}

func (s *contextStubGit) HeadSHA() (string, error)              { return "head", nil }
func (s *contextStubGit) DiffFrom(string) (string, error)       { return s.diffOut, nil }
func (s *contextStubGit) DiffNameOnly(string) ([]string, error) { return s.diffFiles, nil }
func (s *contextStubGit) ResetHard(string) error                { return nil }
func (s *contextStubGit) ApplyPatch([]byte) error               { return nil }
func (s *contextStubGit) AddAll() error                         { return nil }
func (s *contextStubGit) AddPaths([]string) error               { return nil }
func (s *contextStubGit) CommitWithMessage(string) error        { return s.commitErr }
func (s *contextStubGit) RemoveWorktree(string) error           { return nil }

type contextStubLog struct{ ops []ops.Op }

func (l *contextStubLog) ReadAll() ([]ops.Op, error) { return l.ops, nil }
func (l *contextStubLog) Append(op ops.Op) error {
	l.ops = append(l.ops, op)
	return nil
}

func (h *engineSpyHarness) Name() string { return "spy" }

func (h *engineSpyHarness) Run(ctx context.Context, _ HarnessConfig, _ RunOptions) (CheckResult, error) {
	issue := issueFromCtx(ctx)
	require.Equal(h.t, "TASK-CTX", issue.TaskID)
	require.Equal(h.t, "Context wiring", issue.TaskTitle)
	require.Contains(h.t, issue.TaskContract, "test_passes")
	require.Contains(h.t, issue.StructuredContext, `"issue_id":"TASK-CTX"`)
	require.Contains(h.t, issue.StructuredContext, `"name":"core_spec"`)
	return CheckResult{Name: "spy", Passed: true, Severity: SeverityInfo, Message: "ok"}, nil
}

func TestEngine_BuildsStructuredContextRightBeforeHarnessRun(t *testing.T) {
	priorOps := []ops.Op{
		{
			Type:     ops.OpOrchestrateDispatch,
			TargetID: "TASK-CTX",
			Payload: ops.Payload{
				PreDispatchRef: "base123",
				WorktreePath:   "/wt/TASK-CTX",
				RetryBudget:    1,
			},
		},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "TASK-CTX"},
	}

	git := &contextStubGit{
		diffOut:   "",
		diffFiles: []string{},
		commitErr: fmt.Errorf("nothing to commit: index is clean"),
	}
	log := &contextStubLog{ops: priorOps}

	var calls int
	engine := NewEngine(EngineConfig{
		TaskID:       "TASK-CTX",
		TaskTitle:    "Context wiring",
		TaskContract: `[{"type":"test_passes","cmd":"go test ./..."}]`,
		Git:          git,
		OpLog:        log,
		Harness:      &engineSpyHarness{t: t},
		Scope:        []string{"internal/orchestrate/"},
		RetryBudget:  1,
		BuildTaskContext: func(context.Context, string) (string, error) {
			calls++
			return `{"issue_id":"TASK-CTX","layers":[{"name":"core_spec","priority":1,"content":"Definition of Done"}]}`, nil
		},
	})

	_, err := engine.Run(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, calls, "expected context builder to run exactly once at harness dispatch")
}
