package claim

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/scullxbones/armature/internal/ops"
)

// DefaultReplayTTLMinutes is the lease length used when a historical claim op
// recorded ttl 0 or omitted ttl. Writes of TTL 0 are rejected; this value is
// only a read-time default (same magnitude as config's omitted default_ttl).
const DefaultReplayTTLMinutes = 60

// Lease is the single claim-ownership value. Issue claim fields cache it.
type Lease struct {
	Holder       string
	Token        string
	Since        int64
	LastActivity int64
	TTLMinutes   int // recorded; 0 or negative replays as DefaultReplayTTLMinutes
	WorktreePath string
	Status       string
}

// ReplayTTLMinutes maps a recorded claim TTL onto a positive minute count.
func ReplayTTLMinutes(recorded int) int {
	if recorded <= 0 {
		return DefaultReplayTTLMinutes
	}
	return recorded
}

// ValidateTTLMinutes is the write-side D10 twin: a claim TTL must be a
// positive number of minutes that cannot overflow duration conversion.
func ValidateTTLMinutes(ttl int) error {
	if ttl <= 0 {
		return fmt.Errorf("ttl %d is out of range (must be > 0 minutes)", ttl)
	}
	if ttlOverflows(ttl) {
		return fmt.Errorf("ttl %d is out of range (must not overflow claim TTL seconds)", ttl)
	}
	return nil
}

func ttlOverflows(ttl int) bool {
	n := int64(ttl)
	return n > math.MaxInt64/int64(time.Minute) || n > math.MaxInt64/(2*60) || n > math.MaxInt64/60
}

// NewClaimOp is the only production constructor for claim ops.
func NewClaimOp(issueID, workerID string, ts int64, ttl int, worktreePath, token string) (ops.Op, error) {
	if err := ValidateTTLMinutes(ttl); err != nil {
		return ops.Op{}, err
	}
	return ops.Op{
		Type:      ops.OpClaim,
		TargetID:  issueID,
		Timestamp: ts,
		WorkerID:  workerID,
		Payload: ops.Payload{
			TTL:          ttl,
			WorktreePath: worktreePath,
			ClaimToken:   token,
		},
	}, nil
}

// LeaseFromClocks builds a Lease from materialized (or prior-lease) clocks.
func LeaseFromClocks(status, holder, token string, claimedAt, lastHeartbeat, claimingWorkerActivity int64, ttl int, worktreePath string) Lease {
	return Lease{
		Holder:       holder,
		Token:        token,
		Since:        claimedAt,
		LastActivity: int64(FoldLastActivity(claimedAt, lastHeartbeat, claimingWorkerActivity)),
		TTLMinutes:   ttl,
		WorktreePath: worktreePath,
		Status:       status,
	}
}

// LeaseLive reports whether the holder is still inside the TTL window at now.
// Unheld leases are not live. Exact boundary now == last+ttl is takeable.
func LeaseLive(l Lease, now int64) bool {
	if l.Holder == "" {
		return false
	}
	return !IsClaimStale(LastActivity(l.LastActivity), l.TTLMinutes, now)
}

// Accept is the single steal-on-stale step. Same holder always replaces.
// A foreign claim takes only when the held lease is not live at claimOp.Timestamp.
func Accept(held Lease, claimOp ops.Op) (Lease, bool) {
	if claimOp.Type != ops.OpClaim {
		return held, false
	}
	if foreignLiveLeaseBlocks(held, claimOp.WorkerID, claimOp.Timestamp) {
		return held, false
	}
	return leaseFromClaimOp(claimOp), true
}

func foreignLiveLeaseBlocks(held Lease, challengerID string, at int64) bool {
	if held.Status != ops.StatusClaimed && held.Status != ops.StatusInProgress {
		return false
	}
	if held.Holder == "" || held.Holder == challengerID {
		return false
	}
	return LeaseLive(held, at)
}

func leaseFromClaimOp(op ops.Op) Lease {
	return Lease{
		Holder:       op.WorkerID,
		Token:        op.Payload.ClaimToken,
		Since:        op.Timestamp,
		LastActivity: op.Timestamp,
		TTLMinutes:   op.Payload.TTL,
		WorktreePath: op.Payload.WorktreePath,
		Status:       ops.StatusClaimed,
	}
}

