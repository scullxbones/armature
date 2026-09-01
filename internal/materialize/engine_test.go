package materialize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCreateOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Fix auth", Parent: "story-01", NodeType: "task",
			Scope: []string{"src/auth/**"}, DefinitionOfDone: "Tests pass"},
	}
	require.NoError(t, state.ApplyOp(op))
	issue := state.Issues["task-01"]
	assert.Equal(t, "task-01", issue.ID)
	assert.Equal(t, "open", issue.Status)
	assert.Equal(t, "Fix auth", issue.Title)
	assert.Equal(t, "story-01", issue.Parent)
}

func TestApplyClaimOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	issue := state.Issues["task-01"]
	assert.Equal(t, "claimed", issue.Status)
	assert.Equal(t, "w1", issue.ClaimedBy)
	assert.Equal(t, int64(200), issue.ClaimedAt)
}

// TestApplyClaimOp_SetsClaimToken verifies applyClaim materializes the
// winning claim op's ClaimToken onto the issue, which is what lets a later
// compensating transition (IfClaimToken) name the exact claim it targets.
func TestApplyClaimOp_SetsClaimToken(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60, ClaimToken: "token-a"}}))
	assert.Equal(t, "token-a", state.Issues["task-01"].ClaimToken)
}

// TestApplyTransition_IfClaimTokenMismatchIsNoOp_REQ_LNGHZN_S5_T9 is the core
// replay-time guarantee this task adds: a compensating rollback op whose
// IfClaimToken no longer matches the issue's current ClaimToken must be a
// deterministic no-op, regardless of WHERE in the append-only log it lands
// relative to the claim that superseded it. This test applies the superseding
// claim (worker-b, a fresh token) BEFORE the stale rollback (worker-a,
// compensating for its own now-superseded token) — the op order a naive
// "check-then-append" race would produce — and asserts worker-b's claim
// survives untouched.
func TestApplyTransition_IfClaimTokenMismatchIsNoOp_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "worker-a", Payload: ops.Payload{TTL: 60, ClaimToken: "token-a"}}))
	// worker-b takes over well past worker-a's TTL (applyClaim treats
	// worker-a's claim as stale), landing in the log BEFORE worker-a's
	// rollback below.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200 + 60*60 + 1,
		WorkerID: "worker-b", Payload: ops.Payload{TTL: 60, ClaimToken: "token-b"}}))

	// worker-a's stale compensating rollback, naming its own (now superseded)
	// token. This is the op a naive implementation would append unconditionally.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 200 + 60*60 + 2,
		WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusOpen, IfClaimToken: "token-a"}}))

	issue := state.Issues["task-01"]
	assert.Equal(t, "worker-b", issue.ClaimedBy, "worker-b's claim must survive a stale rollback for a different token")
	assert.Equal(t, "token-b", issue.ClaimToken)
	assert.Equal(t, ops.StatusClaimed, issue.Status, "the stale rollback must not have transitioned the issue back to open")
}

// TestApplyTransition_IfClaimTokenMismatchIsNoOp_ReverseOrder_REQ_LNGHZN_S5_T9
// is the same scenario as the test above but with the ops applied in the
// OPPOSITE order (rollback first, then the superseding claim) to prove the
// guarantee genuinely does not depend on op ordering: whichever op is applied
// second, the final materialized state must be identical (worker-b owns the
// claim). This is the property that makes rollback correctness a replay-time
// concern rather than an append-time race.
func TestApplyTransition_IfClaimTokenMismatchIsNoOp_ReverseOrder_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "worker-a", Payload: ops.Payload{TTL: 60, ClaimToken: "token-a"}}))

	// This time the stale rollback is applied FIRST.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 200 + 60*60 + 2,
		WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusOpen, IfClaimToken: "token-a"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200 + 60*60 + 1,
		WorkerID: "worker-b", Payload: ops.Payload{TTL: 60, ClaimToken: "token-b"}}))

	issue := state.Issues["task-01"]
	assert.Equal(t, "worker-b", issue.ClaimedBy)
	assert.Equal(t, "token-b", issue.ClaimToken)
	assert.Equal(t, ops.StatusClaimed, issue.Status)
}

// TestApplyTransition_IfClaimTokenMatchAppliesAsBefore_REQ_LNGHZN_S5_T9
// verifies the normal (non-superseded) path still works: a compensating
// rollback whose IfClaimToken matches the issue's current ClaimToken, from
// the still-current claimant, applies exactly as an unconditional rollback
// would have.
func TestApplyTransition_IfClaimTokenMatchAppliesAsBefore_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "worker-a", Payload: ops.Payload{TTL: 60, ClaimToken: "token-a"}}))

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusOpen, IfClaimToken: "token-a"}}))

	issue := state.Issues["task-01"]
	assert.Equal(t, ops.StatusOpen, issue.Status)
	assert.Equal(t, "", issue.ClaimedBy)
	assert.Equal(t, "", issue.ClaimToken)
}

// TestApplyTransition_IfClaimTokenAgainstTerminalIssueIsNoOp_REQ_LNGHZN_S5_T9
// verifies a conditional rollback can never reopen a done/merged/cancelled
// issue, even if by some (impossible in practice, but not assumed away)
// coincidence its ClaimToken and ClaimedBy still matched: terminal status is
// checked first and unconditionally blocks the op.
func TestApplyTransition_IfClaimTokenAgainstTerminalIssueIsNoOp_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	for _, terminal := range []string{ops.StatusDone, ops.StatusMerged, ops.StatusCancelled} {
		t.Run(terminal, func(t *testing.T) {
			t.Parallel()
			state := NewState()
			require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
			require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
				WorkerID: "worker-a", Payload: ops.Payload{TTL: 60, ClaimToken: "token-a"}}))
			require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 250,
				WorkerID: "worker-a", Payload: ops.Payload{To: terminal}}))
			require.Equal(t, terminal, state.Issues["task-01"].Status)

			// A stale rollback arrives after the issue has already reached a
			// terminal state (e.g. worker-a's provisioning failure handler runs
			// late, after the issue was independently completed and merged).
			require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
				WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusOpen, IfClaimToken: "token-a"}}))

			issue := state.Issues["task-01"]
			assert.Equal(t, terminal, issue.Status, "a conditional rollback must never reopen a terminal issue")
			assert.Equal(t, "worker-a", issue.ClaimedBy)
		})
	}
}

// TestApplyTransition_IfClaimTokenAgainstLiveNonTerminalTransitionIsNoOp_REQ_LNGHZN_S5_T9
// is the non-terminal sibling of the terminal-state test above, and is the
// direct regression test for the PR #95 root cause: claim-owning commands do
// not hold the per-issue claim lock against transition commands
// (acquireClaimLock has exactly one caller, in cmd/armature/claim.go), so a
// concurrent `claimed -> in-progress` or `claimed -> blocked` transition can
// land between a claim and its own compensating rollback. Before
// Issue.ClaimHeldBy existed, applyTransition's IfClaimToken guard checked
// only for a TERMINAL status (done/merged/cancelled) plus a raw
// ClaimToken/ClaimedBy comparison — so a rollback whose token and claimant
// still matched sailed through even though the issue had moved on to a live,
// non-terminal status via a different command. That let a stale claim's
// rollback silently revert workflow progress a newer, non-claim command had
// already made. ClaimHeldBy's Status == StatusClaimed requirement closes
// this: any non-claimed status — terminal OR live — makes the rollback a
// deterministic no-op, leaving status and claimant untouched.
func TestApplyTransition_IfClaimTokenAgainstLiveNonTerminalTransitionIsNoOp_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	for _, liveStatus := range []string{ops.StatusInProgress, ops.StatusBlocked} {
		t.Run(liveStatus, func(t *testing.T) {
			t.Parallel()
			state := NewState()
			require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
			require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
				WorkerID: "worker-a", Payload: ops.Payload{TTL: 60, ClaimToken: "token-a"}}))
			// A different command (e.g. `arm transition`, which does not hold the
			// claim lock) moves the issue to a live, non-terminal status while
			// worker-a's ClaimedBy/ClaimToken are untouched (only a transition to
			// `open` clears them).
			require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 250,
				WorkerID: "worker-a", Payload: ops.Payload{To: liveStatus}}))
			require.Equal(t, liveStatus, state.Issues["task-01"].Status)
			require.Equal(t, "worker-a", state.Issues["task-01"].ClaimedBy)
			require.Equal(t, "token-a", state.Issues["task-01"].ClaimToken)

			// worker-a's own provisioning-failure rollback arrives afterward,
			// targeting the exact claim (same worker, same token) it was written
			// to compensate for.
			require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
				WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusOpen, IfClaimToken: "token-a"}}))

			issue := state.Issues["task-01"]
			assert.Equal(t, liveStatus, issue.Status, "a conditional rollback must not revert a live non-terminal transition made by another command")
			assert.Equal(t, "worker-a", issue.ClaimedBy, "claimant must be untouched by the no-op rollback")
			assert.Equal(t, "token-a", issue.ClaimToken, "claim token must be untouched by the no-op rollback")
		})
	}
}

