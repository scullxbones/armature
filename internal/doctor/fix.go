package doctor

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/ready"
)

// FixAction is a single deterministic remediation planned by PlanFixes.
// Ops is the exact sequence of ops that ApplyFixes will append for this action.
type FixAction struct {
	IssueID string   `json:"issue_id"`
	Reason  string   `json:"reason"`
	Ops     []ops.Op `json:"ops"`
}

// LoadState materializes the ops log and returns the resulting index and per-issue
// state, for callers (such as `arm doctor --fix`) that need the same view Run uses
// internally but also want to compute fixes against it.
func LoadState(issuesDir, stateDir string) (materialize.Index, map[string]*materialize.Issue, error) {
	opsDir := filepath.Join(issuesDir, "ops")
	opItems, _, _, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read ops: %w", err)
	}
	allOps := ops.ExtractOps(opItems)

	if _, err := materialize.Materialize(stateDir, allOps, nil); err != nil {
		return nil, nil, fmt.Errorf("materialize: %w", err)
	}

	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("load index: %w", err)
	}

	allIssues, err := loadAllIssues(stateDir, index)
	if err != nil {
		return nil, nil, fmt.Errorf("load issues: %w", err)
	}
	return index, allIssues, nil
}

// PlanFixes computes the deterministic set of remediation ops for issues whose claim
// has expired, per the recovery state machine (docs/design/recovery-state-machine.md):
//
//   - claimed + claim-expired (D2 — Orphaned Claim): the claiming worker went silent
//     before starting; release the claim by transitioning back to open.
//   - in-progress + claim-expired (D2 — Orphaned Claim + Starvation): the claiming
//     worker went silent mid-work; transition to blocked pending manual investigation
//     rather than silently discarding in-flight work.
//
// Each action is expressed purely as ops to append; PlanFixes never mutates or
// removes existing op log lines. Calling PlanFixes again after ApplyFixes has
// appended its output is idempotent: the affected issues are no longer
// claimed/in-progress with an expired claim, so no further action is planned for
// them.
func PlanFixes(allIssues map[string]*materialize.Issue, workerID string, now time.Time) []FixAction {
	nowUnix := now.Unix()
	var actions []FixAction

	for _, id := range ready.StaleClaims(allIssues, now) {
		actions = append(actions, releaseExpiredClaim(id, allIssues[id], workerID, nowUnix))
	}

	for id, issue := range allIssues {
		if issue == nil || issue.Status != ops.StatusInProgress {
			continue
		}
		if !claimExpired(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, nowUnix) {
			continue
		}
		actions = append(actions, blockStarvedInProgress(id, issue, workerID, nowUnix))
	}

	sort.Slice(actions, func(i, j int) bool { return actions[i].IssueID < actions[j].IssueID })
	return actions
}

// ApplyFixes appends the ops for each planned fix to the given ops log, in order.
// A nil or empty actions slice is a no-op.
func ApplyFixes(logPath string, actions []FixAction) error {
	var flat []ops.Op
	for _, a := range actions {
		flat = append(flat, a.Ops...)
	}
	if len(flat) == 0 {
		return nil
	}
	return ops.AppendOps(logPath, flat)
}

// claimExpired mirrors the staleness formula used by ready.StaleClaims, for the
// in-progress case that StaleClaims itself does not cover (it only inspects
// StatusClaimed).
func claimExpired(claimedAt, lastHeartbeat int64, ttlMinutes int, now int64) bool {
	if ttlMinutes <= 0 {
		return false
	}
	lastActivity := max(claimedAt, lastHeartbeat)
	return now > lastActivity+int64(ttlMinutes)*60
}

func releaseExpiredClaim(id string, issue *materialize.Issue, workerID string, now int64) FixAction {
	claimedBy := ""
	if issue != nil {
		claimedBy = issue.ClaimedBy
	}
	return FixAction{
		IssueID: id,
		Reason:  "claimed + claim-expired: released stale claim, reset to open",
		Ops: []ops.Op{
			{
				Type: ops.OpTransition, TargetID: id, Timestamp: now, WorkerID: workerID,
				Payload: ops.Payload{To: ops.StatusOpen, Outcome: "Claim expired without worker transition; re-opening"},
			},
			{
				Type: ops.OpNote, TargetID: id, Timestamp: now, WorkerID: workerID,
				Payload: ops.Payload{
					Msg:    fmt.Sprintf("doctor --fix: released expired claim (previously claimed by %s)", claimedBy),
					NoteID: fmt.Sprintf("doctor-fix-%s-%d", id, now),
				},
			},
		},
	}
}

func blockStarvedInProgress(id string, issue *materialize.Issue, workerID string, now int64) FixAction {
	return FixAction{
		IssueID: id,
		Reason:  "in-progress + claim-expired: transitioned to blocked pending investigation",
		Ops: []ops.Op{
			{
				Type: ops.OpTransition, TargetID: id, Timestamp: now, WorkerID: workerID,
				Payload: ops.Payload{To: ops.StatusBlocked, Outcome: "Claim expired mid-work; worker unreachable. Manual investigation required."},
			},
			{
				Type: ops.OpNote, TargetID: id, Timestamp: now, WorkerID: workerID,
				Payload: ops.Payload{
					Msg: fmt.Sprintf(
						"doctor --fix: claim expired mid-work (previously claimed by %s, last heartbeat unix %d); blocked pending investigation",
						issue.ClaimedBy, issue.LastHeartbeat,
					),
					NoteID: fmt.Sprintf("doctor-fix-%s-%d", id, now),
				},
			},
		},
	}
}
