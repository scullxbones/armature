package orchestrate_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/orchestrate"
)

// makeOp is a helper to construct a minimal ops.Op for testing.
func makeOp(opType, targetID string, payload ops.Payload) ops.Op {
	return ops.Op{
		Type:     opType,
		TargetID: targetID,
		Payload:  payload,
	}
}

// --- DeriveState: initial state (no ops) ---

func TestDeriveState_NilOps_ReturnsPending(t *testing.T) {
	state := orchestrate.DeriveState(nil, "T1")
	if state.Phase != "pending" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "pending")
	}
	if state.Run != 0 {
		t.Errorf("Run: got %d, want 0", state.Run)
	}
}

func TestDeriveState_EmptyOps_ReturnsPending(t *testing.T) {
	state := orchestrate.DeriveState([]ops.Op{}, "T1")
	if state.Phase != "pending" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "pending")
	}
}

// --- Transition 1: pending → dispatched ---

func TestDeriveState_Dispatch_TransitionsToPending(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
		}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "dispatched" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "dispatched")
	}
	if state.PreDispatchRef != "abc123" {
		t.Errorf("PreDispatchRef: got %q, want %q", state.PreDispatchRef, "abc123")
	}
	if state.WorktreePath != "/worktrees/T1" {
		t.Errorf("WorktreePath: got %q, want %q", state.WorktreePath, "/worktrees/T1")
	}
	if state.Run != 1 {
		t.Errorf("Run: got %d, want 1", state.Run)
	}
}

// --- Transition 2: dispatched → running ---

func TestDeriveState_DispatchComplete_TransitionsToRunning(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "running" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "running")
	}
}

// --- Transition 3: running → verify-failed ---

func TestDeriveState_VerifyFail_TransitionsToVerifyFailed(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
			RetryBudget:    3,
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateVerifyFail, "T1", ops.Payload{
			Msg: "tests failed",
		}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "verify-failed" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "verify-failed")
	}
}

// --- Transition 4: verify-failed → retrying ---

func TestDeriveState_Retry_TransitionsToRetrying(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
			RetryBudget:    3,
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateVerifyFail, "T1", ops.Payload{Msg: "tests failed"}),
		makeOp(ops.OpOrchestrateRetry, "T1", ops.Payload{RetryBudget: 2}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "retrying" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "retrying")
	}
	if state.RetryBudget != 2 {
		t.Errorf("RetryBudget: got %d, want 2", state.RetryBudget)
	}
}

// --- Transition 5: retrying → dispatched (second dispatch cycle) ---

func TestDeriveState_SecondDispatch_TransitionsFromRetrying(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
			RetryBudget:    3,
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateVerifyFail, "T1", ops.Payload{Msg: "tests failed"}),
		makeOp(ops.OpOrchestrateRetry, "T1", ops.Payload{RetryBudget: 2}),
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "def456",
			WorktreePath:   "/worktrees/T1",
		}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "dispatched" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "dispatched")
	}
	if state.Run != 2 {
		t.Errorf("Run: got %d, want 2", state.Run)
	}
	if state.PreDispatchRef != "def456" {
		t.Errorf("PreDispatchRef: got %q, want %q", state.PreDispatchRef, "def456")
	}
}

// --- Transition 6: running → escalated ---

func TestDeriveState_Escalate_TransitionsToEscalated(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
			RetryBudget:    3,
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateVerifyFail, "T1", ops.Payload{Msg: "tests failed"}),
		makeOp(ops.OpOrchestrateRetry, "T1", ops.Payload{RetryBudget: 0}),
		makeOp(ops.OpOrchestrateEscalate, "T1", ops.Payload{Msg: "retry budget exhausted"}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "escalated" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "escalated")
	}
}

// --- Transition 7: running → complete ---

func TestDeriveState_Complete_TransitionsToComplete(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
			RetryBudget:    3,
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateComplete, "T1", ops.Payload{Msg: "no changes committed; lifecycle transition skipped"}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "complete" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "complete")
	}
	if state.CompletionMessage != "no changes committed; lifecycle transition skipped" {
		t.Errorf("CompletionMessage: got %q", state.CompletionMessage)
	}
}

// --- OrchestrateStart carries WorktreePath and RetryBudget ---

func TestDeriveState_Start_SetsWorktreeAndBudget(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateStart, "T1", ops.Payload{
			WorktreePath: "/worktrees/T1",
			RetryBudget:  5,
		}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "pending" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "pending")
	}
	if state.WorktreePath != "/worktrees/T1" {
		t.Errorf("WorktreePath: got %q, want %q", state.WorktreePath, "/worktrees/T1")
	}
	if state.RetryBudget != 5 {
		t.Errorf("RetryBudget: got %d, want 5", state.RetryBudget)
	}
}

