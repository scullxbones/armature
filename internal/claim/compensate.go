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
	Prior        LeaseFacts
	WorkerID     string
	Now          int64
	IfClaimToken string
}

// PlanCompensation is a pure restore-vs-release decision. It returns the
// compensating Transition payload encoded from ops.Compensation: restore a
// live same-Worker lease, or release a stale/foreign lease to open. Empty
// WorkerID or IfClaimToken is an input error. Inputs are never mutated.
// Worktree flags are written only via EncodeWorktree.
func PlanCompensation(in CompensationInput) (ops.Payload, error) {
	if in.WorkerID == "" {
		return ops.Payload{}, fmt.Errorf("worker ID is required")
	}
	if in.IfClaimToken == "" {
		return ops.Payload{}, fmt.Errorf("claim token is required")
	}

	comp := ops.Compensation{
		RestoreClaim: true,
		IfClaimToken: in.IfClaimToken,
		Worktree:     WorktreeRestoreClearingEmptyPrior(in.Prior.WorktreePath),
	}

	liveSameWorker := in.Prior.ClaimedBy == in.WorkerID &&
		!IsClaimStale(
			FoldLastActivity(in.Prior.ClaimedAt, in.Prior.LastHeartbeat, in.Prior.ClaimingWorkerActivity),
			in.Prior.ClaimTTL,
			in.Now,
		)
	if liveSameWorker {
		comp.To = in.Prior.Status
		comp.RestoreClaimedBy = in.Prior.ClaimedBy
		comp.RestoreClaimedAt = in.Prior.ClaimedAt
		comp.RestoreClaimTTL = in.Prior.ClaimTTL
		comp.RestoreLastHeartbeat = in.Prior.LastHeartbeat
		comp.RestoreLastClaimingWorkerActivity = in.Prior.ClaimingWorkerActivity
		comp.RestoreClaimToken = in.Prior.ClaimToken
	} else {
		comp.To = ops.StatusOpen
	}

	return comp.Encode(), nil
}