// Apply folds one op into a lease. Owner is a loop of Apply.
func Apply(held Lease, op ops.Op) Lease {
	switch op.Type {
	case ops.OpClaim:
		next, _ := Accept(held, op)
		return next
	case ops.OpHeartbeat:
		return ApplyHeartbeat(held, op)
	case ops.OpTransition:
		return ApplyTransition(held, op)
	default:
		return held
	}
}

// ApplyHeartbeat extends LastActivity only for the current holder.
func ApplyHeartbeat(held Lease, op ops.Op) Lease {
	if !ClaimantHeartbeatClocks(held.Holder, op.WorkerID) {
		return held
	}
	if op.Timestamp > held.LastActivity {
		held.LastActivity = op.Timestamp
	}
	return held
}

// ApplyTransition mirrors materialize's lease-relevant transition fold.
func ApplyTransition(held Lease, op ops.Op) Lease {
	if op.Payload.IfClaimToken != "" && !heldByExactWorkerAndClaimToken(held, op.WorkerID, op.Payload.IfClaimToken) {
		return held
	}
	wasClaimant := op.WorkerID == held.Holder
	if op.Payload.To == ops.StatusOpen {
		held.Holder = ""
		held.Token = ""
		held.Since = 0
	}
	if wasClaimant && op.Timestamp > held.LastActivity {
		held.LastActivity = op.Timestamp
	}
	if op.Payload.To != "" {
		held.Status = op.Payload.To
	}
	held.WorktreePath = ops.DecodeWorktreeRestore(op.Payload).Apply(held.WorktreePath)
	if op.Payload.RestoreClaim {
		held.Holder = op.Payload.RestoreClaimedBy
		held.Since = op.Payload.RestoreClaimedAt
		held.TTLMinutes = op.Payload.RestoreClaimTTL
		held.Token = op.Payload.RestoreClaimToken
		held.LastActivity = int64(FoldLastActivity(
			op.Payload.RestoreClaimedAt,
			op.Payload.RestoreLastHeartbeat,
			op.Payload.RestoreLastClaimingWorkerActivity,
		))
	}
	return held
}

func heldByExactWorkerAndClaimToken(l Lease, workerID, claimToken string) bool {
	if claimToken == "" {
		return false
	}
	return l.Status == ops.StatusClaimed && l.Holder == workerID && l.Token == claimToken
}

// Owner folds claim, heartbeat, and claimant-transition ops for issueID in
// the same order as materialize.ApplyOpsSorted.
func Owner(log []ops.Op, issueID string) Lease {
	ordered := append([]ops.Op(nil), log...)
	SortForReplay(ordered)
	var held Lease
	for _, op := range ordered {
		if op.TargetID != issueID {
			continue
		}
		held = Apply(held, op)
	}
	return held
}

// SortForReplay matches materialize's timestamp-then-type-key stable sort.
func SortForReplay(allOps []ops.Op) {
	slices.SortStableFunc(allOps, func(a, b ops.Op) int {
		if n := cmp.Compare(a.Timestamp, b.Timestamp); n != 0 {
			return n
		}
		return cmp.Compare(opSortKey(a), opSortKey(b))
	})
}

func opSortKey(op ops.Op) int {
	switch op.Type {
	case ops.OpCreate:
		return 0
	case ops.OpNoteDelete:
		return 2
	default:
		return 1
	}
}

// LostRaceClaimKeys marks claim ops that Accept rejected (took == false).
func LostRaceClaimKeys(log []ops.Op) map[string]bool {
	ordered := append([]ops.Op(nil), log...)
	SortForReplay(ordered)
	heldByIssue := map[string]Lease{}
	lost := map[string]bool{}
	for _, op := range ordered {
		if op.Type != ops.OpClaim {
			heldByIssue[op.TargetID] = Apply(heldByIssue[op.TargetID], op)
			continue
		}
		held := heldByIssue[op.TargetID]
		next, took := Accept(held, op)
		if !took {
			lost[op.TargetID+"|"+op.WorkerID] = true
		}
		heldByIssue[op.TargetID] = next
	}
	return lost
}
