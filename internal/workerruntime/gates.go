package workerruntime

import (
	"context"
	"errors"

	"github.com/scullxbones/armature/internal/ready"
)

var (
	// ErrWorkerDisabled indicates runtime execution is disabled for this worker.
	ErrWorkerDisabled = errors.New("worker disabled")
	// ErrClaimConflict indicates another worker won the claim race.
	ErrClaimConflict = errors.New("claim conflict")
)

// ReadyQueueReader provides a structured ready queue for polling.
type ReadyQueueReader interface {
	Ready(ctx context.Context, workerID string) ([]ready.ReadyEntry, error)
}

// IssueMetaReader provides deterministic issue metadata needed by claim gates.
type IssueMetaReader interface {
	Confidence(ctx context.Context, issueID string) (string, error)
}

// ClaimAttemptor performs claim attempts without string-parsing CLI output.
type ClaimAttemptor interface {
	Claim(ctx context.Context, issueID, workerID string, ttlMinutes int) error
}

// PollAdapter evaluates one structured poll cycle.
func PollAdapter(ctx context.Context, input PollInput, queue ReadyQueueReader) (PollResult, error) {
	entries, err := queue.Ready(ctx, input.WorkerID)
	if err != nil {
		if errors.Is(err, ErrWorkerDisabled) {
			return PollResult{
				Outcome:      PollWorkerDisabled,
				NextState:    StateIdle,
				RecheckAfter: input.Policy.Cooldown.WorkerLocalDelay,
			}, nil
		}
		return PollResult{}, err
	}
	if len(entries) == 0 {
		return PollResult{
			Outcome:      PollNoReadyWork,
			NextState:    NextState(StatePolling, TriggerNoReadyWork),
			RecheckAfter: input.Policy.Cooldown.NoReadyWorkDelay,
		}, nil
	}
	candidate := entries[0]
	return PollResult{
		Outcome:      PollReadyWork,
		NextState:    NextState(StatePolling, TriggerClaimSelected),
		RecheckAfter: 0,
		Candidate:    &candidate,
	}, nil
}

// ClaimGateInput contains deterministic claim-gate parameters.
type ClaimGateInput struct {
	WorkerID   string
	TTLMinutes int
	IssueID    string
}

// ClaimGate evaluates one structured claim attempt.
func ClaimGate(ctx context.Context, input ClaimGateInput, claimer ClaimAttemptor, meta IssueMetaReader) (ClaimResult, error) {
	confidence, err := meta.Confidence(ctx, input.IssueID)
	if err != nil {
		return ClaimResult{}, err
	}
	if confidence == "inferred" {
		return ClaimResult{
			Outcome:    ClaimBlocked,
			NextState:  StateClaimLost,
			ReasonCode: "inferred_node",
		}, nil
	}

	err = claimer.Claim(ctx, input.IssueID, input.WorkerID, input.TTLMinutes)
	if err == nil {
		return ClaimResult{
			Outcome:   ClaimWon,
			NextState: NextState(StateClaimPending, TriggerClaimWon),
		}, nil
	}
	if errors.Is(err, ErrClaimConflict) {
		return ClaimResult{
			Outcome:    ClaimLost,
			NextState:  NextState(StateClaimPending, TriggerClaimLost),
			ReasonCode: "claim_conflict",
		}, nil
	}
	return ClaimResult{}, err
}

// StaticReadyQueue is a test-oriented queue adapter.
type StaticReadyQueue struct {
	Entries []ready.ReadyEntry
	Err     error
}

func (q StaticReadyQueue) Ready(context.Context, string) ([]ready.ReadyEntry, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	return append([]ready.ReadyEntry(nil), q.Entries...), nil
}

// StaticIssueMeta is a test-oriented issue metadata adapter.
type StaticIssueMeta struct {
	Values map[string]string
}

func (m StaticIssueMeta) Confidence(_ context.Context, issueID string) (string, error) {
	if m.Values == nil {
		return "", nil
	}
	return m.Values[issueID], nil
}

// StaticClaimer is a test-oriented claim adapter.
type StaticClaimer struct {
	Err error
}

func (c StaticClaimer) Claim(context.Context, string, string, int) error {
	return c.Err
}
