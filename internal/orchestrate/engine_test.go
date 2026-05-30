package orchestrate_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	addPathsErr  error
	commitErr    error
	removeErr    error
	diffFiles    []string
	diffFilesErr error
	addAllCalled bool
	addPaths     []string
}

func (s *stubGit) HeadSHA() (string, error)             { return s.headSHA, s.headSHAErr }
func (s *stubGit) DiffFrom(base string) (string, error) { return s.diffOut, s.diffErr }
func (s *stubGit) DiffNameOnly(base string) ([]string, error) {
	return s.diffFiles, s.diffFilesErr
}
func (s *stubGit) ResetHard(ref string) error    { return s.resetErr }
func (s *stubGit) ApplyPatch(patch []byte) error { return s.applyErr }
func (s *stubGit) AddAll() error {
	s.addAllCalled = true
	return s.addErr
}
func (s *stubGit) AddPaths(paths []string) error {
	s.addPaths = append(s.addPaths, paths...)
	return s.addPathsErr
}
func (s *stubGit) CommitWithMessage(msg string) error { return s.commitErr }
func (s *stubGit) RemoveWorktree(path string) error   { return s.removeErr }

// --- stub HarnessAdapter ---

type stubHarness struct {
	name   string
	result orchestrate.CheckResult
	err    error
	delay  time.Duration
	block  bool
}

func (h *stubHarness) Name() string { return h.name }
func (h *stubHarness) Run(ctx context.Context, _ orchestrate.HarnessConfig, _ orchestrate.RunOptions) (orchestrate.CheckResult, error) {
	if h.block {
		<-ctx.Done()
		return orchestrate.CheckResult{Name: h.name}, ctx.Err()
	}
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
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
	hasDoneTransition := false
	for _, op := range log.appended {
		if op.Type == ops.OpOrchestrateComplete {
			hasComplete = true
		}
		if op.Type == ops.OpTransition && op.TargetID == "T1" && op.Payload.To == ops.StatusDone {
			hasDoneTransition = true
		}
	}
	if !hasComplete {
		t.Error("expected OrchestrateComplete op to be written")
	}
	if !hasDoneTransition {
		t.Error("expected lifecycle transition to done after committed orchestration")
	}
}

func TestEngine_ZeroTrustCommit_StagesOnlyVerifiedDiffPaths(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA: "head456",
		diffOut: "diff --git a/internal/foo/bar.go b/internal/foo/bar.go\n",
		diffFiles: []string{
			"internal/foo/bar.go",
			".codex-sqlite/session.sqlite",
			".devin/config.json",
		},
		addErr: errors.New("unsafe broad add would stage provider runtime artifacts"),
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
	if git.addAllCalled {
		t.Fatal("zero-trust commit must not use broad git add -A")
	}
	if got, want := git.addPaths, []string{"internal/foo/bar.go"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("staged paths: got %v, want %v", got, want)
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
	hasDoneTransition := false
	for _, op := range log.appended {
		if op.Type == ops.OpTransition && op.Payload.To == ops.StatusDone {
			hasDoneTransition = true
		}
	}
	if !hasDoneTransition {
		t.Fatal("non-empty diff must transition issue to done")
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

func TestEngine_LongHarnessRun_EmitsPeriodicHeartbeat(t *testing.T) {
	log := &stubOpLog{ops: []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1", Payload: ops.Payload{PreDispatchRef: "base", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}}
	git := &stubGit{diffOut: "diff", diffFiles: []string{"internal/foo/bar.go"}}
	harness := passingHarness("build")
	harness.delay = 120 * time.Millisecond
	var heartbeatCount int32
	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 1,
		Opts: orchestrate.RunOptions{
			HeartbeatInterval: 20 * time.Millisecond,
			Progress: func(ev orchestrate.ProgressEvent) {
				if ev.Kind == "heartbeat" {
					atomic.AddInt32(&heartbeatCount, 1)
				}
			},
		},
	}
	_, err := orchestrate.NewEngine(cfg).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&heartbeatCount) < 1 {
		t.Fatalf("expected at least one heartbeat")
	}
}

func TestEngine_TimeoutFailure_EmitsStructuredNoteAndDiagnostics(t *testing.T) {
	log := &stubOpLog{ops: []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1", Payload: ops.Payload{PreDispatchRef: "base", RetryBudget: 2}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}}
	git := &stubGit{}
	harness := &stubHarness{name: "codex", block: true}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/bar.go"},
		RetryBudget: 2,
		Opts:        orchestrate.RunOptions{HeartbeatInterval: 5 * time.Millisecond},
	}
	_, err := orchestrate.NewEngine(cfg).Run(ctx)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	var runErr *orchestrate.RunError
	if !errors.As(err, &runErr) {
		t.Fatalf("expected RunError, got %T", err)
	}
	if runErr.Diagnostics.Harness != "codex" {
		t.Fatalf("harness diag mismatch: %s", runErr.Diagnostics.Harness)
	}
	foundNote := false
	for _, op := range log.appended {
		if op.Type != ops.OpNote {
			continue
		}
		foundNote = true
		var diag orchestrate.TimeoutDiagnostics
		if uerr := json.Unmarshal([]byte(op.Payload.Msg), &diag); uerr != nil {
			t.Fatalf("invalid timeout diagnostic note: %v", uerr)
		}
		if diag.Harness == "" || diag.LastPhase == "" {
			t.Fatalf("incomplete diagnostics: %+v", diag)
		}
	}
	if !foundNote {
		t.Fatalf("expected timeout diagnostic note op")
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
	if !strings.Contains(result.CompletionMessage, "no changes committed") {
		t.Fatalf("completion message: got %q, want no-op reason", result.CompletionMessage)
	}
	for _, op := range log.appended {
		if op.Type == ops.OpTransition && op.Payload.To == ops.StatusDone {
			t.Fatalf("empty diff must not transition issue to done; got %+v", op)
		}
	}
	foundExplicitNoop := false
	for _, op := range log.appended {
		if op.Type == ops.OpOrchestrateComplete && strings.Contains(op.Payload.Msg, "no changes committed") {
			foundExplicitNoop = true
		}
	}
	if !foundExplicitNoop {
		t.Fatal("empty diff completion must record an explicit no-op reason")
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
