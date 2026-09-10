package claim

import (
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
)

// LeaseFacts is the prior claim lease captured before a compensating Transition.
type LeaseFacts struct {
	Status                 string
	ClaimedBy              string
	ClaimedAt              int64
	LastHeartbeat          int64
	ClaimTTL               int
	ClaimingWorkerActivity int64
	WorktreePath           string
	ClaimToken             string
}

// CompensationInput is the borrowed, read-only decision input for PlanCompensation.
type CompensationInput struct {
	Prior      LeaseFacts
	WorkerID   string
	Now        int64
	ClaimToken string // IfClaimToken of the Claim this compensates
}

// PlanCompensation is a pure restore-vs-release decision. It returns the
// compensating Transition payload: restore a live same-Worker lease, or
// release a stale/foreign lease to open. Empty WorkerID or ClaimToken is an
// input error. Inputs are never mutated.
func PlanCompensation(in CompensationInput) (ops.Payload, error) {
	if in.WorkerID == "" {
		return ops.Payload{}, fmt.Errorf("worker ID is required")
	}
	if in.ClaimToken == "" {
		return ops.Payload{}, fmt.Errorf("claim token is required")
	}

	payload := ops.Payload{
		RestoreClaim: true,
		IfClaimToken: in.ClaimToken,
	}

	liveSameWorker := in.Prior.ClaimedBy == in.WorkerID &&
		!IsClaimStale(in.Prior.ClaimedAt, in.Prior.LastHeartbeat, in.Prior.ClaimingWorkerActivity, in.Prior.ClaimTTL, in.Now)
	if liveSameWorker {
		payload.To = in.Prior.Status
		payload.RestoreClaimedBy = in.Prior.ClaimedBy
		payload.RestoreClaimedAt = in.Prior.ClaimedAt
		payload.RestoreClaimTTL = in.Prior.ClaimTTL
		payload.RestoreLastHeartbeat = in.Prior.LastHeartbeat
		payload.RestoreLastClaimingWorkerActivity = in.Prior.ClaimingWorkerActivity
		payload.RestoreClaimToken = in.Prior.ClaimToken
	} else {
		payload.To = ops.StatusOpen
	}

	if in.Prior.WorktreePath != "" {
		payload.WorktreePath = in.Prior.WorktreePath
	} else {
		payload.ClearWorktreePath = true
	}

	return payload, nil
}