// TestApplyTransition_RestoreClaimTokenRestoresPriorToken_REQ_LNGHZN_S5_T9
// verifies RestoreClaimToken is applied alongside the other Restore* lease
// fields, so an active same-worker retry's rollback restores its OWN prior
// claim's token rather than leaving the just-superseded retry's token in place.
func TestApplyTransition_RestoreClaimTokenRestoresPriorToken_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60, ClaimToken: "token-original"}}))
	state.Issues["task-01"].Status = ops.StatusInProgress

	// A same-worker retry claims again (new token), then its provisioning
	// fails and rolls back, restoring the original token via RestoreClaim.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60, ClaimToken: "token-retry"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 400,
		WorkerID: "w1", Payload: ops.Payload{
			To: ops.StatusInProgress, IfClaimToken: "token-retry",
			RestoreClaim: true, RestoreClaimedBy: "w1", RestoreClaimedAt: 200, RestoreClaimToken: "token-original",
		}}))

	assert.Equal(t, "token-original", state.Issues["task-01"].ClaimToken)
}

// TestApplyTransition_RestoresWorktreePathFromPayload_REQ_LNGHZN_S5_T1 verifies
// that a transition op carrying a WorktreePath restores it onto the issue. This
// is the mechanism a claim rollback relies on to put back the pre-claim (legacy)
// worktree path after the canonical path's provisioning failed. Pre-fix,
// applyTransition ignored Payload.WorktreePath entirely, so a rollback could not
// restore it and the active claim was left pointing at the just-removed canonical
// path.
func TestApplyTransition_RestoresWorktreePathFromPayload_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	// Claim overwrites WorktreePath with the canonical path.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60, WorktreePath: "/repo/.worktrees/task-01"}}))
	require.Equal(t, "/repo/.worktrees/task-01", state.Issues["task-01"].WorktreePath)

	// Rollback transition restores the prior (legacy) worktree path while keeping
	// the same worker's active claim.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: ops.StatusInProgress, WorktreePath: "/legacy/path/task-01"}}))
	assert.Equal(t, "/legacy/path/task-01", state.Issues["task-01"].WorktreePath,
		"a transition carrying a WorktreePath must restore it")
	assert.Equal(t, "w1", state.Issues["task-01"].ClaimedBy, "the claim must remain with the same worker")
}

// TestApplyTransition_NoWorktreePathPreservesExisting_REQ_LNGHZN_S5_T1 verifies
// that a normal transition (no WorktreePath in payload) leaves the issue's
// existing WorktreePath untouched, so the restore mechanism never clobbers a
// live path during ordinary status changes.
func TestApplyTransition_NoWorktreePathPreservesExisting_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60, WorktreePath: "/repo/.worktrees/task-01"}}))

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: ops.StatusDone}}))
	assert.Equal(t, "/repo/.worktrees/task-01", state.Issues["task-01"].WorktreePath,
		"a normal transition must not clear the existing WorktreePath")
}

// TestApplyTransition_ClearWorktreePathRestoresEmpty_REQ_LNGHZN_S5_T1 verifies
// that a transition op carrying ClearWorktreePath restores the issue's
// WorktreePath to empty. This is the claim-rollback case where the pre-claim
// path was empty (a first claim with no pre-existing worktree): an empty
// Payload.WorktreePath alone is indistinguishable from "no change", so without
// the explicit clear-signal the rollback would silently leave the issue pointing
// at the just-removed canonical path.
func TestApplyTransition_ClearWorktreePathRestoresEmpty_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	// Claim overwrites the (empty) WorktreePath with the canonical path.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60, WorktreePath: "/repo/.worktrees/task-01"}}))
	require.Equal(t, "/repo/.worktrees/task-01", state.Issues["task-01"].WorktreePath)

	// Rollback restores the pre-claim path, which was empty: ClearWorktreePath
	// must win over the (absent) WorktreePath and zero the field.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: ops.StatusInProgress, ClearWorktreePath: true}}))
	assert.Equal(t, "", state.Issues["task-01"].WorktreePath,
		"ClearWorktreePath must restore the WorktreePath to empty")
}

// TestApplyTransition_RestoresCompleteClaimSnapshot_REQ_LNGHZN_S5_T1 verifies
// that a failed same-worker retry restores the full lease snapshot, not only
// the status and worktree path. The explicit marker makes zero-valued fields
// unambiguous in the append-only compensating transition.
func TestApplyTransition_RestoresCompleteClaimSnapshot_REQ_LNGHZN_S5_T1(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60, WorktreePath: "/legacy/task-01"}}))
	issue := state.Issues["task-01"]
	issue.Status = ops.StatusInProgress
	issue.LastHeartbeat = 240
	issue.LastClaimingWorkerActivity = 250
	issue.ClaimTTL = 90
	before := *issue

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{
			To:                                before.Status,
			WorktreePath:                      before.WorktreePath,
			RestoreClaim:                      true,
			RestoreClaimedBy:                  before.ClaimedBy,
			RestoreClaimedAt:                  before.ClaimedAt,
			RestoreClaimTTL:                   before.ClaimTTL,
			RestoreLastHeartbeat:              before.LastHeartbeat,
			RestoreLastClaimingWorkerActivity: before.LastClaimingWorkerActivity,
		}}))
	after := state.Issues["task-01"]
	assert.Equal(t, before.Status, after.Status)
	assert.Equal(t, before.ClaimedBy, after.ClaimedBy)
	assert.Equal(t, before.ClaimedAt, after.ClaimedAt)
	assert.Equal(t, before.ClaimTTL, after.ClaimTTL)
	assert.Equal(t, before.LastHeartbeat, after.LastHeartbeat)
	assert.Equal(t, before.LastClaimingWorkerActivity, after.LastClaimingWorkerActivity)
	assert.Equal(t, before.WorktreePath, after.WorktreePath)
}

func TestApplyClaimOp_DoesNotOverrideActiveClaimFromDifferentWorker(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "worker-a", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 210,
		WorkerID: "worker-b", Payload: ops.Payload{TTL: 60}}))
	issue := state.Issues["task-01"]
	assert.Equal(t, "claimed", issue.Status)
	assert.Equal(t, "worker-a", issue.ClaimedBy)
	assert.Equal(t, int64(200), issue.ClaimedAt)
}

// TestApplyHeartbeatOp_NonClaimantHeartbeatDoesNotBumpLastHeartbeat verifies
// that a heartbeat op from a worker who is NOT the current claimant does not
// extend LastHeartbeat. LastHeartbeat feeds directly into claim.IsClaimStale /
// doctor's claimExpired staleness formula, so an errant or malicious
// non-claimant heartbeat must not be able to mask a genuinely stale claim
// (PR #84 review: "Ignore non-claimant heartbeats for expiry").
func TestApplyHeartbeatOp_NonClaimantHeartbeatDoesNotBumpLastHeartbeat(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "claimant", Payload: ops.Payload{TTL: 60}}))
	// A different worker (not the claimant) sends a heartbeat for this issue.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpHeartbeat, TargetID: "task-01", Timestamp: 900,
		WorkerID: "other-worker"}))
	issue := state.Issues["task-01"]
	assert.Equal(t, int64(200), issue.LastHeartbeat, "non-claimant heartbeat must not bump LastHeartbeat beyond the claim's own timestamp")
	assert.Equal(t, int64(200), issue.LastClaimingWorkerActivity,
		"non-claimant heartbeat must not bump LastClaimingWorkerActivity beyond the claim's own timestamp")
}

// TestApplyHeartbeatOp_ClaimantHeartbeatBumpsLastHeartbeat is the counterpart:
// a heartbeat from the actual claimant must still extend LastHeartbeat (and
// LastClaimingWorkerActivity), preserving existing behavior for the common case.
func TestApplyHeartbeatOp_ClaimantHeartbeatBumpsLastHeartbeat(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "claimant", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpHeartbeat, TargetID: "task-01", Timestamp: 900,
		WorkerID: "claimant"}))
	issue := state.Issues["task-01"]
	assert.Equal(t, int64(900), issue.LastHeartbeat)
	assert.Equal(t, int64(900), issue.LastClaimingWorkerActivity)
}

