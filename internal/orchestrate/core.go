package orchestrate

import "github.com/scullxbones/armature/internal/ops"

// EffectKind identifies a planned orchestration side effect.
type EffectKind string

const (
	EffectAppendDispatchOp EffectKind = "append-dispatch-op"
)

// DecisionInput is the pure input surface for orchestration planning.
type DecisionInput struct {
	TaskID         string
	WorkerID       string
	NowUnix        int64
	RetryBudget    int
	Scope          []string
	ActiveScopes   map[string][]string
	PreDispatchRef string
	WorktreePath   string
}

// Effect is a planned side effect for the imperative shell to interpret.
type Effect struct {
	Kind   EffectKind
	TaskID string
	Op     ops.Op
}

// PlanNextStep produces the next orchestration state and any required effects
// without touching external adapters.
func PlanNextStep(state OrchestrateState, input DecisionInput) (OrchestrateState, []Effect) {
	if state.Phase != "pending" && state.Phase != "retrying" {
		return state, nil
	}

	dispatchOp := ops.Op{
		Type:      ops.OpOrchestrateDispatch,
		TargetID:  input.TaskID,
		Timestamp: input.NowUnix,
		WorkerID:  input.WorkerID,
		Payload: ops.Payload{
			PreDispatchRef: input.PreDispatchRef,
			WorktreePath:   input.WorktreePath,
			RetryBudget:    input.RetryBudget,
		},
	}

	next := state
	next.Phase = "dispatched"
	next.PreDispatchRef = input.PreDispatchRef
	next.WorktreePath = input.WorktreePath
	next.RetryBudget = input.RetryBudget
	next.Run++

	return next, []Effect{{
		Kind:   EffectAppendDispatchOp,
		TaskID: input.TaskID,
		Op:     dispatchOp,
	}}
}
