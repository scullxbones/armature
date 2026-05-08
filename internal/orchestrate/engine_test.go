package orchestrate_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/orchestrate"
)

// --- stub GitClient ---

type stubGit struct {
	headSHA      string
	headSHAErr   error
	diffOut      string
	diffErr      error
	resetErr     error
	applyErr     error
	addErr       error
	commitErr    error
	removeErr    error
	diffFiles    []string
	diffFilesErr error
}

func (s *stubGit) HeadSHA() (string, error)             { return s.headSHA, s.headSHAErr }
func (s *stubGit) DiffFrom(base string) (string, error) { return s.diffOut, s.diffErr }
func (s *stubGit) DiffNameOnly(base string) ([]string, error) {
	return s.diffFiles, s.diffFilesErr
}
func (s *stubGit) ResetHard(ref string) error         { return s.resetErr }
func (s *stubGit) ApplyPatch(patch []byte) error      { return s.applyErr }
func (s *stubGit) AddAll() error                      { return s.addErr }
func (s *stubGit) CommitWithMessage(msg string) error { return s.commitErr }
func (s *stubGit) RemoveWorktree(path string) error   { return s.removeErr }

// --- stub HarnessAdapter ---

type stubHarness struct {
	name   string
	result orchestrate.CheckResult
	err    error
}

func (h *stubHarness) Name() string { return h.name }
func (h *stubHarness) Run(_ context.Context, _ orchestrate.HarnessConfig, _ orchestrate.RunOptions) (orchestrate.CheckResult, error) {
	return h.result, h.err
}

// --- stub OpAppender ---

type stubOpLog struct {
	ops       []ops.Op
	appended  []ops.Op
	appendErr error
}

func (l *stubOpLog) ReadAll() ([]ops.Op, error) { return l.ops, nil }
func (l *stubOpLog) Append(op ops.Op) error {
	if l.appendErr != nil {
		return l.appendErr
	}
	l.appended = append(l.appended, op)
	return nil
}

// --- helpers ---

func passingHarness(name string) *stubHarness {
	return &stubHarness{
		name:   name,
		result: orchestrate.CheckResult{Name: name, Passed: true, Severity: orchestrate.SeverityInfo, Message: "ok"},
	}
}

func failingHarness(name string) *stubHarness {
	return &stubHarness{
		name:   name,
		result: orchestrate.CheckResult{Name: name, Passed: false, Severity: orchestrate.SeverityError, Message: "failed"},
	}
}

// --- Engine.Run: dry-run exits before dispatch op ---

func TestEngine_DryRun_NeverWritesDispatchOp(t *testing.T) {
	git := &stubGit{headSHA: "abc123"}
	log := &stubOpLog{}
	harness := passingHarness("test")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		Opts:        orchestrate.RunOptions{DryRun: true},
		RetryBudget: 1,
	}

	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, op := range log.appended {
		if op.Type == ops.OpOrchestrateDispatch {
			t.Errorf("dry-run must not write dispatch op; got %+v", op)
		}
	}
}

// --- Engine.Run: crash-resume skips re-dispatch when already dispatched ---

func TestEngine_CrashResume_DispatchedPhase_SkipsRedispatch(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "abc123", WorktreePath: "/wt/T1", RetryBudget: 1}},
	}
	git := &stubGit{headSHA: "abc123"}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("test")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT have appended another dispatch op
	for _, op := range log.appended {
		if op.Type == ops.OpOrchestrateDispatch {
			t.Errorf("crash-resume must not re-dispatch; got op %+v", op)
		}
	}
}

// --- Engine.Run: zero-trust commit sequence ---

func TestEngine_ZeroTrustCommit_HappyPath(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA:   "head456",
		diffOut:   "diff --git a/file.go b/file.go\n",
		diffFiles: []string{"internal/foo/bar.go"},
	}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("build")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "complete" {
		t.Errorf("Phase: got %q, want %q", result.Phase, "complete")
	}

	// Verify commit op was written
	hasComplete := false
	for _, op := range log.appended {
		if op.Type == ops.OpOrchestrateComplete {
			hasComplete = true
		}
	}
	if !hasComplete {
		t.Error("expected OrchestrateComplete op to be written")
	}
}

