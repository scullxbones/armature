package claim

import "github.com/scullxbones/armature/internal/ops"

// HeldClaim is the replay-visible lease ForeignLiveLeaseBlocksChallenger inspects.
type HeldClaim struct {
	Status                     string
	ClaimedBy                  string
	ClaimedAt                  int64
	LastHeartbeat              int64
	LastClaimingWorkerActivity int64
	TTLMinutes                 int
}

// ForeignLiveLeaseBlocksChallenger reports whether a challenger's claim op is a no-op because
// another worker still holds a live claimed or in-progress lease.
func ForeignLiveLeaseBlocksChallenger(held HeldClaim, challengerID string, now int64) bool {
	_, took := Accept(LeaseFromClocks(
		held.Status, held.ClaimedBy, "",
		held.ClaimedAt, held.LastHeartbeat, held.LastClaimingWorkerActivity,
		held.TTLMinutes, "",
	), ops.Op{Type: ops.OpClaim, WorkerID: challengerID, Timestamp: now})
	return !took
}

// ClaimantHeartbeatClocks reports whether workerID may advance LastHeartbeat
// and LastClaimingWorkerActivity. Updated is always advanced by applyHeartbeat.
func ClaimantHeartbeatClocks(claimedBy, workerID string) bool {
	return workerID == claimedBy
}