// TestApplyTransitionOp_ClaimantTransitionBumpsLastClaimingWorkerActivity verifies
// that a transition op authored by the current claimant bumps LastClaimingWorkerActivity,
// even when the transition is to `open` (which clears ClaimedBy as part of the same
// op) — the claimant's own release-of-claim is still claimant activity and must be
// captured before ClaimedBy is zeroed (PR #84 review: "Reuse claimant activity in
// stale-claim checks").
func TestApplyTransitionOp_ClaimantTransitionBumpsLastClaimingWorkerActivity(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "claimant", Payload: ops.Payload{TTL: 60}}))
	// The claimant transitions their own claim back to open.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 900,
		WorkerID: "claimant", Payload: ops.Payload{To: ops.StatusOpen}}))
	issue := state.Issues["task-01"]
	assert.Equal(t, int64(900), issue.LastClaimingWorkerActivity,
		"claimant's own transition (even to open, which clears ClaimedBy) must bump LastClaimingWorkerActivity")
}

// TestApplyTransitionOp_NonClaimantTransitionDoesNotBumpLastClaimingWorkerActivity
// verifies that a transition op from a worker who is NOT the current claimant does
// not extend LastClaimingWorkerActivity, mirroring the heartbeat non-claimant guard.
func TestApplyTransitionOp_NonClaimantTransitionDoesNotBumpLastClaimingWorkerActivity(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "claimant", Payload: ops.Payload{TTL: 60}}))
	// A different worker (not the claimant) sends a transition for this issue.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 900,
		WorkerID: "other-worker", Payload: ops.Payload{To: ops.StatusInProgress}}))
	issue := state.Issues["task-01"]
	assert.Equal(t, int64(200), issue.LastClaimingWorkerActivity,
		"non-claimant transition must not bump LastClaimingWorkerActivity")
}

func TestApplyUnknownOpType_ReturnsError(t *testing.T) {
	t.Parallel()
	state := NewState()
	err := state.ApplyOp(ops.Op{
		Type:      "worker-runtime-decision",
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  "worker-a",
		Payload:   ops.Payload{Msg: "runtime decision"},
	})
	require.Error(t, err, "unknown op type must return an error")
	assert.Contains(t, err.Error(), "unknown op type")
}

func TestRegisteredOpTypes_ReturnsAllSupportedTypes(t *testing.T) {
	t.Parallel()
	registered := RegisteredOpTypes()

	// Verify that all registered types are non-empty strings
	require.NotEmpty(t, registered)
	for _, opType := range registered {
		assert.NotEmpty(t, opType, "registered op type must not be empty")
	}

	// Verify that the known op types are present
	expectedTypes := []string{
		ops.OpCreate, ops.OpClaim, ops.OpHeartbeat, ops.OpTransition,
		ops.OpNote, ops.OpNoteDelete, ops.OpLink, ops.OpUnlink,
		ops.OpDecision, ops.OpAssign, ops.OpAmend, ops.OpSourceLink,
		ops.OpSourceFingerprint, ops.OpCitationAccepted, ops.OpDAGTransition,
		ops.OpScopeRename, ops.OpScopeDelete, ops.OpReparent, ops.OpAssessmentAttested,
		ops.OpGateEvidence,
	}

	for _, expected := range expectedTypes {
		assert.Contains(t, registered, expected, "op type %q must be in RegisteredOpTypes", expected)
	}
}

// TestRegisteredOpTypes_ManagedExecutionOpsNotRegistered verifies that managed-execution
// op types (heartbeat, orchestrate-*, worker-runtime-decision) are NOT in RegisteredOpTypes,
// and that all standard materialization ops ARE registered. This guards against divergence
// between the ops package constants and the materialize engine's handler map.
func TestRegisteredOpTypes_ManagedExecutionOpsNotRegistered(t *testing.T) {
	t.Parallel()
	registered := RegisteredOpTypes()
	registeredSet := make(map[string]bool, len(registered))
	for _, opType := range registered {
		registeredSet[opType] = true
	}

	// Managed-execution ops must NOT be registered in the materializer.
	managedOps := []string{
		"orchestrate-start",
		"orchestrate-dispatch",
		"orchestrate-dispatch-complete",
		"orchestrate-verify-fail",
		"orchestrate-retry",
		"orchestrate-escalate",
		"orchestrate-complete",
		"orchestrate-check-result",
		"worker-runtime-decision",
	}
	for _, opType := range managedOps {
		assert.False(t, registeredSet[opType], "managed-execution op type %q must NOT be in RegisteredOpTypes", opType)
	}

	// All standard materialization ops must be registered.
	standardOps := []string{
		ops.OpCreate, ops.OpClaim, ops.OpHeartbeat, ops.OpTransition,
		ops.OpNote, ops.OpNoteDelete, ops.OpLink, ops.OpUnlink,
		ops.OpDecision, ops.OpAssign, ops.OpAmend, ops.OpSourceLink,
		ops.OpSourceFingerprint, ops.OpCitationAccepted, ops.OpDAGTransition,
		ops.OpScopeRename, ops.OpScopeDelete, ops.OpReparent,
		ops.OpGateEvidence,
	}
	for _, opType := range standardOps {
		assert.True(t, registeredSet[opType], "standard op type %q must be in RegisteredOpTypes", opType)
	}
}

func TestGenerateSchema_DocumentsEveryRegisteredOpType(t *testing.T) {
	t.Parallel()

	schema := ops.GenerateSchema()
	documented := make(map[string]bool)
	for line := range strings.SplitSeq(schema, "\n") {
		if !strings.HasPrefix(line, "#   ") {
			continue
		}
		opType, _, found := strings.Cut(strings.TrimPrefix(line, "#   "), ":")
		if found {
			documented[opType] = true
		}
	}

	for _, opType := range RegisteredOpTypes() {
		assert.True(t, documented[opType], "schema payload docs must include registered op type %q", opType)
	}
}

func TestApplyTransitionOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "Fixed it"}}))
	issue := state.Issues["task-01"]
	assert.Equal(t, "done", issue.Status)
	assert.Equal(t, "Fixed it", issue.Outcome)
}

func TestApplyNoteOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Msg: "Found edge case"}}))
	assert.Len(t, state.Issues["task-01"].Notes, 1)
	assert.Equal(t, "Found edge case", state.Issues["task-01"].Notes[0].Msg)
}

func TestApplyNoteOp_AutoGeneratedIDCollisionGetsDisambiguatedSuffix(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	// Two notes from the same worker at the same timestamp, neither supplying an
	// explicit NoteID, collide on the auto-generated "note-<ts>-<worker>" base and
	// must be disambiguated rather than silently overwriting one another.
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Msg: "first"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Msg: "second"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Msg: "third"}}))

	notes := state.Issues["task-01"].Notes
	require.Len(t, notes, 3)
	ids := map[string]bool{}
	for _, n := range notes {
		assert.False(t, ids[n.ID], "note IDs must be unique, got duplicate %q", n.ID)
		ids[n.ID] = true
	}
	assert.Equal(t, "note-200-w1", notes[0].ID)
	assert.Equal(t, "note-200-w1-2", notes[1].ID)
	assert.Equal(t, "note-200-w1-3", notes[2].ID)
}

func TestApplyNoteDeleteOp_TombstonesExistingNote(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Msg: "Found edge case", NoteID: "note-1"}}))

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpNoteDelete, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{NoteID: "note-1"}}))

	require.Len(t, state.Issues["task-01"].Notes, 1)
	assert.Equal(t, "note-1", state.Issues["task-01"].Notes[0].ID)
	assert.True(t, state.Issues["task-01"].Notes[0].Deleted)
}

func TestNoteDeleteAtSameTimestampAsAdd_TombstonesViaSort(t *testing.T) {
	t.Parallel()
	// Simulates two workers at the same Unix second: one adds, one deletes.
	// After sortOpsByTimestamp the note-add must precede note-delete so the
	// tombstone is not silently dropped.
	allOps := []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpNoteDelete, TargetID: "task-01", Timestamp: 200, WorkerID: "w2",
			Payload: ops.Payload{NoteID: "note-1"}},
		{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
			Payload: ops.Payload{Msg: "Found edge case", NoteID: "note-1"}},
	}
	sortOpsByTimestamp(allOps)

	state := NewState()
	for _, op := range allOps {
		require.NoError(t, state.ApplyOp(op))
	}

	require.Len(t, state.Issues["task-01"].Notes, 1)
	assert.True(t, state.Issues["task-01"].Notes[0].Deleted, "tombstone from w2 must survive same-tick sort")
}

