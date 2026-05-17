package orchestrate

import (
	"context"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
)

type scopeTestGit struct {
	headSHA   string
	diffOut   string
	diffFiles []string
}

func (g *scopeTestGit) HeadSHA() (string, error)              { return g.headSHA, nil }
func (g *scopeTestGit) DiffFrom(string) (string, error)       { return g.diffOut, nil }
func (g *scopeTestGit) DiffNameOnly(string) ([]string, error) { return g.diffFiles, nil }
func (g *scopeTestGit) ResetHard(string) error                { return nil }
func (g *scopeTestGit) ApplyPatch([]byte) error               { return nil }
func (g *scopeTestGit) AddAll() error                         { return nil }
func (g *scopeTestGit) CommitWithMessage(string) error        { return nil }
func (g *scopeTestGit) RemoveWorktree(string) error           { return nil }

type scopeTestOpLog struct {
	ops      []ops.Op
	appended []ops.Op
}

func (l *scopeTestOpLog) ReadAll() ([]ops.Op, error) { return l.ops, nil }
func (l *scopeTestOpLog) Append(op ops.Op) error {
	l.appended = append(l.appended, op)
	return nil
}

type captureScopeHarness struct {
	scopeAtRun []string
}

func (h *captureScopeHarness) Name() string { return "capture-scope" }
func (h *captureScopeHarness) Run(ctx context.Context, _ HarnessConfig, _ RunOptions) (CheckResult, error) {
	h.scopeAtRun = append([]string(nil), issueFromCtx(ctx).Scope...)
	return CheckResult{Name: "capture-scope", Passed: true, Severity: SeverityInfo, Message: "ok"}, nil
}

func TestEngine_RunningPhase_PassesScopeToHarnessContext(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1", Payload: ops.Payload{PreDispatchRef: "base123", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	log := &scopeTestOpLog{ops: priorOps}
	git := &scopeTestGit{
		headSHA:   "head456",
		diffOut:   "diff --git a/file.go b/file.go\n",
		diffFiles: []string{"internal/foo/bar.go"},
	}
	harness := &captureScopeHarness{}
	scope := []string{"internal/foo/bar.go"}

	cfg := EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       scope,
		RetryBudget: 1,
	}

	if _, err := NewEngine(cfg).Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(harness.scopeAtRun) != len(scope) || harness.scopeAtRun[0] != scope[0] {
		t.Fatalf("harness scope: got %v, want %v", harness.scopeAtRun, scope)
	}
}
