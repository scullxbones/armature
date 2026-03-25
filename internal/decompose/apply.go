package decompose

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
)

// DryRunResult holds the result of a dry-run apply.
type DryRunResult struct {
	// WouldCreate contains the issue IDs and titles that would be created.
	WouldCreate []DryRunEntry
}

// DryRunEntry is a single would-be create entry.
type DryRunEntry struct {
	ID    string
	Title string
}

// DryRunApplyPlan validates the plan and returns what would be created, without writing any ops.
func DryRunApplyPlan(plan *Plan, state *materialize.State) (*DryRunResult, error) {
	result := &DryRunResult{}
	for _, issue := range plan.Issues {
		if _, exists := state.Issues[issue.ID]; exists {
			continue
		}
		result.WouldCreate = append(result.WouldCreate, DryRunEntry{ID: issue.ID, Title: issue.Title})
	}
	return result, nil
}

// ApplyPlan appends create ops for each issue in the plan to the op log.
// Skips issues that already exist in state (by ID).
// Returns count of issues created.
func ApplyPlan(plan *Plan, issuesDir string, workerID string, state *materialize.State) (int, error) {
	logPath := filepath.Join(issuesDir, workerID+".log")
	count := 0

	for _, issue := range plan.Issues {
		if _, exists := state.Issues[issue.ID]; exists {
			continue
		}

		scope := []string{}
		if issue.Scope != "" {
			scope = []string{issue.Scope}
		}

		op := ops.Op{
			Type:      ops.OpCreate,
			TargetID:  issue.ID,
			Timestamp: time.Now().Unix(),
			WorkerID:  workerID,
			Payload: ops.Payload{
				Title:            issue.Title,
				NodeType:         issue.Type,
				Scope:            scope,
				Priority:         issue.Priority,
				DefinitionOfDone: issue.DoD,
				Parent:           issue.Parent,
				Confidence:       "draft",
			},
		}

		if err := ops.AppendOp(logPath, op); err != nil {
			return count, fmt.Errorf("append op for issue %s: %w", issue.ID, err)
		}
		count++

		// Emit link ops for blocked_by relationships.
		for _, dep := range issue.BlockedBy {
			linkOp := ops.Op{
				Type:      ops.OpLink,
				TargetID:  issue.ID,
				Timestamp: time.Now().Unix(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					Dep: dep,
					Rel: "blocked_by",
				},
			}
			if err := ops.AppendOp(logPath, linkOp); err != nil {
				return count, fmt.Errorf("append link op for issue %s -> %s: %w", issue.ID, dep, err)
			}
		}
	}

	return count, nil
}
