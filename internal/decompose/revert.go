package decompose

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// DryRunRevertResult holds the result of a dry-run revert.
type DryRunRevertResult struct {
	// WouldCancel contains the issue IDs and titles that would be cancelled.
	WouldCancel []DryRunEntry
}

// DryRunRevertPlan returns what would be cancelled by RevertPlan, without writing any ops.
func DryRunRevertPlan(plan *Plan, state *materialize.State) (*DryRunRevertResult, error) {
	result := &DryRunRevertResult{}
	for _, issue := range plan.Issues {
		stateIssue, exists := state.Issues[issue.ID]
		if !exists {
			continue
		}
		if stateIssue.Status != ops.StatusOpen {
			continue
		}
		result.WouldCancel = append(result.WouldCancel, DryRunEntry{ID: issue.ID, Title: issue.Title})
	}
	return result, nil
}

// RevertPlan appends cancel ops for each issue in the plan that exists in state with status "open".
// Returns count of issues cancelled.
func RevertPlan(plan *Plan, issuesDir string, workerID string, state *materialize.State) (int, error) {
	logPath := filepath.Join(issuesDir, workerID+".log")
	count := 0

	for _, issue := range plan.Issues {
		stateIssue, exists := state.Issues[issue.ID]
		if !exists {
			continue
		}
		if stateIssue.Status != ops.StatusOpen {
			continue
		}

		op := ops.Op{
			Type:      ops.OpTransition,
			TargetID:  issue.ID,
			Timestamp: time.Now().Unix(),
			WorkerID:  workerID,
			Payload: ops.Payload{
				To: ops.StatusCancelled,
			},
		}

		if err := ops.AppendOp(logPath, op); err != nil {
			return count, fmt.Errorf("append revert op for issue %s: %w", issue.ID, err)
		}
		count++
	}

	return count, nil
}
