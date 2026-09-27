//go:build ignore

// Claim-order + cattle-worker sketch (architect Phase B).
// Not part of the Go module build (ignore tag). Panic bodies only.
//
// Caller usage is the spec; types follow. Assumes claim-ttl
// Accept / Owner / LeaseLive have landed in internal/claim.

package claimorder

import "github.com/scullxbones/armature/internal/ops"

// Lease is claim-ttl's claim.Lease once that PR lands. Duplicated here so
// this ignore-tagged sketch does not import a type that is not on main.
type Lease struct {
	Holder, Token string
	Since         int64
	LastActivity  int64
	TTLMinutes    int
	StatusAtHold  string
}

// ---------------------------------------------------------------------------
// Caller usage (cmd / dispatcher) — derive types from this, not the reverse.
// ---------------------------------------------------------------------------

func usageClaim(published []LocatedOp, pending []ops.Op, issueID, workerID, token string, now int64) {
	held := OwnerPublished(published, issueID, now)
	if held.Token == token && held.Holder == workerID {
		_ = "provision worktree; this clone published and won"
		return
	}
	if TokenPending(pending, issueID, token) {
		panic("CLAIM-NOT-PUBLISHED")
	}
	panic("lost_claim_race")
}

func usageOwnerVerb(published []LocatedOp, issueID, workerID, token string, now int64) {
	if err := RequireOwner(published, issueID, workerID, token, now); err != nil {
		panic(err.Error()) // NOT-CLAIM-OWNER
	}
	_ = "append heartbeat or transition; if heartbeat renews LeaseLive, publish now"
}

func usageIdentity(repoPath string, env map[string]string) {
	id, err := ResolveIdentity(IdentityInput{
		RepoPath:     repoPath,
		EnvWorkerID:  env["ARM_WORKER_ID"],
		EnvLogSlot:   env["ARM_LOG_SLOT"],
		GitConfigID:  "", // filled by worker.GetWorkerID inside real impl
		GitConfigSet: false,
	})
	if err != nil {
		panic(err.Error())
	}
	unlock, err := LockLogPath(id.LogPath)
	if err != nil {
		panic("LOG-SLOT-COLLISION")
	}
	defer unlock()
	_ = id.ID
}

func usageReplay(opsWT string, cp Checkpoint) []LocatedOp {
	located, _ := LocateOps(LocateInput{
		OpsWorktree: opsWT,
		FromCommit:  cp.LastMaterializedCommit,
		Cutover:     cp.CutoverCommit,
	})
	return located
}

// ---------------------------------------------------------------------------
// Located ops: git commit order recovered without rewriting JSONL.
// ---------------------------------------------------------------------------

// Seq is the total order every clone agrees on after fetch: introducing
// commit rank on origin/_armature, then byte/line in that commit.
// Pre-cutover ops use epoch 0 + worker timestamp (claim-ttl grandfather).
type Seq struct {
	Epoch     int    // 0 = pre-cutover timestamp order; 1 = commit order
	CommitN   int64  // 0 for epoch 0
	Line      int    // stable within a commit
	Timestamp int64  // op.Timestamp; epoch 0 sort key / debug
	Filename  string // tie-break for epoch 0 (ReadDir name)
}

type LocatedOp struct {
	Op            ops.Op
	Seq           Seq
	CommitSHA     string
	CommitterUnix int64 // git committer date of introducing commit
	Published     bool  // introducing commit reachable from origin/_armature
}

type Checkpoint struct {
	LastMaterializedCommit string
	CutoverCommit          string
	ByteOffsets            map[string]int64
}

type LocateInput struct {
	OpsWorktree string
	FromCommit  string
	Cutover     string
}

func LocateOps(in LocateInput) ([]LocatedOp, error) {
	panic("not implemented")
}

func SortLocated(ops []LocatedOp) {
	panic("not implemented")
}

// ---------------------------------------------------------------------------
// Ownership: claim-ttl Accept/Owner over published located ops.
// ---------------------------------------------------------------------------

func OwnerPublished(published []LocatedOp, issueID string, now int64) Lease {
	panic("not implemented")
}

func AcceptLocated(held Lease, claimOp LocatedOp) (next Lease, took bool) {
	panic("not implemented")
}

func TokenPending(pending []ops.Op, issueID, token string) bool {
	panic("not implemented")
}

type OwnerError string

func (e OwnerError) Error() string { return string(e) }

const (
	ErrNotClaimOwner      OwnerError = "NOT-CLAIM-OWNER"
	ErrClaimNotPublished  OwnerError = "CLAIM-NOT-PUBLISHED"
	ErrLostClaimRace      OwnerError = "lost_claim_race"
	ErrLogSlotCollision   OwnerError = "LOG-SLOT-COLLISION"
	ErrWorkerIDCollision  OwnerError = "WORKER-COLLISION"
	ErrWorkerIDMissing    OwnerError = "WORKER-ID-MISSING"
)

func RequireOwner(published []LocatedOp, issueID, workerID, token string, now int64) error {
	panic("not implemented")
}

// LeaseLivePublished is claim.LeaseLive using last *published* activity
// and now = challenger committer time (fold) or observer now (ready/doctor).
func LeaseLivePublished(l Lease, now int64) bool {
	panic("not implemented")
}

// ---------------------------------------------------------------------------
// Cattle identity. Opaque ID string; claim package does not parse it.
// ---------------------------------------------------------------------------

type Identity struct {
	ID      string
	LogPath string
	Source  string // "env" | "git-config" | "env+slot"
}

type IdentityInput struct {
	RepoPath     string
	IssuesDir    string
	EnvWorkerID  string
	EnvLogSlot   string
	GitConfigID  string
	GitConfigSet bool
}

func ResolveIdentity(in IdentityInput) (Identity, error) {
	panic("not implemented")
}

func LockLogPath(logPath string) (unlock func(), err error) {
	panic("not implemented")
}
