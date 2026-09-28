package claim

import "github.com/scullxbones/armature/internal/ops"

// OwnerError is a named ownership / identity error (design sketch § errors).
type OwnerError string

func (e OwnerError) Error() string { return string(e) }

const (
	ErrNotClaimOwner     OwnerError = "NOT-CLAIM-OWNER"
	ErrClaimNotPublished OwnerError = "CLAIM-NOT-PUBLISHED"
	ErrLostClaimRace     OwnerError = "lost_claim_race"
	ErrLogSlotCollision  OwnerError = "LOG-SLOT-COLLISION"
	ErrWorkerIDInvalid   OwnerError = "WORKER-ID-INVALID"
	ErrLogSlotInvalid    OwnerError = "LOG-SLOT-INVALID"
	ErrWorkerIDInUse     OwnerError = "WORKER-ID-IN-USE"
	ErrWorkerIDMissing   OwnerError = "WORKER-ID-MISSING"
)

// OwnerPublished folds claim-ttl Owner over ops that are already the published prefix.
func OwnerPublished(published []ops.Op, issueID string) Lease {
	return Owner(published, issueID)
}

// TokenPending reports whether pending (unpublished) ops include this claim token.
func TokenPending(pending []ops.Op, issueID, token string) bool {
	if token == "" {
		return false
	}
	for _, op := range pending {
		if op.Type == ops.OpClaim && op.TargetID == issueID && op.Payload.ClaimToken == token {
			return true
		}
	}
	return false
}
