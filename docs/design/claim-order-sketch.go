//go:build ignore

// Claim-order + cattle-worker sketch (architect Phase B, revision 1).
// Not part of the Go module build (ignore tag). Panic bodies only.
//
// Caller usage is the spec; types follow. Assumes claim-ttl
// Accept / Owner / LeaseLive have landed in internal/claim.
// Identity: worktree git config beats inherited ARM_WORKER_ID.

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
// Caller usage (coordinator + cmd) — derive types from this, not the reverse.
// ---------------------------------------------------------------------------

func usageCoordinatorDispatch(mainRepo, worktreePath, issueID string) {
	_ = EnableWorktreeConfig(worktreePath)
	id, err := InitWorker(InitWorkerInput{
		RepoPath: worktreePath,
		ID:       "", // generate UUID; never copy clone-level baked id
		Reuse:    false,
	})
	if err != nil {
		panic(err.Error()) // WORKER-ID-IN-USE if --id collides with a published log
	}
	_ = id
	_ = issueID
	_ = mainRepo
	// spawn subagent with cwd=worktreePath; it may inherit ARM_WORKER_ID
}

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

func usageIdentity(repoPath string, flagID string, env map[string]string) {
	id, err := ResolveIdentity(IdentityInput{
		RepoPath:           repoPath,
		FlagWorkerID:       flagID,
		EnvWorkerID:        env["ARM_WORKER_ID"],
		EnvLogSlot:         env["ARM_LOG_SLOT"],
		WorktreeGitConfigID: "", // git config --worktree armature.worker-id
		CloneGitConfigID:   "", // git config --local armature.worker-id
	})
	if err != nil {
		panic(err.Error()) // WORKER-ID-INVALID, LOG-SLOT-INVALID, WORKER-ID-MISSING
	}
	unlock, err := LockLogID(repoPath, id.ID)
	if err != nil {
		panic("LOG-SLOT-COLLISION")
	}
	defer unlock()
	_ = id.LogPath
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
// Ownership: claim-ttl Accept/Owner over published located ops. Unchanged in
// revision 1 — WorkerID is an opaque string.
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
	ErrNotClaimOwner     OwnerError = "NOT-CLAIM-OWNER"
	ErrClaimNotPublished OwnerError = "CLAIM-NOT-PUBLISHED"
	ErrLostClaimRace     OwnerError = "lost_claim_race"
	ErrLogSlotCollision  OwnerError = "LOG-SLOT-COLLISION"
	ErrWorkerIDInvalid   OwnerError = "WORKER-ID-INVALID"
	ErrLogSlotInvalid    OwnerError = "LOG-SLOT-INVALID"
	ErrWorkerIDInUse     OwnerError = "WORKER-ID-IN-USE"
	ErrWorkerIDMissing   OwnerError = "WORKER-ID-MISSING"
)

func RequireOwner(published []LocatedOp, issueID, workerID, token string, now int64) error {
	panic("not implemented")
}

func LeaseLivePublished(l Lease, now int64) bool {
	panic("not implemented")
}

// ---------------------------------------------------------------------------
// Identity (revision 1). Opaque ID; claim package does not parse it.
// Resolve order: flag > worktree git config > env > clone git config.
// ---------------------------------------------------------------------------

type Identity struct {
	ID      string
	LogPath string
	Source  string // "flag" | "worktree-config" | "env" | "clone-config"
}

type IdentityInput struct {
	RepoPath            string
	IssuesDir           string
	FlagWorkerID        string
	EnvWorkerID         string
	EnvLogSlot          string
	WorktreeGitConfigID string
	CloneGitConfigID    string
}

// MaxWorkerIDLen is the filename cap (APFS case-insensitive + git).
const MaxWorkerIDLen = 64

func ValidateWorkerID(id string) error {
	panic("not implemented")
}

func ResolveIdentity(in IdentityInput) (Identity, error) {
	panic("not implemented")
}

type InitWorkerInput struct {
	RepoPath string
	ID       string // empty → generate lowercase UUID
	Reuse    bool   // allow --id that already has a published log
}

func EnableWorktreeConfig(repoPath string) error {
	panic("not implemented")
}

func InitWorker(in InitWorkerInput) (Identity, error) {
	panic("not implemented")
}

// LockLogID flocks <git-common-dir>/armature-log-<id>.lock so two linked
// worktrees of one clone that share an id collide. Cross-clone: no-op.
func LockLogID(repoPath, workerID string) (unlock func(), err error) {
	panic("not implemented")
}
