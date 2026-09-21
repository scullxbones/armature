package doctor

import (
	"fmt"
	"sort"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/worktree"
)

// FixAction is a single deterministic remediation planned by PlanFixes.
// Ops is the exact sequence of ops that `arm doctor --fix` will append.
type FixAction struct {
	IssueID string   `json:"issue_id"`
	Reason  string   `json:"reason"`
	Ops     []ops.Op `json:"ops"`
}

func LoadState(issuesDir, stateDir string) (materialize.Index, map[string]*materialize.Issue, error) {
	loaded, err := loadMaterializedState(issuesDir, stateDir)
	if err != nil {
		return nil, nil, err
	}
	return loaded.index, loaded.issues, nil
}

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
		if !issue.ClaimStale(nowUnix) {
			continue
		}
		actions = append(actions, blockStarvedInProgress(id, issue, workerID, nowUnix))
		fixed[id] = true
	}

	if repoPath != "" {
		inventory, err := worktree.List(repoPath)
		if err != nil {
			return actions
		}
		for id, issue := range allIssues {
			if issue == nil || fixed[id] {
				continue
			}
			if issue.Status != ops.StatusClaimed && issue.Status != ops.StatusInProgress {
				continue
			}
			if issue.ClaimedBy != workerID {
				continue
			}
			location, livePath := worktree.LocateBinding(inventory, id, issue.WorktreePath)
			switch location {
			case worktree.BindingAtRecordedPath:
				continue
			case worktree.BindingElsewhere:
				actions = append(actions, reportWorktreePathDrift(id, issue.WorktreePath, livePath))
				continue
			case worktree.BindingAmbiguous:
				actions = append(actions, reportAmbiguousWorktreeBinding(id))
				continue
			case worktree.BindingNone:
				actions = append(actions, releaseMissingWorktreeClaim(id, issue, workerID, nowUnix))
				fixed[id] = true
			}
		}
	}

	sort.Slice(actions, func(i, j int) bool { return actions[i].IssueID < actions[j].IssueID })
	return actions
}

func reportWorktreePathDrift(id, recordedPath, livePath string) FixAction {
	return FixAction{
		IssueID: id,
		Reason:  fmt.Sprintf("claimed + worktree path drift: recorded %s, live binding at %s; claim preserved", recordedPath, livePath),
	}
}

func reportAmbiguousWorktreeBinding(id string) FixAction {
	return FixAction{
		IssueID: id,
		Reason:  "claimed + ambiguous worktree binding: claim preserved; disambiguate manually",
	}
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
