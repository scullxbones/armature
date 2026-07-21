package doctor

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
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

// PlanFixes computes the deterministic set of remediation ops for issues whose
// claim or worktree binding is broken, per the recovery state machine
// (docs/design/recovery-state-machine.md):
//
//   - claimed + claim-expired (D2 — Orphaned Claim): the claiming worker went silent
//     before starting; release the claim by transitioning back to open.
//   - in-progress + claim-expired (D2 — Orphaned Claim + Starvation): the claiming
//     worker went silent mid-work; transition to blocked pending manual investigation
//     rather than silently discarding in-flight work.
//   - claimed/in-progress + missing worktree: `arm claim --worktree` always creates
//     the task's worktree as part of a successful claim, so a claimed or in-progress
//     issue whose `task/<id>` branch has no live worktree registered against repoPath
//     indicates the worktree was torn down (or its git metadata corrupted) out from
//     under an active claim — the same class of failure this fix pass exists to
//     recover from, independent of whether the TTL has expired yet. This check is
//     skipped entirely — for every issue, not just the ones it would otherwise
//     flag — whenever GitWorktreeBranches cannot positively confirm which
//     branches have live worktrees (repoPath empty, not a git repo, or any other
//     git failure). Treating "couldn't determine" the same as "confirmed missing"
//     would misfire on every currently claimed/in-progress issue in the graph from
//     a single transient git error — exactly the mass-false-positive risk this
//     fix pass exists to avoid, not reintroduce.
//
// Each action is expressed purely as ops to append; PlanFixes never mutates or
// removes existing op log lines. Calling PlanFixes again after ApplyFixes has
// appended its output is idempotent: the affected issues are no longer
// claimed/in-progress with a broken claim, so no further action is planned for them.
//
// Deliberately out of scope: reopening `done` issues that lack a corroborating git
// commit ("half-recorded transitions" in the fully general sense). Unlike the cases
// above, that check has no way to bound itself to a small, currently-active set of
// issues — it would have to scan every done/merged issue in the whole graph, and a
// deleted-after-merge task branch is a normal, expected state for old work, not a
// sign of corruption. Auto-reopening on that signal risks mass false positives
// across the graph's history. Left as a manual `arm doctor` diagnostic follow-up
// rather than an automated fix.
func PlanFixes(allIssues map[string]*materialize.Issue, workerID string, now time.Time, repoPath string) []FixAction {
	nowUnix := now.Unix()
	var actions []FixAction
	fixed := make(map[string]bool)

	for _, id := range ready.StaleClaims(allIssues, now) {
		actions = append(actions, releaseExpiredClaim(id, allIssues[id], workerID, nowUnix))
		fixed[id] = true
	}

	for id, issue := range allIssues {
		if issue == nil || issue.Status != ops.StatusInProgress || fixed[id] {
			continue
		}
		if !claimExpired(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, nowUnix) {
			continue
		}
		actions = append(actions, blockStarvedInProgress(id, issue, workerID, nowUnix))
		fixed[id] = true
	}

	if liveBranches, err := adapters.GitWorktreeBranches(repoPath); err == nil {
		for id, issue := range allIssues {
			if issue == nil || fixed[id] {
				continue
			}
			if issue.Status != ops.StatusClaimed && issue.Status != ops.StatusInProgress {
				continue
			}
			branch := materialize.DeriveBranchName(issue.Type, id)
			if branch == "" || liveBranches[branch] {
				continue
			}
			actions = append(actions, releaseMissingWorktreeClaim(id, issue, workerID, nowUnix))
			fixed[id] = true
		}
	}

	sort.Slice(actions, func(i, j int) bool { return actions[i].IssueID < actions[j].IssueID })
	return actions
}

// ApplyFixes appends the ops for each planned fix to the given ops log, in
// order, committing each one to the worktree's branch the same way
// high-stakes ops (claim, transition, assign) do — via ops.AppendAndCommit —
// rather than writing to the local ops log file only. Pass worktreePath=""
// and gc=nil for single-branch mode, where AppendAndCommit skips the commit
// step. A nil or empty actions slice is a no-op.
func ApplyFixes(logPath, worktreePath string, actions []FixAction, gc ops.GitCommitter) error {
	for _, a := range actions {
		for _, op := range a.Ops {
			if err := ops.AppendAndCommit(logPath, worktreePath, op, gc); err != nil {
				return err
			}
		}
	}
	return nil
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

func releaseMissingWorktreeClaim(id string, issue *materialize.Issue, workerID string, now int64) FixAction {
	toStatus := ops.StatusOpen
	outcome := "Claimed task's worktree/branch no longer exists; releasing claim for re-dispatch."
	reason := "claimed + missing-worktree: no live task branch worktree found, released for re-dispatch"
	if issue.Status == ops.StatusInProgress {
		toStatus = ops.StatusBlocked
		outcome = "In-progress task's worktree/branch no longer exists; blocked pending investigation (possible in-flight work loss)."
		reason = "in-progress + missing-worktree: no live task branch worktree found, blocked pending investigation"
	}
	return FixAction{
		IssueID: id,
		Reason:  reason,
		Ops: []ops.Op{
			{
				Type: ops.OpTransition, TargetID: id, Timestamp: now, WorkerID: workerID,
				Payload: ops.Payload{To: toStatus, Outcome: outcome},
			},
			{
				Type: ops.OpNote, TargetID: id, Timestamp: now, WorkerID: workerID,
				Payload: ops.Payload{
					Msg:    fmt.Sprintf("doctor --fix: no live worktree for task/%s (previously claimed by %s)", id, issue.ClaimedBy),
					NoteID: fmt.Sprintf("doctor-fix-worktree-%s-%d", id, now),
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
