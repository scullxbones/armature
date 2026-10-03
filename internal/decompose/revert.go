package decompose

import (
	"fmt"
	"sort"

	"github.com/scullxbones/armature/internal/clock"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

type DryRunRevertResult struct {
	WouldCancel []DryRunEntry
}

// ForeignChildError is returned when revert would strand an issue that is
// not in the plan but currently has a planned issue as its parent.
type ForeignChildError struct {
	Child  string
	Parent string
}

func (e *ForeignChildError) Error() string {
	if e == nil {
		return "cannot revert: a child not in the plan is attached to a planned issue"
	}
	return fmt.Sprintf("cannot revert: issue %s is a child of planned issue %s and is not in the plan", e.Child, e.Parent)
}

// CheckForeignChildren refuses revert when any issue not in the plan has a
// planned issue as its parent. Dry-run and apply share the same error.
func CheckForeignChildren(plan *Plan, state *materialize.State) error {
	if plan == nil || state == nil {
		return nil
	}
	inPlan := make(map[string]struct{}, len(plan.Issues))
	for _, issue := range plan.Issues {
		inPlan[issue.ID] = struct{}{}
	}

	type pair struct{ child, parent string }
	var found []pair
	seen := make(map[string]struct{})
	add := func(child, parent string) {
		if child == "" || parent == "" {
			return
		}
		if _, ok := inPlan[child]; ok {
			return
		}
		if _, ok := inPlan[parent]; !ok {
			return
		}
		key := child + "\x00" + parent
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		found = append(found, pair{child: child, parent: parent})
	}

	for id, issue := range state.Issues {
		if issue == nil {
			continue
		}
		add(id, issue.Parent)
		if _, ok := inPlan[id]; !ok {
			continue
		}
		for _, childID := range issue.Children {
			add(childID, id)
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].child == found[j].child {
			return found[i].parent < found[j].parent
		}
		return found[i].child < found[j].child
	})
	return &ForeignChildError{Child: found[0].child, Parent: found[0].parent}
}

func plannedOpenIssues(plan *Plan, state *materialize.State) []PlanIssue {
	var out []PlanIssue
	if plan == nil || state == nil {
		return out
	}
	for _, issue := range plan.Issues {
		stateIssue, exists := state.Issues[issue.ID]
		if !exists || stateIssue.Status != ops.StatusOpen {
			continue
		}
		out = append(out, issue)
	}
	return out
}

// DryRunRevertPlan returns what would be cancelled by CancelOps, without writing any ops.
func DryRunRevertPlan(plan *Plan, state *materialize.State) (*DryRunRevertResult, error) {
	if err := CheckForeignChildren(plan, state); err != nil {
		return nil, err
	}
	result := &DryRunRevertResult{}
	for _, issue := range plannedOpenIssues(plan, state) {
		result.WouldCancel = append(result.WouldCancel, DryRunEntry{ID: issue.ID, Title: issue.Title})
	}
	return result, nil
}

// CancelOps returns cancel transitions for still-open planned issues after
// the foreign-child guard.
func CancelOps(plan *Plan, workerID string, state *materialize.State, clk clock.Clock) ([]ops.Op, error) {
	if err := CheckForeignChildren(plan, state); err != nil {
		return nil, err
	}
	var proposed []ops.Op
	for _, issue := range plannedOpenIssues(plan, state) {
		proposed = append(proposed, ops.Op{
			Type:      ops.OpTransition,
			TargetID:  issue.ID,
			Timestamp: clk(),
			WorkerID:  workerID,
			Payload: ops.Payload{
				To: ops.StatusCancelled,
			},
		})
	}
	return proposed, nil
}