func TestReplayOpsTolerant_CountsSkippedApplyErrors(t *testing.T) {
	t.Parallel()
	allOps := []ops.Op{
		{Type: ops.OpLink, TargetID: "task-01", Timestamp: 50, WorkerID: "w1",
			Payload: ops.Payload{Dep: "other", Rel: "blocked_by"}},
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
	}
	state, skipped, firstErr := ReplayOpsTolerant(allOps)
	require.NotNil(t, state)
	assert.Equal(t, 1, skipped)
	require.Error(t, firstErr)
	assert.Contains(t, firstErr.Error(), "source issue")
	require.Contains(t, state.Issues, "task-01")
}

func TestApplyLinkOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}}))
	assert.Contains(t, state.Issues["task-01"].BlockedBy, "task-02")
	assert.Contains(t, state.Issues["task-02"].Blocks, "task-01")
}

func TestApplyLinkOp_LegacyNonBlockedByRelIsNoOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}}))

	for _, rel := range []string{"blocks", "depends-on", "relates-to"} {
		err := state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
			WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: rel}})
		require.NoError(t, err, "legacy rel=%q must replay as a no-op", rel)
		assert.NotContains(t, state.Issues["task-01"].BlockedBy, "task-02")
		assert.NotContains(t, state.Issues["task-02"].Blocks, "task-01")
		assert.Equal(t, int64(200), state.Issues["task-01"].Updated, "no-op must retain historical Updated semantics")
	}
}

func TestApplyDecisionOp_LastWriteWins(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpDecision, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Topic: "db", Choice: "postgres", Rationale: "mature"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpDecision, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w2", Payload: ops.Payload{Topic: "db", Choice: "sqlite", Rationale: "simpler"}}))
	decisions := state.Issues["task-01"].Decisions
	active := activeDecisionForTopic(decisions, "db")
	assert.Equal(t, "sqlite", active.Choice)
}

func TestMaterializePipeline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	logPath := filepath.Join(opsDir, "worker-a1.log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "worker-a1", Payload: ops.Payload{Title: "Epic", NodeType: "epic"}}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "worker-a1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "epic-01"}}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "worker-a1", Payload: ops.Payload{TTL: 60}}))

	// Read ops from disk
	allOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	result, err := Materialize(filepath.Join(dir, "state"), allOps, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result.IssueCount)

	assert.FileExists(t, filepath.Join(stateDir, "index.json"))
	assert.FileExists(t, filepath.Join(issuesDir, "task-01.json"))
	assert.FileExists(t, filepath.Join(stateDir, "checkpoint.json"))
}

func TestPropRandomOpsNeverCrash(t *testing.T) {
	t.Parallel()
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 500

	properties := gopter.NewProperties(params)

	opTypeGen := gen.OneConstOf(ops.OpCreate, ops.OpClaim, ops.OpHeartbeat,
		ops.OpTransition, ops.OpNote, ops.OpLink, ops.OpDecision)

	properties.Property("random op sequences never panic", prop.ForAll(
		func(opType string, targetID string, ts int64) bool {
			if targetID == "" {
				return true
			}
			state := NewState()

			_ = state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: targetID, Timestamp: ts, //nolint:errcheck // property test checks for panic, not error correctness
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})

			_ = state.ApplyOp(ops.Op{Type: opType, TargetID: targetID, Timestamp: ts + 1, //nolint:errcheck // property test checks for panic, not error correctness
				WorkerID: "w1", Payload: ops.Payload{TTL: 60, To: "done", Msg: "test",
					Dep: "other", Rel: "blocked_by", Topic: "t", Choice: "c"}})

			return true
		},
		opTypeGen,
		gen.AlphaString(),
		gen.Int64Range(0, 1<<50),
	))

	properties.TestingRun(t)
}

func TestPropCreateIdempotent(t *testing.T) {
	t.Parallel()
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	properties.Property("duplicate creates are idempotent", prop.ForAll(
		func(id string) bool {
			if id == "" {
				return true
			}
			state := NewState()
			op := ops.Op{Type: ops.OpCreate, TargetID: id, Timestamp: 100,
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}

			_ = state.ApplyOp(op) //nolint:errcheck // property test checks for panic, not error correctness
			_ = state.ApplyOp(op) //nolint:errcheck // property test checks for panic, not error correctness

			return len(state.Issues) == 1 && state.Issues[id].Title == "T"
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

func TestApplyCreateOp_DraftConfidence_Propagated(t *testing.T) {
	t.Parallel()
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-draft", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Draft task", NodeType: "task", Confidence: "draft"},
	}
	require.NoError(t, state.ApplyOp(op))
	assert.Equal(t, "draft", state.Issues["task-draft"].Provenance.Confidence)
}

func TestApplyCreateOp_NoConfidence_DefaultsToVerified(t *testing.T) {
	t.Parallel()
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-legacy", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Legacy task", NodeType: "task"},
	}
	require.NoError(t, state.ApplyOp(op))
	assert.Equal(t, "verified", state.Issues["task-legacy"].Provenance.Confidence)
}

func TestApplyCreateOp_VerifiedConfidence_Propagated(t *testing.T) {
	t.Parallel()
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-verified", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Verified task", NodeType: "task", Confidence: "verified"},
	}
	require.NoError(t, state.ApplyOp(op))
	assert.Equal(t, "verified", state.Issues["task-verified"].Provenance.Confidence)
}

func TestApplyDagTransitionOp_PromotesDraftSubtreeToVerified(t *testing.T) {
	t.Parallel()
	state := NewState()
	// Create a root epic with two draft children; one is outside the subtree
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic", NodeType: "epic", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story", Parent: "epic-01", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task under story", NodeType: "task", Parent: "story-01", Confidence: "draft"}}))
	// outside the subtree
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-outside", Timestamp: 103,
		WorkerID: "w1", Payload: ops.Payload{Title: "Outside task", NodeType: "task", Confidence: "draft"}}))

	// Apply dag-transition with IssueID="epic-01"
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpDAGTransition, TargetID: "epic-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{IssueID: "epic-01"},
	}))

	assert.Equal(t, "verified", state.Issues["epic-01"].Provenance.Confidence)
	assert.Equal(t, "verified", state.Issues["story-01"].Provenance.Confidence)
	assert.Equal(t, "verified", state.Issues["task-01"].Provenance.Confidence)
	// outside the subtree is unaffected
	assert.Equal(t, "draft", state.Issues["task-outside"].Provenance.Confidence)
}

func TestApplyDagTransitionOp_CustomTargetConfidence(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task", Confidence: "draft"}}))

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpDAGTransition, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{IssueID: "task-01", To: "verified"},
	}))

	assert.Equal(t, "verified", state.Issues["task-01"].Provenance.Confidence)
}

func TestApplyDagTransitionOp_NodesOutsideSubtreeUnaffected(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-A", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic A", NodeType: "epic", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-B", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic B", NodeType: "epic", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-A1", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task A1", NodeType: "task", Parent: "epic-A", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-B1", Timestamp: 103,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task B1", NodeType: "task", Parent: "epic-B", Confidence: "draft"}}))

	// Promote only epic-A subtree
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpDAGTransition, TargetID: "epic-A", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{IssueID: "epic-A"},
	}))

	assert.Equal(t, "verified", state.Issues["epic-A"].Provenance.Confidence)
	assert.Equal(t, "verified", state.Issues["task-A1"].Provenance.Confidence)
	assert.Equal(t, "draft", state.Issues["epic-B"].Provenance.Confidence)
	assert.Equal(t, "draft", state.Issues["task-B1"].Provenance.Confidence)
}

func TestApplyDagTransitionOp_BackwardCompatExistingConfirmedBehavior(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	assert.False(t, state.Issues["task-01"].Provenance.DAGConfirmed)

	// Old-style op (no IssueID) still sets DAGConfirmed
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpDAGTransition, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Confirmed: true}}))
	assert.True(t, state.Issues["task-01"].Provenance.DAGConfirmed)
}

func TestApplySourceLinkOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpSourceLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{SourceID: "entry-42", SourceURL: "https://example.com/doc", Title: "Ref Doc"}}))
	issue := state.Issues["task-01"]
	require.Len(t, issue.SourceLinks, 1)
	assert.Equal(t, "entry-42", issue.SourceLinks[0].SourceEntryID)
	assert.Equal(t, "https://example.com/doc", issue.SourceLinks[0].SourceURL)
	assert.Equal(t, "Ref Doc", issue.SourceLinks[0].Title)
}

func TestApplyDAGTransitionOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	assert.False(t, state.Issues["task-01"].Provenance.DAGConfirmed)
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpDAGTransition, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Confirmed: true}}))
	assert.True(t, state.Issues["task-01"].Provenance.DAGConfirmed)
}

func TestApplyAssignOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{AssignedTo: "worker-x"}}))
	assert.Equal(t, "worker-x", state.Issues["task-01"].AssignedWorker)
}

func TestApplyUnassignOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{AssignedTo: "worker-x"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{AssignedTo: ""}}))
	assert.Equal(t, "", state.Issues["task-01"].AssignedWorker)
}

func TestApplyAssignOp_ToleratesUnknownIssue(t *testing.T) {
	t.Parallel()
	state := NewState()
	// No create op — assign should not error
	err := state.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "unknown-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{AssignedTo: "worker-x"}})
	assert.NoError(t, err)
}

func TestBuildIndex_IncludesAssignedWorker(t *testing.T) {
	t.Parallel()
	s := NewState()
	s.Issues["T-001"] = &Issue{
		ID: "T-001", Type: "task", Status: "open", Title: "task",
		AssignedWorker: "worker-x",
		Children:       []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	index := s.BuildIndex()
	entry := index["T-001"]
	assert.Equal(t, "worker-x", entry.AssignedWorker)
}

func TestBuildIndex_IncludesBranchAndPR(t *testing.T) {
	t.Parallel()
	s := NewState()
	s.Issues["T-001"] = &Issue{
		ID: "T-001", Type: "task", Status: "done",
		Title: "some task", Branch: "feature/my-work", PR: "42",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}

	index := s.BuildIndex()
	entry, ok := index["T-001"]
	require.True(t, ok)
	assert.Equal(t, "feature/my-work", entry.Branch)
	assert.Equal(t, "42", entry.PR)
}

func TestMaterializeAndReturn_BasicPipeline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	logPath := filepath.Join(opsDir, "worker-b1.log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "worker-b1", Payload: ops.Payload{Title: "My Task", NodeType: "task"}}))

	// Read ops from disk
	allOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	state, result, err := MaterializeAndReturn(filepath.Join(dir, "state"), allOps, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount)
	require.NotNil(t, state)
	assert.Contains(t, state.Issues, "task-01")
	assert.Equal(t, "My Task", state.Issues["task-01"].Title)
}

func TestMaterializeAndReturn_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// No ops dir — should return empty state
	state, result, err := MaterializeAndReturn(filepath.Join(dir, "state"), []ops.Op{}, nil)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, 0, result.IssueCount)
	assert.Equal(t, 0, result.OpsProcessed)
}

func TestAppendUnique_AddsNew(t *testing.T) {
	t.Parallel()
	slice := []string{"a", "b"}
	result := appendUnique(slice, "c")
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestAppendUnique_SkipsDuplicate(t *testing.T) {
	t.Parallel()
	slice := []string{"a", "b", "c"}
	result := appendUnique(slice, "b")
	assert.Equal(t, []string{"a", "b", "c"}, result)
	assert.Len(t, result, 3, "duplicate should not be added")
}

func TestRunRollup_PromotesStoryWhenAllChildrenMerged(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))

	// In dual-branch mode, done must be explicitly promoted to merged
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "done"}}))
	// Manually set to merged since auto-promotion is removed
	state.Issues["task-01"].Status = ops.StatusMerged

	state.RunRollup()
	assert.Equal(t, "merged", state.Issues["story-01"].Status)
}

func TestRunRollup_DoesNotPromoteWithUnmergedChild(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task B", NodeType: "task", Parent: "story-01"}}))

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done"}}))
	// Manually set to merged since auto-promotion is removed
	state.Issues["task-01"].Status = ops.StatusMerged

	state.RunRollup()
	assert.NotEqual(t, "merged", state.Issues["story-01"].Status, "story should not be merged with open task-02")
}

func TestRunRollup_CascadesToEpic(t *testing.T) {
	t.Parallel()
	// epic-01 → story-01 → task-01; when task-01 is merged, both story and epic should cascade-merge.
	// This exercises the parent-decrement path at engine.go:371-380.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic", NodeType: "epic"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story", Parent: "epic-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "story-01"}}))

	// Mark task merged directly (simulating single-branch done → merged)
	state.Issues["task-01"].Status = ops.StatusMerged

	state.RunRollup()
	assert.Equal(t, "merged", state.Issues["story-01"].Status, "story should cascade-merge when all tasks merged")
	assert.Equal(t, "merged", state.Issues["epic-01"].Status, "epic should cascade-merge when all stories merged")
}

func TestRunRollup_PromotesStoryWithCancelledChild_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	// A descoped (cancelled) sibling must not block rollup: cancelled is terminal,
	// so a parent waiting on it can never resolve. Mirrors the merged||cancelled
	// predicate already used by internal/worktree/reconcile.go.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task B", NodeType: "task", Parent: "story-01"}}))

	state.Issues["task-01"].Status = ops.StatusMerged
	state.Issues["task-02"].Status = ops.StatusCancelled

	state.RunRollup()
	assert.Equal(t, "merged", state.Issues["story-01"].Status,
		"story should roll up when every child is merged or cancelled")
}

func TestRunRollup_DoesNotPromoteWhenAllChildrenCancelled_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	// Nothing shipped, so the parent must not claim delivery. At least one merged
	// child is required before a parent rolls up to merged.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task B", NodeType: "task", Parent: "story-01"}}))

	state.Issues["task-01"].Status = ops.StatusCancelled
	state.Issues["task-02"].Status = ops.StatusCancelled

	state.RunRollup()
	assert.NotEqual(t, "merged", state.Issues["story-01"].Status,
		"a wholly-cancelled story must not claim delivery")
}

func TestRunRollup_CascadesThroughCancelledChild_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	// The stranding observed on HKREFACT: an epic whose story rolls up only
	// because a cancelled child no longer blocks it.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic", NodeType: "epic"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story", Parent: "epic-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 103,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task B", NodeType: "task", Parent: "story-01"}}))

	state.Issues["task-01"].Status = ops.StatusMerged
	state.Issues["task-02"].Status = ops.StatusCancelled

	state.RunRollup()
	assert.Equal(t, "merged", state.Issues["story-01"].Status)
	assert.Equal(t, "merged", state.Issues["epic-01"].Status,
		"epic should cascade once the cancelled child stops blocking its story")
}

func TestApplyUnlinkOp_BlockedByRel(t *testing.T) {
	t.Parallel()
	// Create two linked tasks then unlink them — exercises applyUnlink (engine.go:184, 445)
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}}))
	require.Contains(t, state.Issues["task-01"].BlockedBy, "task-02")

	// Unlink: task-01 is no longer blocked_by task-02
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpUnlink, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}}))
	assert.NotContains(t, state.Issues["task-01"].BlockedBy, "task-02")
	assert.NotContains(t, state.Issues["task-02"].Blocks, "task-01")
}

func TestApplyUnlinkOp_NonBlockedByRel_NoOp(t *testing.T) {
	t.Parallel()
	// Unlink with a rel other than "blocked_by" should be a no-op (engine.go:184 negation path)
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}}))
	// Link with "blocked_by" first
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}}))
	// Unlink with a different rel — should not remove the blocked_by relationship
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpUnlink, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "relates-to"}}))
	assert.Contains(t, state.Issues["task-01"].BlockedBy, "task-02", "blocked_by not removed for non-blocked_by unlink")
}

func TestApplyTransition_ReopenClearsPriorOutcome(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "First attempt done"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 400,
		WorkerID: "w1", Payload: ops.Payload{To: "open"}}))

	issue := state.Issues["task-01"]
	assert.Equal(t, "open", issue.Status)
	assert.Empty(t, issue.Outcome, "outcome should be cleared on reopen")
	assert.Contains(t, issue.PriorOutcomes, "First attempt done")
}

func TestApplyTransition_ClaimedToOpenClearsClaimedBy(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	// Verify the claim was applied
	issue := state.Issues["task-01"]
	assert.Equal(t, "claimed", issue.Status)
	assert.Equal(t, "w1", issue.ClaimedBy)
	assert.Equal(t, int64(200), issue.ClaimedAt)

	// Apply compensating rollback: claimed → open (not done → open)
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "open"}}))

	// After transitioning to open, ClaimedBy and ClaimedAt must be cleared
	issue = state.Issues["task-01"]
	assert.Equal(t, "open", issue.Status)
	assert.Equal(t, "", issue.ClaimedBy, "ClaimedBy should be cleared on transition to open")
	assert.Equal(t, int64(0), issue.ClaimedAt, "ClaimedAt should be cleared on transition to open")
}

func TestPromoteParentToInProgress_SkipsAlreadyInProgress(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task", Parent: "story-01"}}))

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	assert.Equal(t, "in-progress", state.Issues["story-01"].Status)

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w2", Payload: ops.Payload{TTL: 60}}))
	assert.Equal(t, "in-progress", state.Issues["story-01"].Status)
}

