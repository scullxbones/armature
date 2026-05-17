package workerruntime

import (
	"context"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

type readyStub struct {
	ids []string
	i   int
}

func (s *readyStub) NextReady(_ context.Context) (string, bool, error) {
	if s.i >= len(s.ids) {
		return "", false, nil
	}
	id := s.ids[s.i]
	s.i++
	return id, true, nil
}

type claimStub struct {
	lose map[string]bool
}

func (s *claimStub) Claim(_ context.Context, issueID string) (bool, error) {
	return !s.lose[issueID], nil
}

type execStub struct {
	ran []string
}

func (s *execStub) Run(_ context.Context, issueID string) error {
	s.ran = append(s.ran, issueID)
	return nil
}

type traceStub struct {
	events []string
}

func (s *traceStub) Trace(event string) {
	s.events = append(s.events, event)
}

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

func TestRuntimeDrainsMultipleTasksAndHandlesClaimContention(t *testing.T) {
	ready := &readyStub{ids: []string{"T1", "T2", "T3"}}
	claim := &claimStub{lose: map[string]bool{"T2": true}}
	exec := &execStub{}
	trace := &traceStub{}
	rt := &Runtime{
		Ready: ready,
		Claim: claim,
		Exec:  exec,
		Trace: trace,
	}

	result, err := rt.Run(context.Background(), RuntimeOptions{WorkerID: "w1"})
	assert.NoError(t, err)
	assert.Equal(t, 2, result.TasksCompleted)
	assert.Equal(t, StateIdle, result.FinalState)
	assert.Equal(t, []string{"T1", "T3"}, exec.ran)
	assert.Contains(t, trace.events, EventClaimLost)
}

func TestRecoveryPauseAndStopTransitionsRemainDeterministic(t *testing.T) {
	assert.Equal(t, StatePolling, NextState(StateRecovering, TriggerRecoveryComplete))
	assert.Equal(t, StateEscalated, NextState(StateRecovering, TriggerRecoveryFailed))
	assert.Equal(t, StatePaused, NextState(StateRecovering, TriggerPause))
	assert.Equal(t, StateStopped, NextState(StateRecovering, TriggerStop))
	assert.Equal(t, StateStopped, NextState(StatePaused, TriggerStop))
	assert.Equal(t, StateStopped, NextState(StateEscalated, TriggerStop))

	assert.Equal(t, EventTierSnapshot, DurableAdmission(RuntimeEvent{Type: EventCooldownScheduled}))
	assert.Equal(t, EventTierSnapshot, DurableAdmission(RuntimeEvent{Type: EventPauseCheckpoint}))
	assert.Equal(t, EventTierSnapshot, DurableAdmission(RuntimeEvent{Type: EventStopRequested}))

	assert.Equal(t, EventTierSnapshot, DurableAdmission(RuntimeEvent{Type: EventHumanEscalation}))
	assert.Equal(t, EventTierDurable, DurableAdmission(RuntimeEvent{
		Type:           EventHumanEscalation,
		SharedDecision: true,
	}))
}
