package claim

// OwnerError is a named ownership / identity error (design sketch § errors).
type OwnerError string

func (e OwnerError) Error() string { return string(e) }

const (
	ErrNotClaimOwner    OwnerError = "NOT-CLAIM-OWNER"
	ErrLogSlotCollision OwnerError = "LOG-SLOT-COLLISION"
	ErrWorkerIDInvalid  OwnerError = "WORKER-ID-INVALID"
	ErrLogSlotInvalid   OwnerError = "LOG-SLOT-INVALID"
	ErrWorkerIDInUse    OwnerError = "WORKER-ID-IN-USE"
	ErrWorkerIDMissing  OwnerError = "WORKER-ID-MISSING"
)
