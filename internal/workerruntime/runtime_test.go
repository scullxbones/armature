package workerruntime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	err error
}

func (s *execStub) Run(_ context.Context, issueID string) error {
	s.ran = append(s.ran, issueID)
	return s.err
}

type blockingExecStub struct {
	ran []string
}

func (s *blockingExecStub) Run(ctx context.Context, issueID string) error {
	s.ran = append(s.ran, issueID)
	<-ctx.Done()
	return ctx.Err()
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

func TestExecutionHandoffInvokesSingleTaskOrchestrator(t *testing.T) {
	ready := &readyStub{ids: []string{"T1"}}
	claim := &claimStub{}
	exec := &execStub{err: assert.AnError}
	trace := &traceStub{}
	rt := &Runtime{
		Ready: ready,
		Claim: claim,
		Exec:  exec,
		Trace: trace,
	}

	result, err := rt.Run(context.Background(), RuntimeOptions{WorkerID: "w1"})
	assert.Error(t, err)
	assert.Equal(t, StateEscalated, result.FinalState)
	assert.Equal(t, 0, result.TasksCompleted)
	assert.Equal(t, []string{"T1"}, exec.ran)

	assert.Equal(t, EventTierTrace, DurableAdmission(RuntimeEvent{Type: EventExecutionFailed}))
	assert.Equal(t, EventTierDurable, DurableAdmission(RuntimeEvent{
		Type:           EventExecutionSummary,
		SharedDecision: true,
	}))
}

func TestWorkerRuntimeIntegratesReadyClaimAndOrchestrate(t *testing.T) {
	ready := &readyStub{ids: []string{"TASK-1", "TASK-2"}}
	claim := &claimStub{lose: map[string]bool{"TASK-2": true}}
	exec := &execStub{}
	trace := &traceStub{}
	rt := &Runtime{
		Ready: ready,
		Claim: claim,
		Exec:  exec,
		Trace: trace,
	}

	result, err := rt.Run(context.Background(), RuntimeOptions{WorkerID: "worker-1"})
	require.NoError(t, err)
	assert.Equal(t, StateIdle, result.FinalState)
	assert.Equal(t, 1, result.TasksCompleted)
	assert.Equal(t, []string{"TASK-1"}, exec.ran)
	assert.Contains(t, trace.events, EventClaimLost)
	assert.Contains(t, trace.events, EventExecutionCompleted)
}

type inMemoryReadyProvider struct {
	ids []string
	idx int
}

func (p *inMemoryReadyProvider) NextReady(context.Context) (string, bool, error) {
	if p.idx >= len(p.ids) {
		return "", false, nil
	}
	id := p.ids[p.idx]
	p.idx++
	return id, true, nil
}

type inMemoryClaimer struct {
	claimed []string
}

func (c *inMemoryClaimer) Claim(context.Context, string) (bool, error) {
	c.claimed = append(c.claimed, "claimed")
	return true, nil
}

type inMemoryOrchestrator struct {
	ran []string
}

func (o *inMemoryOrchestrator) Run(_ context.Context, issueID string) error {
	o.ran = append(o.ran, issueID)
	return nil
}

func TestWorkerRuntimeRunsClaimedReadyTaskWithRealAdapters(t *testing.T) {
	ready := &inMemoryReadyProvider{ids: []string{"TASK-READY-1"}}
	claim := &inMemoryClaimer{}
	exec := &inMemoryOrchestrator{}
	rt := &Runtime{Ready: ready, Claim: claim, Exec: exec}

	result, err := rt.Run(context.Background(), RuntimeOptions{WorkerID: "worker-1", MaxTasks: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TasksCompleted)
	assert.Equal(t, StateStopped, result.FinalState)
	assert.Equal(t, []string{"TASK-READY-1"}, exec.ran)
	assert.Len(t, claim.claimed, 1)
}

func TestRuntimeEscalatesOnMaxRuntimeTimeout(t *testing.T) {
	ready := &readyStub{ids: []string{"T1"}}
	claim := &claimStub{}
	exec := &blockingExecStub{}
	rt := &Runtime{Ready: ready, Claim: claim, Exec: exec}

	result, err := rt.Run(context.Background(), RuntimeOptions{
		WorkerID:   "worker-1",
		MaxRuntime: 50 * time.Millisecond,
	})
	require.Error(t, err)
	assert.Equal(t, StateEscalated, result.FinalState)
	assert.ErrorIs(t, result.Err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "runtime timeout")
	assert.Contains(t, err.Error(), "Action:")
}

type wrappedBlockingExecStub struct {
	ran []string
}

func (s *wrappedBlockingExecStub) Run(ctx context.Context, issueID string) error {
	s.ran = append(s.ran, issueID)
	<-ctx.Done()
	return fmt.Errorf("wrapped exec failure: %w", ctx.Err())
}

func TestRuntimeEscalatesOnWrappedMaxRuntimeTimeout(t *testing.T) {
	ready := &readyStub{ids: []string{"T1"}}
	claim := &claimStub{}
	exec := &wrappedBlockingExecStub{}
	rt := &Runtime{Ready: ready, Claim: claim, Exec: exec}

	result, err := rt.Run(context.Background(), RuntimeOptions{
		WorkerID:   "worker-1",
		MaxRuntime: 50 * time.Millisecond,
	})
	require.Error(t, err)
	assert.Equal(t, StateEscalated, result.FinalState)
	assert.ErrorIs(t, result.Err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "runtime timeout")
}
