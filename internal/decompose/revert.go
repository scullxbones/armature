package decompose

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
)

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