// --- CheckResult ops are accumulated ---

func TestDeriveState_CheckResult_AccumulatesChecks(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateCheckResult, "T1", ops.Payload{
			Msg: "build passed",
		}),
		makeOp(ops.OpOrchestrateCheckResult, "T1", ops.Payload{
			Msg: "lint passed",
		}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "running" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "running")
	}
	if len(state.Checks) != 2 {
		t.Errorf("Checks count: got %d, want 2", len(state.Checks))
	}
}

// --- Multi-task filter: ops for other tasks are ignored ---

func TestDeriveState_MultiTask_FiltersByTaskID(t *testing.T) {
	allOps := []ops.Op{
		// T2 ops that should be ignored
		makeOp(ops.OpOrchestrateDispatch, "T2", ops.Payload{
			PreDispatchRef: "zzz999",
			WorktreePath:   "/worktrees/T2",
			RetryBudget:    1,
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T2", ops.Payload{}),
		makeOp(ops.OpOrchestrateComplete, "T2", ops.Payload{}),

		// T1 ops that should be processed
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
			RetryBudget:    3,
		}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
	}

	state := orchestrate.DeriveState(allOps, "T1")
	if state.Phase != "running" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "running")
	}
	if state.WorktreePath != "/worktrees/T1" {
		t.Errorf("WorktreePath: got %q, want %q", state.WorktreePath, "/worktrees/T1")
	}
	if state.PreDispatchRef != "abc123" {
		t.Errorf("PreDispatchRef: got %q, want %q", state.PreDispatchRef, "abc123")
	}
	if state.RetryBudget != 3 {
		t.Errorf("RetryBudget: got %d, want 3", state.RetryBudget)
	}
}

// --- Non-orchestration ops are skipped without error ---

func TestDeriveState_NonOrchestrationOps_AreIgnored(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpNote, "T1", ops.Payload{Msg: "some note"}),
		makeOp(ops.OpClaim, "T1", ops.Payload{TTL: 300}),
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{
			PreDispatchRef: "abc123",
			WorktreePath:   "/worktrees/T1",
		}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.Phase != "dispatched" {
		t.Errorf("Phase: got %q, want %q", state.Phase, "dispatched")
	}
}

// Fix 6: TestDeriveState_TransitionWritten_WhenTransitionOpPresent verifies that
// TransitionWritten is true when an OpTransition targeting the task is in the ops.
func TestDeriveState_TransitionWritten_WhenTransitionOpPresent(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{PreDispatchRef: "abc123"}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateComplete, "T1", ops.Payload{}),
		makeOp(ops.OpTransition, "T1", ops.Payload{To: ops.StatusDone, Outcome: "done"}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if !state.TransitionWritten {
		t.Errorf("TransitionWritten: got false, want true when OpTransition is present")
	}
}

// Fix 6: TestDeriveState_TransitionWritten_FalseWhenTransitionOpAbsent verifies
// that TransitionWritten is false when no OpTransition is in the ops.
func TestDeriveState_TransitionWritten_FalseWhenTransitionOpAbsent(t *testing.T) {
	taskOps := []ops.Op{
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{PreDispatchRef: "abc123"}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateComplete, "T1", ops.Payload{}),
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.TransitionWritten {
		t.Errorf("TransitionWritten: got true, want false when OpTransition is absent")
	}
}

// P1-3: TestDeriveState_PriorManualTransitionDoesNotSatisfyCompletionTransitionWritten
// verifies that an OpTransition written BEFORE OpOrchestrateComplete (e.g. a manual
// in-progress transition) does not satisfy the TransitionWritten completion guard.
func TestDeriveState_PriorManualTransitionDoesNotSatisfyCompletionTransitionWritten(t *testing.T) {
	taskOps := []ops.Op{
		// Manual in-progress transition BEFORE orchestration begins.
		makeOp(ops.OpTransition, "T1", ops.Payload{To: "in-progress"}),
		makeOp(ops.OpOrchestrateDispatch, "T1", ops.Payload{PreDispatchRef: "abc123"}),
		makeOp(ops.OpOrchestrateDispatchComplete, "T1", ops.Payload{}),
		makeOp(ops.OpOrchestrateComplete, "T1", ops.Payload{}),
		// No post-completion OpTransition (crash between complete and done-transition ops).
	}
	state := orchestrate.DeriveState(taskOps, "T1")
	if state.TransitionWritten {
		t.Error("TransitionWritten should be false: OpTransition before OpOrchestrateComplete must not satisfy completion-transition guard")
	}
}