func TestSortOpsByTimestamp(t *testing.T) {
	t.Parallel()
	allOps := []ops.Op{
		{Timestamp: 300, WorkerID: "w1"},
		{Timestamp: 100, WorkerID: "w1"},
		{Timestamp: 200, WorkerID: "w1"},
	}
	sortOpsByTimestamp(allOps)
	assert.Equal(t, int64(100), allOps[0].Timestamp)
	assert.Equal(t, int64(200), allOps[1].Timestamp)
	assert.Equal(t, int64(300), allOps[2].Timestamp)
}

func TestSortOpsByTimestamp_StableOnEqualTimestamp(t *testing.T) {
	t.Parallel()
	allOps := []ops.Op{
		{Timestamp: 100, WorkerID: "w2", Type: "first"},
		{Timestamp: 100, WorkerID: "w1", Type: "second"},
	}
	sortOpsByTimestamp(allOps)
	assert.Equal(t, "first", allOps[0].Type)
	assert.Equal(t, "second", allOps[1].Type)
}

func TestApplyAmendOp_PatchesType(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "S1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Story", NodeType: "story"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "S1", Timestamp: 200, WorkerID: "w2",
		Payload: ops.Payload{NodeType: "epic"},
	}))
	assert.Equal(t, "epic", state.Issues["S1"].Type)
}

func TestApplyAmendOp_PatchesAcceptance(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Task", NodeType: "task"},
	}))
	acceptance := json.RawMessage(`[{"type":"test_passes","cmd":"make check"}]`)
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "T1", Timestamp: 200, WorkerID: "w2",
		Payload: ops.Payload{Acceptance: acceptance},
	}))
	assert.NotEmpty(t, state.Issues["T1"].Acceptance)
	assert.Equal(t, string(acceptance), string(state.Issues["T1"].Acceptance))
}

func TestApplyAmendOp_PatchesScope(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Task", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "T1", Timestamp: 200, WorkerID: "w2",
		Payload: ops.Payload{Scope: []string{"internal/**"}},
	}))
	assert.Equal(t, []string{"internal/**"}, state.Issues["T1"].Scope)
}

func TestApplyCreateOp_SetsContextFiles(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{
			Title:        "Task",
			NodeType:     "task",
			ContextFiles: []string{"docs/adr.md", "docs/plan.md"},
		},
	}))
	assert.Equal(t, []string{"docs/adr.md", "docs/plan.md"}, state.Issues["T1"].ContextFiles)
}

func TestApplyAmendOp_ReplacesAndClearsContextFiles(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{
			Title:        "Task",
			NodeType:     "task",
			ContextFiles: []string{"docs/original.md"},
		},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "T1", Timestamp: 200, WorkerID: "w2",
		Payload: ops.Payload{ContextFiles: []string{"docs/replaced.md", "docs/extra.md"}},
	}))
	assert.Equal(t, []string{"docs/replaced.md", "docs/extra.md"}, state.Issues["T1"].ContextFiles)

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "T1", Timestamp: 300, WorkerID: "w2",
		Payload: ops.Payload{ClearContextFiles: true},
	}))
	assert.Empty(t, state.Issues["T1"].ContextFiles)
}

func TestApplyCreateOp_NormalizesCommaSeparatedScope(t *testing.T) {
	t.Parallel()
	// Legacy ops stored scope as a single comma-joined string; materializer must split them.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{
			Title:    "T",
			NodeType: "task",
			Scope:    []string{"cmd/a.go, cmd/b.go, cmd/c.go"},
		},
	}))
	assert.Equal(t, []string{"cmd/a.go", "cmd/b.go", "cmd/c.go"}, state.Issues["T1"].Scope)
}

func TestApplyAmendOp_NormalizesCommaSeparatedScope(t *testing.T) {
	t.Parallel()
	// Same normalization must apply when scope is set via amend.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "T1", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{Scope: []string{"cmd/x.go, cmd/y.go"}},
	}))
	assert.Equal(t, []string{"cmd/x.go", "cmd/y.go"}, state.Issues["T1"].Scope)
}

func TestApplyAmendOp_NormalizesCommaSeparatedContextFiles(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "T1", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ContextFiles: []string{"docs/a.md, docs/b.md"}},
	}))
	assert.Equal(t, []string{"docs/a.md", "docs/b.md"}, state.Issues["T1"].ContextFiles)
}

func TestMaterializedStateCollapsesHistoricalClaims(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Task", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a",
		Payload: ops.Payload{TTL: 60},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300, WorkerID: "worker-b",
		Payload: ops.Payload{TTL: 90},
	}))

	issue := state.Issues["task-01"]
	require.NotNil(t, issue)
	assert.Equal(t, "claimed", issue.Status)
	assert.Equal(t, "worker-a", issue.ClaimedBy)
	assert.Equal(t, int64(200), issue.ClaimedAt)
	assert.Equal(t, 60, issue.ClaimTTL)
}

func TestMaterializedState_AllowsClaimTakeoverWhenPriorClaimIsStale(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Task", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a",
		Payload: ops.Payload{TTL: 1},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 400, WorkerID: "worker-b",
		Payload: ops.Payload{TTL: 90},
	}))

	issue := state.Issues["task-01"]
	require.NotNil(t, issue)
	assert.Equal(t, "claimed", issue.Status)
	assert.Equal(t, "worker-b", issue.ClaimedBy)
	assert.Equal(t, int64(400), issue.ClaimedAt)
	assert.Equal(t, 90, issue.ClaimTTL)
}

func TestApplyAmendOp_UnknownIssue_NoError(t *testing.T) {
	t.Parallel()
	state := NewState()
	err := state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "NONEXISTENT", Timestamp: 100,
		Payload: ops.Payload{NodeType: "epic"},
	})
	assert.NoError(t, err)
}

func TestApplyCitationAccepted(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCitationAccepted, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ConfirmedNoninteractively: true},
	}))
	issue := state.Issues["task-01"]
	require.Len(t, issue.CitationAcceptances, 1)
	assert.Equal(t, "w1", issue.CitationAcceptances[0].WorkerID)
	assert.Equal(t, int64(200), issue.CitationAcceptances[0].Timestamp)
	assert.True(t, issue.CitationAcceptances[0].ConfirmedNoninteractively)
	assert.Equal(t, int64(200), issue.Updated)
}

func TestApplyCitationAccepted_UnknownIssue_NoError(t *testing.T) {
	t.Parallel()
	state := NewState()
	err := state.ApplyOp(ops.Op{
		Type: ops.OpCitationAccepted, TargetID: "NONEXISTENT", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{ConfirmedNoninteractively: false},
	})
	assert.NoError(t, err)
}

func TestApplyCitationAccepted_SourceEntryID_Populated(t *testing.T) {
	t.Parallel()
	// CitationAcceptance.SourceEntryID must be populated from op.Payload.SourceEntryID.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCitationAccepted, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ConfirmedNoninteractively: true, SourceEntryID: "entry-xyz"},
	}))
	issue := state.Issues["task-01"]
	require.Len(t, issue.CitationAcceptances, 1)
	assert.Equal(t, "entry-xyz", issue.CitationAcceptances[0].SourceEntryID)
}

func TestApplyCitationAccepted_SourceEntryID_EmptyWhenAbsent(t *testing.T) {
	t.Parallel()
	// CitationAcceptance.SourceEntryID must be empty string when not set in payload.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCitationAccepted, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ConfirmedNoninteractively: false},
	}))
	issue := state.Issues["task-01"]
	require.Len(t, issue.CitationAcceptances, 1)
	assert.Equal(t, "", issue.CitationAcceptances[0].SourceEntryID)
}

