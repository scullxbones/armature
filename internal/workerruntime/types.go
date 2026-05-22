package workerruntime

import "time"

// Runtime state constants.
const (
	StateIdle         = "idle"
	StatePolling      = "polling"
	StateClaimPending = "claim_pending"
	StateClaimLost    = "claim_lost"
	StateClaimWon     = "claim_won"
	StateExecuting    = "executing"
	StateRecovering   = "recovering"
	StatePaused       = "paused"
	StateEscalated    = "escalated"
	StateStopped      = "stopped"
)

// Trigger constants for deterministic state transitions.
const (
	TriggerClaimSelected     = "claim_selected"
	TriggerClaimLost         = "claim_lost"
	TriggerClaimWon          = "claim_won"
	TriggerExecutionComplete = "execution_complete"
	TriggerExecutionFailed   = "execution_failed"
	TriggerRecoveryComplete  = "recovery_complete"
	TriggerRecoveryFailed    = "recovery_failed"
	TriggerNoReadyWork       = "no_ready_work"
	TriggerPause             = "pause"
	TriggerResume            = "resume"
	TriggerEscalate          = "escalate"
	TriggerStop              = "stop"
	TriggerMaxTasksReached   = "max_tasks_reached"
)

// transitions maps (state, trigger) -> next state.
var transitions = map[[2]string]string{
	{StateIdle, TriggerClaimSelected}:          StateClaimPending,
	{StatePolling, TriggerClaimSelected}:       StateClaimPending,
	{StatePolling, TriggerNoReadyWork}:         StateIdle,
	{StateClaimPending, TriggerClaimWon}:       StateClaimWon,
	{StateClaimPending, TriggerClaimLost}:      StateClaimLost,
	{StateClaimLost, TriggerClaimSelected}:     StateClaimPending,
	{StateClaimLost, TriggerNoReadyWork}:       StateIdle,
	{StateClaimWon, TriggerClaimSelected}:      StateExecuting,
	{StateClaimWon, TriggerExecutionComplete}:  StateExecuting,
	{StateExecuting, TriggerExecutionComplete}: StatePolling,
	{StateExecuting, TriggerExecutionFailed}:   StateRecovering,
	{StateExecuting, TriggerEscalate}:          StateEscalated,
	{StateExecuting, TriggerMaxTasksReached}:   StateStopped,
	{StateRecovering, TriggerRecoveryComplete}: StatePolling,
	{StateRecovering, TriggerRecoveryFailed}:   StateEscalated,
	{StateRecovering, TriggerPause}:            StatePaused,
	{StateRecovering, TriggerStop}:             StateStopped,
	{StatePolling, TriggerPause}:               StatePaused,
	{StateIdle, TriggerPause}:                  StatePaused,
	{StatePaused, TriggerResume}:               StatePolling,
	{StatePolling, TriggerStop}:                StateStopped,
	{StateIdle, TriggerStop}:                   StateStopped,
	{StatePaused, TriggerStop}:                 StateStopped,
	{StateEscalated, TriggerStop}:              StateStopped,
}

// InitialState returns the starting state for a new runtime.
func InitialState() string {
	return StateIdle
}

// NextState returns the next state for a given (current state, trigger) pair.
// Returns the current state unchanged if no transition is defined.
func NextState(current, trigger string) string {
	if next, ok := transitions[[2]string{current, trigger}]; ok {
		return next
	}
	return current
}

// PollResult is the structured outcome of a poll-gate evaluation.
type PollResult struct {
	Outcome      string
	NextState    string
	RecheckAfter time.Duration
	Candidate    interface{} // *ready.ReadyEntry when outcome == PollReadyWork
}

// ClaimResult is the structured outcome of a claim-gate evaluation.
type ClaimResult struct {
	Outcome    string
	NextState  string
	ReasonCode string
}

// ExecuteResult is the structured outcome of an execute-gate evaluation.
type ExecuteResult struct {
	Outcome   string
	NextState string
}

// RecoveryResult is the structured outcome of a recovery-gate evaluation.
type RecoveryResult struct {
	Outcome   string
	NextState string
}

// ResumeResult is the structured outcome of a resume-gate evaluation.
type ResumeResult struct {
	Outcome   string
	NextState string
}

// StopResult is the structured outcome of a stop-gate evaluation.
type StopResult struct {
	Outcome   string
	NextState string
}

// Poll outcome constants.
const (
	PollReadyWork      = "ready_work"
	PollNoReadyWork    = "no_ready_work"
	PollWorkerDisabled = "worker_disabled"
)

// Claim outcome constants.
const (
	ClaimWon     = "claim_won"
	ClaimLost    = "claim_lost"
	ClaimBlocked = "claim_blocked"
)

// Execute outcome constants.
const (
	ExecuteSuccess          = "success"
	ExecuteAlreadyComplete  = "already_complete"
	ExecuteAlreadyEscalated = "already_escalated"
	ExecuteFailure          = "failure"
	ExecuteCancelled        = "cancelled"
)

// RuntimeOptions configures a worker runtime instance.
type RuntimeOptions struct {
	WorkerID   string
	MaxTasks   int
	MaxRuntime time.Duration
	DryRun     bool
	Policy     Policy
}

// RunResult summarises the outcome of a completed runtime run.
type RunResult struct {
	TasksCompleted int
	FinalState     string
	Err            error
}

// PollInput carries inputs to a poll-gate evaluation.
type PollInput struct {
	WorkerID string
	Policy   Policy
	Now      time.Time
}
