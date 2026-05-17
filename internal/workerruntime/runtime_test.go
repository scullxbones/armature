package workerruntime

import (
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestDefaultPolicyAndStatesExposeV1RuntimeSurface(t *testing.T) {
	policy := DefaultPolicy()

	assert.Equal(t, StateIdle, InitialState())
	assert.Equal(t, StateClaimPending, NextState(StatePolling, TriggerClaimSelected))
	assert.Equal(t, "execution_lane_routing", policy.Subworkflows.ExecutionLaneRouting.Workflow)
	assert.False(t, policy.Subworkflows.ExecutionLaneRouting.ExceptionAgentEnabled)
	assert.Equal(t, 1, policy.Retry.MaxVerificationFailures)
	assert.Equal(t, 30*time.Second, policy.Cooldown.NoReadyWorkDelay)
}

func TestDurableAdmissionKeepsExecutionNoiseOutOfOpsLog(t *testing.T) {
	assert.Equal(t, EventTierTrace, DurableAdmission(RuntimeEvent{Type: EventPollStarted}))
	assert.Equal(t, EventTierTrace, DurableAdmission(RuntimeEvent{Type: EventNoReadyWork}))
	assert.Equal(t, EventTierTrace, DurableAdmission(RuntimeEvent{Type: EventClaimLost}))
	assert.Equal(t, EventTierTrace, DurableAdmission(RuntimeEvent{Type: EventExecutionCompleted}))
	assert.Equal(t, EventTierSnapshot, DurableAdmission(RuntimeEvent{Type: "cooldown_checkpoint"}))
}

func TestDurableRuntimeEventCarriesCorrelationForSharedDecision(t *testing.T) {
	op := BuildDurableRuntimeDecisionOp(DurableDecisionInput{
		IssueID:       "ORCH-RUNTIME-V1-T4",
		WorkerID:      "worker-a",
		Message:       "escalated to human",
		CorrelationID: "corr-123",
		CausationID:   "cause-456",
		DecisionClass: "human_accountable_escalation",
	})

	assert.Equal(t, ops.OpWorkerRuntimeDecision, op.Type)
	assert.Equal(t, "corr-123", op.Payload.CorrelationID)
	assert.Equal(t, "cause-456", op.Payload.CausationID)
	assert.Equal(t, "human_accountable_escalation", op.Payload.DecisionClass)
}