func TestCitationAcceptance_SourceEntryID_RoundTripsJSON(t *testing.T) {
	t.Parallel()
	// CitationAcceptance.SourceEntryID must survive WriteIssue/LoadIssue JSON round-trip.
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	issue := Issue{
		ID:     "task-ca-rtrip",
		Type:   "task",
		Status: "open",
		Title:  "Citation acceptance round-trip",
		CitationAcceptances: []CitationAcceptance{
			{WorkerID: "w1", Timestamp: 100, SourceEntryID: "entry-roundtrip"},
		},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	require.NoError(t, WriteIssue(issuesDir, issue))

	loaded, err := LoadIssue(filepath.Join(issuesDir, "task-ca-rtrip.json"))
	require.NoError(t, err)
	require.Len(t, loaded.CitationAcceptances, 1)
	assert.Equal(t, "entry-roundtrip", loaded.CitationAcceptances[0].SourceEntryID)
}

func TestToTraceabilityRefs_PopulatesCitationAcceptanceCount(t *testing.T) {
	t.Parallel()
	issues := map[string]*Issue{
		"task-01": {
			ID: "task-01",
			CitationAcceptances: []CitationAcceptance{
				{WorkerID: "w1", Timestamp: 100},
				{WorkerID: "w2", Timestamp: 200},
			},
		},
		"task-02": {
			ID:                  "task-02",
			CitationAcceptances: nil,
		},
	}

	refs := toTraceabilityRefs(issues)

	refsByID := make(map[string]any)
	for _, r := range refs {
		refsByID[r.ID] = r
	}

	require.Len(t, refs, 2)

	for _, r := range refs {
		switch r.ID {
		case "task-01":
			assert.Equal(t, 2, r.CitationAcceptanceCount)
		case "task-02":
			assert.Equal(t, 0, r.CitationAcceptanceCount)
		}
	}
	_ = refsByID
}

func TestApplyScopeRenameOp_ExactPath(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"internal/auth/handler.go", "internal/auth/middleware.go"}},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "internal/auth/handler.go", NewPath: "internal/auth/router.go"},
	}))
	issue := state.Issues["task-01"]
	assert.Contains(t, issue.Scope, "internal/auth/router.go")
	assert.NotContains(t, issue.Scope, "internal/auth/handler.go")
	assert.Contains(t, issue.Scope, "internal/auth/middleware.go")
}

func TestApplyScopeRenameOp_GlobPattern(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"internal/auth/**", "internal/util/**"}},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "internal/auth/**", NewPath: "internal/authn/**"},
	}))
	issue := state.Issues["task-01"]
	assert.Contains(t, issue.Scope, "internal/authn/**")
	assert.NotContains(t, issue.Scope, "internal/auth/**")
	assert.Contains(t, issue.Scope, "internal/util/**")
}

func TestApplyScopeRenameOp_NoMatch_NoOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"internal/auth/**"}},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "internal/nonexistent/**", NewPath: "internal/other/**"},
	}))
	issue := state.Issues["task-01"]
	assert.Equal(t, []string{"internal/auth/**"}, issue.Scope, "scope should be unchanged when OldPath not found")
	assert.Equal(t, int64(200), issue.Updated, "Updated timestamp should be set even on no-match")
}

func TestApplyScopeRenameOp_Idempotent(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"internal/auth/**"}},
	}))
	renameOp := ops.Op{
		Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "internal/auth/**", NewPath: "internal/authn/**"},
	}
	require.NoError(t, state.ApplyOp(renameOp))
	require.NoError(t, state.ApplyOp(renameOp))
	issue := state.Issues["task-01"]
	assert.Equal(t, []string{"internal/authn/**"}, issue.Scope, "applying rename twice should not duplicate entries")
}