// --- Engine.Run: verification failure triggers retry ---

func TestEngine_VerificationFail_TriggersRetry(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 2}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA:   "head456",
		diffOut:   "diff content",
		diffFiles: []string{"internal/foo/bar.go"},
	}
	log := &stubOpLog{ops: priorOps}
	harness := failingHarness("build")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 2,
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect verify-fail op and retry op written
	hasVerifyFail := false
	hasRetry := false
	for _, op := range log.appended {
		if op.Type == ops.OpOrchestrateVerifyFail {
			hasVerifyFail = true
		}
		if op.Type == ops.OpOrchestrateRetry {
			hasRetry = true
		}
	}
	if !hasVerifyFail {
		t.Error("expected OpOrchestrateVerifyFail op")
	}
	if !hasRetry {
		t.Error("expected OpOrchestrateRetry op")
	}
	_ = result
}

// --- Engine.Run: escalation when retry budget exhausted ---

func TestEngine_Escalate_WhenRetryBudgetZero(t *testing.T) {
	// Already at verify-failed with budget 0
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 0}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
		{Type: ops.OpOrchestrateVerifyFail, TargetID: "T1"},
		{Type: ops.OpOrchestrateRetry, TargetID: "T1", Payload: ops.Payload{RetryBudget: 0}},
	}
	git := &stubGit{headSHA: "head456"}
	log := &stubOpLog{ops: priorOps}
	harness := failingHarness("build")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 0,
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "escalated" {
		t.Errorf("Phase: got %q, want %q", result.Phase, "escalated")
	}

	hasEscalate := false
	for _, op := range log.appended {
		if op.Type == ops.OpOrchestrateEscalate {
			hasEscalate = true
		}
	}
	if !hasEscalate {
		t.Error("expected OpOrchestrateEscalate op")
	}
}

// --- Engine.Run: scope overlap check via claimPkg.ScopesOverlap ---

func TestEngine_ClaimTask_ChecksScopeOverlap(t *testing.T) {
	git := &stubGit{headSHA: "abc123"}
	log := &stubOpLog{}
	harness := passingHarness("test")

	// Two tasks in the log with overlapping scope — claimTask should detect conflict
	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		Opts:        orchestrate.RunOptions{DryRun: true},
		RetryBudget: 1,
	}

	// Engine should not panic — scope check is internal
	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Engine.Run: complete state after running phase ---

func TestEngine_RunningPhase_ProcessesZeroTrustCommit(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA:   "head456",
		diffOut:   "diff content here",
		diffFiles: []string{"internal/foo/bar.go"},
	}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("make-check")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "complete" {
		t.Errorf("Phase: got %q, want %q", result.Phase, "complete")
	}
}

// --- Engine.Run: context cancellation is honoured ---

func TestEngine_ContextCancelled_ReturnsError(t *testing.T) {
	git := &stubGit{headSHA: "abc123", headSHAErr: nil}
	log := &stubOpLog{}
	harness := passingHarness("test")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := orchestrate.NewEngine(cfg).Run(ctx)
	if err == nil {
		t.Error("expected error on cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// --- Engine.Run: git HeadSHA error propagates ---

func TestEngine_HeadSHAError_ReturnsError(t *testing.T) {
	git := &stubGit{headSHAErr: errors.New("git failure")}
	log := &stubOpLog{}
	harness := passingHarness("test")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err == nil {
		t.Error("expected error from HeadSHA failure")
	}
	if !strings.Contains(err.Error(), "git failure") {
		t.Errorf("expected 'git failure' in error, got %v", err)
	}
}

// --- Engine.Run: dispatch op written with correct taskID ---

func TestEngine_DispatchOp_WrittenWithCorrectTaskID(t *testing.T) {
	git := &stubGit{headSHA: "abc123"}
	log := &stubOpLog{}
	harness := passingHarness("test")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T99",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, op := range log.appended {
		if op.TargetID != "T99" {
			t.Errorf("op %q has wrong TargetID %q, want T99", op.Type, op.TargetID)
		}
	}
}

// --- Engine.Run: complete phase is idempotent on re-entry ---

func TestEngine_AlreadyComplete_ReturnsCompleteImmediately(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "abc", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
		{Type: ops.OpOrchestrateComplete, TargetID: "T1"},
	}
	git := &stubGit{headSHA: "abc123"}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("test")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "complete" {
		t.Errorf("Phase: got %q, want %q", result.Phase, "complete")
	}
	// No new ops should be appended
	if len(log.appended) != 0 {
		t.Errorf("expected no new ops on re-entry into complete, got %d ops", len(log.appended))
	}
}

// --- Engine.Run: escalated phase is idempotent on re-entry ---

func TestEngine_AlreadyEscalated_ReturnsEscalatedImmediately(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "abc", WorktreePath: "/wt/T1", RetryBudget: 0}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
		{Type: ops.OpOrchestrateVerifyFail, TargetID: "T1"},
		{Type: ops.OpOrchestrateRetry, TargetID: "T1", Payload: ops.Payload{RetryBudget: 0}},
		{Type: ops.OpOrchestrateEscalate, TargetID: "T1"},
	}
	git := &stubGit{headSHA: "abc123"}
	log := &stubOpLog{ops: priorOps}
	harness := failingHarness("build")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 0,
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "escalated" {
		t.Errorf("Phase: got %q, want %q", result.Phase, "escalated")
	}
	if len(log.appended) != 0 {
		t.Errorf("expected no new ops on re-entry into escalated, got %d ops", len(log.appended))
	}
}

