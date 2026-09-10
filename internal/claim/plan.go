package claim

import (
	"fmt"
	"slices"

	"github.com/scullxbones/armature/internal/ops"
)

// IssueFacts is the borrowed, read-only snapshot of one issue used by PlanClaim.
type IssueFacts struct {
	Type      string
	Status    string
	ClaimedBy string
	Title     string
	Scope     []string
}

// PlanInput is the borrowed, read-only input to PlanClaim.
type PlanInput struct {
	TargetID    string
	TargetScope []string
	WorkerID    string
	Force       bool
	Issues      map[string]IssueFacts
	Graph       HierarchyGraph
	PriorOps    []ops.Op
}

// NoteIntent is a note PlanClaim would append; the command adapter writes it.
type NoteIntent struct {
	IssueID string
	Message string
}

// ClaimPlan is the overlap decision for a proposed Claim, produced before any
// Op is appended. Distinct from the Claim Op itself.
type ClaimPlan struct {
	BlockReasons []string
	Warnings     []string
	Notes        []NoteIntent
}

// PlanClaim decides overlapping-scope Claims as a pure in-process plan.
// Inputs are borrowed and read-only. Never panics.
func PlanClaim(in PlanInput) (ClaimPlan, error) {
	if in.TargetID == "" {
		return ClaimPlan{}, fmt.Errorf("plan claim: empty target id")
	}
	if in.WorkerID == "" {
		return ClaimPlan{}, fmt.Errorf("plan claim: empty worker id")
	}
	if in.Graph == nil {
		return ClaimPlan{}, fmt.Errorf("plan claim: nil hierarchy graph")
	}

	ids := make([]string, 0, len(in.Issues))
	for id := range in.Issues {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	var blocks, warnings []string
	var notes []NoteIntent

	for _, id := range ids {
		if id == in.TargetID {
			continue
		}
		fact := in.Issues[id]
		if !competes(fact) {
			continue
		}
		if !ScopesOverlapEx(in.TargetScope, fact.Scope, in.Graph, in.TargetID, id) {
			continue
		}

		holder := fact.ClaimedBy
		if holder == "" {
			holder = "unknown"
		}
		reason := fmt.Sprintf("scope overlap with %s (%s), a %s %s held by %s",
			id, fact.Title, fact.Type, fact.Status, holder)

		if fact.ClaimedBy == in.WorkerID {
			if !hasSameWorkerDismissal(in.PriorOps, in.TargetID, id, in.WorkerID) {
				notes = append(notes, NoteIntent{
					IssueID: in.TargetID,
					Message: fmt.Sprintf("Serial claim: scope overlap with %s (same worker, dismissed)", id),
				})
			}
			continue
		}

		if !in.Force {
			blocks = append(blocks, reason)
			continue
		}

		warnings = append(warnings, reason)
		notes = append(notes,
			NoteIntent{
				IssueID: in.TargetID,
				Message: fmt.Sprintf("Scope overlap with %s detected at claim time", id),
			},
			NoteIntent{
				IssueID: id,
				Message: fmt.Sprintf("Scope overlap with %s detected at claim time", in.TargetID),
			},
		)
	}

	if len(blocks) > 0 {
		return ClaimPlan{BlockReasons: blocks}, nil
	}
	return ClaimPlan{Warnings: warnings, Notes: notes}, nil
}

func competes(fact IssueFacts) bool {
	if fact.Type != "task" {
		return false
	}
	return fact.Status == ops.StatusClaimed || fact.Status == ops.StatusInProgress
}

func hasSameWorkerDismissal(prior []ops.Op, targetID, otherID, workerID string) bool {
	if !HasOverlapDismissalNote(prior, targetID, otherID) {
		return false
	}
	expected := fmt.Sprintf("Serial claim: scope overlap with %s (same worker, dismissed)", otherID)
	for _, op := range prior {
		if op.Type == ops.OpNote && op.TargetID == targetID && op.WorkerID == workerID && op.Payload.Msg == expected {
			return true
		}
	}
	return false
}
