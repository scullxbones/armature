package orchestrate

import (
	"github.com/scullxbones/armature/internal/ops"
)

// DeriveState replays the op log for a single taskID and returns the derived
// OrchestrateState for crash-resume. Ops belonging to other tasks are filtered
// out. Non-orchestration ops are silently ignored.
//
// Phase state machine:
//
//	pending → dispatched  (OpOrchestrateDispatch)
//	dispatched → running  (OpOrchestrateDispatchComplete)
//	running → verify-failed (OpOrchestrateVerifyFail)
//	verify-failed → retrying (OpOrchestrateRetry)
//	retrying → dispatched (OpOrchestrateDispatch — next cycle)
//	running → escalated   (OpOrchestrateEscalate)
//	running → complete    (OpOrchestrateComplete)
func DeriveState(allOps []ops.Op, taskID string) OrchestrateState {
	state := OrchestrateState{
		Phase: "pending",
	}

	for _, op := range allOps {
		// Filter: only process ops targeting the requested task.
		if op.TargetID != taskID {
			continue
		}

		switch op.Type {
		case ops.OpOrchestrateStart:
			// Start op sets the worktree path and retry budget.
			// Phase remains "pending".
			if op.Payload.WorktreePath != "" {
				state.WorktreePath = op.Payload.WorktreePath
			}
			if op.Payload.RetryBudget > 0 {
				state.RetryBudget = op.Payload.RetryBudget
			}

		case ops.OpOrchestrateDispatch:
			// Transition: pending/retrying → dispatched.
			state.Phase = "dispatched"
			state.Run++
			if op.Payload.PreDispatchRef != "" {
				state.PreDispatchRef = op.Payload.PreDispatchRef
			}
			if op.Payload.WorktreePath != "" {
				state.WorktreePath = op.Payload.WorktreePath
			}
			if op.Payload.RetryBudget > 0 {
				state.RetryBudget = op.Payload.RetryBudget
			}

		case ops.OpOrchestrateDispatchComplete:
			// Transition: dispatched → running.
			state.Phase = "running"

		case ops.OpOrchestrateVerifyFail:
			// Transition: running → verify-failed.
			state.Phase = "verify-failed"
			state.Failed = true

		case ops.OpOrchestrateRetry:
			// Transition: verify-failed → retrying.
			state.Phase = "retrying"
			if op.Payload.RetryBudget >= 0 {
				state.RetryBudget = op.Payload.RetryBudget
			}

		case ops.OpOrchestrateEscalate:
			// Transition: retrying/verify-failed → escalated.
			state.Phase = "escalated"

		case ops.OpOrchestrateComplete:
			// Transition: running → complete.
			state.Phase = "complete"

		case ops.OpOrchestrateCheckResult:
			// Accumulate check results without changing phase.
			state.Checks = append(state.Checks, CheckResult{
				Message: op.Payload.Msg,
				Passed:  true,
			})
		}
	}

	return state
}