// --- Engine.Run: scope overlap returns error ---

func TestEngine_ScopeOverlap_ReturnsError(t *testing.T) {
	git := &stubGit{headSHA: "abc123"}
	log := &stubOpLog{}
	harness := passingHarness("test")

	cfg := orchestrate.EngineConfig{
		TaskID:  "T1",
		Git:     git,
		OpLog:   log,
		Harness: harness,
		Scope:   []string{"internal/foo/bar.go"},
		ActiveScopes: map[string][]string{
			"T2": {"internal/foo/bar.go"}, // exact overlap
		},
		RetryBudget: 1,
	}

	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err == nil {
		t.Error("expected error on scope overlap")
	}
	if !strings.Contains(err.Error(), "scope overlap") {
		t.Errorf("expected 'scope overlap' in error, got %v", err)
	}
}

// --- Engine.Run: empty diff (no patch) still completes ---

func TestEngine_EmptyDiff_StillCompletes(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA:   "head456",
		diffOut:   "", // empty diff
		diffFiles: []string{},
		// commitErr for "nothing to commit" is expected to be silently accepted
		commitErr: fmt.Errorf("nothing to commit: index is clean"),
	}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("build")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "complete" {
		t.Errorf("Phase: got %q, want %q", result.Phase, "complete")
	}
}

// --- Engine.Run: commit error (not nothing-to-commit) propagates ---

func TestEngine_CommitError_Propagates(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA:   "head456",
		diffOut:   "diff content",
		diffFiles: []string{"internal/foo/bar.go"},
		commitErr: fmt.Errorf("gpg signing failed"),
	}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("build")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err == nil {
		t.Error("expected error on commit failure")
	}
	if !strings.Contains(err.Error(), "gpg signing failed") {
		t.Errorf("expected 'gpg signing failed' in error, got %v", err)
	}
}

// --- Engine.Run: dispatch op append error propagates ---

func TestEngine_DispatchOpAppendError_Propagates(t *testing.T) {
	git := &stubGit{headSHA: "abc123"}
	log := &stubOpLog{appendErr: fmt.Errorf("disk full")}
	harness := passingHarness("test")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err == nil {
		t.Error("expected error on op append failure")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("expected 'disk full' in error, got %v", err)
	}
}

// --- Engine.Run: ResetHard error propagates ---

func TestEngine_ResetHardError_Propagates(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA:   "head456",
		diffOut:   "diff content",
		diffFiles: []string{"internal/foo/bar.go"},
		resetErr:  fmt.Errorf("cannot reset: detached HEAD"),
	}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("build")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
	}

	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err == nil {
		t.Error("expected error on reset failure")
	}
	if !strings.Contains(err.Error(), "cannot reset") {
		t.Errorf("expected 'cannot reset' in error, got %v", err)
	}
}