func TestApplyScopeRenameOp_UnknownIssue_Tolerated(t *testing.T) {
	t.Parallel()
	state := NewState()
	err := state.ApplyOp(ops.Op{
		Type: ops.OpScopeRename, TargetID: "nonexistent-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "internal/auth/**", NewPath: "internal/authn/**"},
	})
	assert.NoError(t, err, "scope-rename on unknown issue should be tolerated")
}

func TestApplyScopeDeleteOp_ExactMatch(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"internal/auth/handler.go", "internal/auth/middleware.go"}},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeDelete, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{DeletedPath: "internal/auth/handler.go"},
	}))
	issue := state.Issues["task-01"]
	assert.NotContains(t, issue.Scope, "internal/auth/handler.go", "deleted path should be removed from scope")
	assert.Contains(t, issue.Scope, "internal/auth/middleware.go", "non-deleted path should remain in scope")
}

func TestApplyScopeDeleteOp_GlobNotRemoved(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"internal/auth/**", "internal/auth/handler.go"}},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeDelete, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{DeletedPath: "internal/auth/handler.go"},
	}))
	issue := state.Issues["task-01"]
	assert.Contains(t, issue.Scope, "internal/auth/**", "glob entry should survive exact-match delete")
	assert.NotContains(t, issue.Scope, "internal/auth/handler.go", "exact entry should be removed")
}

func TestApplyScopeDeleteOp_NoMatch_NoOp(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"internal/auth/**"}},
	}))
	originalUpdated := state.Issues["task-01"].Updated
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeDelete, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{DeletedPath: "internal/nonexistent.go"},
	}))
	issue := state.Issues["task-01"]
	assert.Equal(t, []string{"internal/auth/**"}, issue.Scope, "scope should be unchanged when DeletedPath not found")
	assert.Equal(t, originalUpdated, issue.Updated, "Updated should be unchanged when no entry matched")
}

func TestApplyScopeDeleteOp_UnknownIssue_Tolerated(t *testing.T) {
	t.Parallel()
	state := NewState()
	err := state.ApplyOp(ops.Op{
		Type: ops.OpScopeDelete, TargetID: "nonexistent-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{DeletedPath: "internal/auth/handler.go"},
	})
	assert.NoError(t, err, "scope-delete on unknown issue should be tolerated")
}

func TestApplyCreateOp_PreferredModel_Propagated(t *testing.T) {
	t.Parallel()
	// Issue.PreferredModel must be populated from Payload.PreferredModel on create.
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-pm", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Task with model", NodeType: "task", PreferredModel: "claude-opus-4"},
	}
	require.NoError(t, state.ApplyOp(op))
	assert.Equal(t, "claude-opus-4", state.Issues["task-pm"].PreferredModel)
}

func TestApplyCreateOp_PreferredModel_EmptyWhenAbsent(t *testing.T) {
	t.Parallel()
	// Issue.PreferredModel must be empty when not set in the create payload.
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-nopm", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Task without model", NodeType: "task"},
	}
	require.NoError(t, state.ApplyOp(op))
	assert.Equal(t, "", state.Issues["task-nopm"].PreferredModel)
}

func TestIssue_PreferredModel_RoundTripsJSON(t *testing.T) {
	t.Parallel()
	// Issue.PreferredModel must survive WriteIssue/LoadIssue JSON round-trip.
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	issue := Issue{
		ID:             "task-rtrip",
		Type:           "task",
		Status:         "open",
		Title:          "Round-trip test",
		PreferredModel: "claude-sonnet-5",
		Children:       []string{},
		BlockedBy:      []string{},
		Blocks:         []string{},
		DecisionRefs:   []string{},
	}

	require.NoError(t, WriteIssue(issuesDir, issue))

	loaded, err := LoadIssue(filepath.Join(issuesDir, "task-rtrip.json"))
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-5", loaded.PreferredModel)
}

func TestApplyOp_GateEvidenceIsRecognizedNoOp_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	before := state.Issues["task-01"].Status

	err := state.ApplyOp(ops.Op{
		Type:      ops.OpGateEvidence,
		TargetID:  "full",
		Timestamp: 200,
		WorkerID:  "w1",
	})
	require.NoError(t, err, "gate-evidence is audit-only and must not be an unknown op")
	assert.Equal(t, before, state.Issues["task-01"].Status)
	assert.Contains(t, RegisteredOpTypes(), ops.OpGateEvidence)
}

func TestApplyOp_ManagedExecutionOps_ReturnUnknownError(t *testing.T) {
	t.Parallel()
	// Materializer must return unknown-op-type errors for managed-execution op types.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	removedOps := []string{
		"orchestrate-start",
		"orchestrate-dispatch",
		"orchestrate-dispatch-complete",
		"orchestrate-verify-fail",
		"orchestrate-retry",
		"orchestrate-escalate",
		"orchestrate-complete",
		"orchestrate-check-result",
		"worker-runtime-decision",
	}
	for _, opType := range removedOps {
		err := state.ApplyOp(ops.Op{
			Type: opType, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		})
		assert.Error(t, err, "managed-execution op type %q must return an error", opType)
		assert.Contains(t, err.Error(), "unknown op type", "error must say 'unknown op type'")
	}
}

// BenchmarkRunRollup_10kIssues benchmarks the rollup operation on a large hierarchy.
// This test demonstrates that RunRollup should complete in O(n) time.
// With the previous O(n²) implementation, 10k issues would take too long.
func BenchmarkRunRollup_10kIssues(b *testing.B) {
	state := NewState()

	// Create a 3-level hierarchy: 1 epic -> 100 stories -> 100 tasks per story
	// Total: ~10,101 issues
	timestamp := int64(100)

	// Create epic
	epicID := "epic-0"
	require.NoError(b, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: epicID, Timestamp: timestamp, WorkerID: "w1",
		Payload: ops.Payload{Title: "Epic", NodeType: "epic"},
	}))
	timestamp++

	// Create stories under epic
	storyIDs := make([]string, 100)
	for i := range 100 {
		storyID := "story-" + string(rune('0'+i/10)) + string(rune('0'+i%10))
		storyIDs[i] = storyID
		require.NoError(b, state.ApplyOp(ops.Op{
			Type: ops.OpCreate, TargetID: storyID, Timestamp: timestamp, WorkerID: "w1",
			Payload: ops.Payload{Title: "Story " + string(rune('0'+i/10)) + string(rune('0'+i%10)), NodeType: "story", Parent: epicID},
		}))
		timestamp++
	}

	// Create tasks under each story
	taskIDs := make([][]string, 100)
	for si := range 100 {
		taskIDs[si] = make([]string, 100)
		for ti := range 100 {
			taskID := "task-" + string(rune('0'+si/10)) + string(rune('0'+si%10)) + "-" + string(rune('0'+ti/10)) + string(rune('0'+ti%10))
			taskIDs[si][ti] = taskID
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpCreate, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: storyIDs[si]},
			}))
			timestamp++
		}
	}

	// Mark all tasks as merged (done must be explicitly promoted to merged in dual-branch mode)
	for si := range 100 {
		for ti := range 100 {
			taskID := taskIDs[si][ti]
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpClaim, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{TTL: 60},
			}))
			timestamp++
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpTransition, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{To: "done"},
			}))
			timestamp++
			// Manually set to merged since auto-promotion is removed
			state.Issues[taskID].Status = ops.StatusMerged
		}
	}

	// Now run the benchmark
	b.ResetTimer()
	for b.Loop() {
		state.RunRollup()
	}
	b.StopTimer()

	// Verify that the epic is merged (all children promoted)
	if state.Issues[epicID].Status != "merged" {
		b.Fatalf("epic should be merged after rollup, got %s", state.Issues[epicID].Status)
	}
}

func TestApplyReparent_MovesIssueToNewParent(t *testing.T) {
	t.Parallel()
	state := NewState()
	state.Issues["parent-A"] = &Issue{ID: "parent-A", Children: []string{"child-01"}}
	state.Issues["parent-B"] = &Issue{ID: "parent-B", Children: []string{}}
	state.Issues["child-01"] = &Issue{ID: "child-01", Parent: "parent-A"}

	err := state.ApplyOp(ops.Op{
		Type:      ops.OpReparent,
		TargetID:  "child-01",
		Timestamp: 1000,
		WorkerID:  "w1",
		Payload:   ops.Payload{Parent: "parent-B"},
	})
	require.NoError(t, err)

	assert.Equal(t, "parent-B", state.Issues["child-01"].Parent)
	assert.Contains(t, state.Issues["parent-B"].Children, "child-01")
	assert.NotContains(t, state.Issues["parent-A"].Children, "child-01")
}

func TestApplyReparent_MissingTargetIsNoop(t *testing.T) {
	t.Parallel()
	state := NewState()

	err := state.ApplyOp(ops.Op{
		Type:     ops.OpReparent,
		TargetID: "nonexistent",
		WorkerID: "w1",
		Payload:  ops.Payload{Parent: "some-parent"},
	})
	require.NoError(t, err)
}

func TestApplyReparent_EmptyNewParentMakesTopLevel(t *testing.T) {
	t.Parallel()
	state := NewState()
	state.Issues["parent-A"] = &Issue{ID: "parent-A", Children: []string{"child-01"}}
	state.Issues["child-01"] = &Issue{ID: "child-01", Parent: "parent-A"}

	err := state.ApplyOp(ops.Op{
		Type:      ops.OpReparent,
		TargetID:  "child-01",
		Timestamp: 1000,
		WorkerID:  "w1",
		Payload:   ops.Payload{Parent: ""},
	})
	require.NoError(t, err)

	assert.Equal(t, "", state.Issues["child-01"].Parent)
	assert.NotContains(t, state.Issues["parent-A"].Children, "child-01")
}

// Fix N4: normalizeScopeEntries must filter empty string entries so that
// --context-file "" does not become [""] and render as (missing: ).
func TestNormalizeScopeEntries_FiltersEmptyStrings(t *testing.T) {
	t.Parallel()
	state := NewState()
	// Apply a create op with an empty string in ContextFiles (simulates --context-file "").
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task", ContextFiles: []string{""}},
	}))
	issue := state.Issues["task-01"]
	assert.Empty(t, issue.ContextFiles, "empty string context_files entries must be filtered out")
}

func TestNormalizeScopeEntries_FiltersEmptyStringsViaAmend(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	// Amend with an empty context-file string.
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ContextFiles: []string{""}},
	}))
	issue := state.Issues["task-01"]
	assert.Empty(t, issue.ContextFiles, "empty string context_files from amend must be filtered out")
}

func TestApplyAssessmentAttested(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))

	// Create an assessment attestation and marshal it
	att := review.AssessmentAttestation{
		SchemaVersion:           1,
		BundleID:                "bundle-1",
		ContractFingerprint:     "cf-hash",
		DeliveryFingerprint:     "df-hash",
		BaseSHA:                 "base-sha",
		HeadSHA:                 "head-sha",
		Rating:                  review.Green,
		ResultFingerprint:       "result-fp-1",
		SatisfiedCount:          3,
		PartiallySatisfiedCount: 0,
		NotSatisfiedCount:       0,
		IndeterminateCount:      0,
	}
	assessmentJSON, err := json.Marshal(att)
	require.NoError(t, err)

	// Apply the assessment-attested op
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAssessmentAttested, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Assessment: assessmentJSON},
	}))

	issue := state.Issues["task-01"]
	require.Len(t, issue.AssessmentAttestations, 1)
	assert.Equal(t, "bundle-1", issue.AssessmentAttestations[0].BundleID)
	assert.Equal(t, "result-fp-1", issue.AssessmentAttestations[0].ResultFingerprint)
	assert.Equal(t, int64(200), issue.Updated)
}

func TestApplyAssessmentAttested_DeduplicatesByResultFingerprint(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))

	// Create an assessment attestation with fingerprint "result-fp-1"
	att := review.AssessmentAttestation{
		SchemaVersion:       1,
		BundleID:            "bundle-1",
		ContractFingerprint: "cf-hash",
		DeliveryFingerprint: "df-hash",
		BaseSHA:             "base-sha",
		HeadSHA:             "head-sha",
		Rating:              review.Green,
		ResultFingerprint:   "result-fp-1",
		SatisfiedCount:      3,
	}
	assessmentJSON, err := json.Marshal(att)
	require.NoError(t, err)

	// Apply the first assessment-attested op
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAssessmentAttested, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Assessment: assessmentJSON},
	}))
	require.Len(t, state.Issues["task-01"].AssessmentAttestations, 1)

	// Apply the same assessment again (same fingerprint)
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAssessmentAttested, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w2", Payload: ops.Payload{Assessment: assessmentJSON},
	}))

	// Should still have only 1, not 2 (deduplicated)
	require.Len(t, state.Issues["task-01"].AssessmentAttestations, 1)
}

func TestApplyAssessmentAttested_DifferentFingerprintAdded(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))

	// First assessment
	att1 := review.AssessmentAttestation{
		SchemaVersion:     1,
		BundleID:          "bundle-1",
		ResultFingerprint: "result-fp-1",
		SatisfiedCount:    3,
	}
	assessmentJSON1, err := json.Marshal(att1)
	require.NoError(t, err)

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAssessmentAttested, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Assessment: assessmentJSON1},
	}))

	// Second assessment with different fingerprint
	att2 := review.AssessmentAttestation{
		SchemaVersion:     1,
		BundleID:          "bundle-2",
		ResultFingerprint: "result-fp-2",
		SatisfiedCount:    2,
	}
	assessmentJSON2, err := json.Marshal(att2)
	require.NoError(t, err)

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAssessmentAttested, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w2", Payload: ops.Payload{Assessment: assessmentJSON2},
	}))

	// Should have 2 attestations
	issue := state.Issues["task-01"]
	require.Len(t, issue.AssessmentAttestations, 2)
	assert.Equal(t, "result-fp-1", issue.AssessmentAttestations[0].ResultFingerprint)
	assert.Equal(t, "result-fp-2", issue.AssessmentAttestations[1].ResultFingerprint)
}

func TestApplyAssessmentAttested_IssueNotFound(t *testing.T) {
	t.Parallel()
	state := NewState()

	att := review.AssessmentAttestation{
		ResultFingerprint: "result-fp-1",
	}
	assessmentJSON, err := json.Marshal(att)
	require.NoError(t, err)

	// Apply op to non-existent issue
	err = state.ApplyOp(ops.Op{
		Type: ops.OpAssessmentAttested, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Assessment: assessmentJSON},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "issue task-01 not found")
}
