//go:build ignore

// Claim/TTL architecture sketches. Not part of the Go module.
// Companion: /opt/cursor/artifacts/claim-ttl-architecture.md
//
// Bodies are deliberately unimplemented. Names are the design, not a PR.

package claimttl

import (
	"time"

	"github.com/scullxbones/armature/internal/ops"
)

// ---------------------------------------------------------------------------
// Candidate A — keep current shape, accident-only
// Law remains incremental applyClaim. Callers must not re-derive it.
// ---------------------------------------------------------------------------

// A_LeaseLive is today's IsClaimStale inverted, with no 60-minute rewrite
// for TTL<=0 (--ttl 0 never expires; config default_ttl 0 stays D10).
func A_LeaseLive(claimedAt, lastHeartbeat, claimingWorkerActivity int64, ttlMinutes int, now int64) bool {
	panic("design sketch: not implemented")
}

// A_AcceptClaim is today's applyClaim decision extracted so audit/workers
// cannot call ResolveClaim. Same worker always takes; foreign takes iff
// !A_LeaseLive(held, claim.Timestamp).
func A_AcceptClaim(held A_Held, claim ops.Op) (took bool) {
	panic("design sketch: not implemented")
}

type A_Held struct {
	Status                     string
	ClaimedBy                  string
	ClaimedAt                  int64
	LastHeartbeat              int64
	LastClaimingWorkerActivity int64
	TTLMinutes                 int
	ClaimToken                 string
}

// Deleted in A: ResolveClaim as a production winner.
// Deleted in A: zeroTTLHeldLeaseReplayFallbackMinutes.
// Deleted in A: cmd/armature ready TUI private OpClaim append.
// Unchanged in A: clone flock, high-stakes pushOpsBranch, I2 keep-local,
// PlanClaim overlap, PlanCompensation, HeldByExactWorkerAndClaimToken
// StatusClaimed gate, Arena B parked, D11 reserved.

// ---------------------------------------------------------------------------
// Candidate B (recommended) — one ownership function at read time
// Issue claim fields are a cache of Owner after the last applied op.
// ---------------------------------------------------------------------------

// Lease is the only claim-ownership value cmd, ready, doctor, audit, and
// materialize are allowed to consult.
type Lease struct {
	Holder       string // empty ⇒ unheld
	Token        string
	Since        int64 // claim op timestamp
	LastActivity int64 // FoldLastActivity
	TTLMinutes   int   // 0 ⇒ never expires (recorded --ttl 0)
	WorktreePath string
}

func (l Lease) Live(now int64) bool {
	panic("design sketch: not implemented")
}

// Accept is the single steal-on-stale step. applyClaim must call this and
// write Issue fields from the returned Lease when took.
//
// Rules (normative for B, matching post-accident Law M):
//   - empty claim.WorkerID is an input error at the command layer, not here
//   - same Holder always takes (re-claim / new token)
//   - foreign takes only when !held.Live(claim.Timestamp)
//   - recorded TTL 0 is never stale
func Accept(held Lease, claim ops.Op) (next Lease, took bool) {
	panic("design sketch: not implemented")
}

// Owner folds every claim/heartbeat/claimant-transition for issueID in
// timestamp order (same sort as ApplyOpsSorted) by calling Accept / clock
// updates. now is only for Live() at the end; steal uses each op's timestamp.
func Owner(log []ops.Op, issueID string, now int64) Lease {
	panic("design sketch: not implemented")
}

// ReportClaimOutcome is what arm claim uses after append+publish.
// Lost means Owner's Token != appended token (or Holder != worker).
// Publish failure is a different error, already mapped to CLAIM-1; this
// type is not used for that.
type ClaimOutcome struct {
	IssueID string
	Won     bool
	Lease   Lease
	Reason  string // "", "lost_claim_race", "superseded_by_same_worker"
}

func ReportClaimOutcome(log []ops.Op, issueID, workerID, token string, now int64) ClaimOutcome {
	panic("design sketch: not implemented")
}

// Module map (B):
//
//	internal/claim     Owner, Accept, Lease.Live, PlanClaim, PlanCompensation,
//	                   ShouldHeartbeat (5m debounce stays here)
//	internal/materialize  ApplyOp claim/heartbeat/transition; stores Lease fields
//	cmd/armature          flock+exclude+worktree I/O; appendHighStakesOp;
//	                      no clock folds
//	internal/audit        lost-race marks via Accept over prefixes
//	internal/ops          unchanged JSONL schema
//	internal/doctor       D2 uses Lease.Live; D11 still reserved
//
// Deleted: ResolveClaim, claimWinnersByIssue, hookFindActiveClaimID fold,
// ForeignLiveLeaseBlocksChallenger 60-minute fallback.
//
// Callers:
//
//	arm claim:  PlanClaim → append → push → ReportClaimOutcome → provision?
//	arm ready:  ComputeReady uses Owner/Live, not a private stale predicate
//	materialize: next, took := Accept(held, op)
//	doctor:     !Owner(log, id, now).Live()

func B_ApplyHeartbeat(held Lease, hb ops.Op) Lease {
	panic("design sketch: not implemented")
}

func B_ApplyClaimantTransition(held Lease, tr ops.Op) Lease {
	panic("design sketch: not implemented")
}

// ---------------------------------------------------------------------------
// Candidate C — explicit expiry (liveness is written)
// Challenger cannot steal until an expire (or open transition) is in the log.
// ---------------------------------------------------------------------------

const (
	OpRenew  = "renew"  // optional; heartbeat may carry ExpiresAt instead
	OpExpire = "expire" // doctor or arm expire; not a daemon
)

type WireLease struct {
	Holder     string
	Token      string
	ExpiresAt  int64 // 0 ⇒ never; doctor still must not invent config 0
	ClaimedAt  int64
}

func C_ApplyClaim(held WireLease, claim ops.Op) (WireLease, bool) {
	panic("design sketch: not implemented")
}

func C_ApplyRenew(held WireLease, renew ops.Op) WireLease {
	panic("design sketch: not implemented")
}

func C_ApplyExpire(held WireLease, expire ops.Op) WireLease {
	panic("design sketch: not implemented")
}

// C_PlanExpire is the only now-using function. Doctor --fix calls it and
// appends expire/open transitions. Materialize and ready do not take now.
func C_PlanExpire(held WireLease, now int64) (ops.Op, bool) {
	panic("design sketch: not implemented")
}

// Migration: append expire ops for currently stale leases; do not rewrite
// historical claim lines. Until expire is published, other clones still see
// a holder. Reclaim is blocked on that second write.

// ---------------------------------------------------------------------------
// Shared publish contract (not a candidate; already shipped in #244/#272)
// ---------------------------------------------------------------------------

// AfterHighStakesCommit: Push; on error FetchAndRebase+Push; remaining error
// fails the CLI; local commit kept. Low-stakes: best-effort. No sketch types
// — do not replace opsync or invent arm opsync (ADR 0011).

type PublishClass int

const (
	PublishOK PublishClass = iota
	PublishAuth
	PublishNonFastForward
	PublishOther
)

func KeepLocalCommitOnPublishFailure() {
	panic("design sketch: not implemented")
}

// Clone flock (not a winner): TryLock armature-claim-<id>.lock in git dir.
func CloneClaimMutex(repoPath, issueID string) (unlock func(), err error) {
	panic("design sketch: not implemented")
}

// HeartbeatDebounce remains 5*time.Minute, independent of TTL.
var HeartbeatDebounce = 5 * time.Minute
