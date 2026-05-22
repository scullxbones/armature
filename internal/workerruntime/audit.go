package workerruntime

import (
	"time"

	"github.com/scullxbones/armature/internal/ops"
)

type DurableDecisionInput struct {
	IssueID       string
	WorkerID      string
	Message       string
	CorrelationID string
	CausationID   string
	DecisionClass string
}

// BuildDurableRuntimeDecisionOp builds the durable op payload for shared decisions.
func BuildDurableRuntimeDecisionOp(in DurableDecisionInput) ops.Op {
	return ops.Op{
		Type:      ops.OpWorkerRuntimeDecision,
		TargetID:  in.IssueID,
		Timestamp: time.Now().Unix(),
		WorkerID:  in.WorkerID,
		Payload: ops.Payload{
			Msg:           in.Message,
			CorrelationID: in.CorrelationID,
			CausationID:   in.CausationID,
			DecisionClass: in.DecisionClass,
		},
	}
}
