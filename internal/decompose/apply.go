package decompose

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
)

// DryRunResult holds the result of a dry-run apply.
type DryRunResult struct {
	// WouldCreate contains the issue IDs and titles that would be created.
	WouldCreate []DryRunEntry
	// Warnings contains advisory messages from plan validation.
	Warnings []string
}

// DryRunEntry is a single would-be create entry.
type DryRunEntry struct {
	ID    string
	Title string
}

// ApplyOptions controls optional behaviour for ApplyPlan / DryRunApplyPlan.
type ApplyOptions struct {
	// Strict makes advisory warnings from ValidatePlan into errors.
	Strict bool
	// GenerateIDs replaces plan-specified IDs with system-generated UUIDs.
	GenerateIDs bool
	// Root, when non-empty, is used as the parent for any top-level plan
	// issues (those whose Parent field is empty).
	Root string
}

// ValidatePlan returns a list of advisory warnings for the plan.
// These are non-fatal by default; use ApplyOptions.Strict to treat them as errors.
func ValidatePlan(plan *Plan) []string {
	var warnings []string
	for _, issue := range plan.Issues {
		if issue.DoD == "" {
			warnings = append(warnings, fmt.Sprintf("issue %s (%s) is missing a definition of done", issue.ID, issue.Title))
		}
	}
	return warnings
}

// preparePlan applies the ApplyOptions transformations to a copy of the plan,
// returning the transformed plan and the ID mapping (old → new) when GenerateIDs
// is set.
func preparePlan(plan *Plan, opts ApplyOptions) (*Plan, map[string]string) {
	// Deep-copy issues to avoid mutating the caller's plan.
	issues := make([]PlanIssue, len(plan.Issues))
	copy(issues, plan.Issues)

	idMap := make(map[string]string)

	if opts.GenerateIDs {
		// First pass: assign new UUIDs.
		for i, issue := range issues {
			newID := uuid.New().String()
			idMap[issue.ID] = newID
			issues[i].ID = newID
		}
		// Second pass: rewrite Parent and BlockedBy references.
		for i, issue := range issues {
			if mapped, ok := idMap[issue.Parent]; ok {
				issues[i].Parent = mapped
			}
			newBlockedBy := make([]string, len(issue.BlockedBy))
			for j, dep := range issue.BlockedBy {
				if mapped, ok := idMap[dep]; ok {
					newBlockedBy[j] = mapped
				} else {
					newBlockedBy[j] = dep
				}
			}
			issues[i].BlockedBy = newBlockedBy
		}
	}

	// Apply --root: top-level issues (no parent) get root as their parent.
	if opts.Root != "" {
		for i := range issues {
			if issues[i].Parent == "" {
				issues[i].Parent = opts.Root
			}
		}
	}

	transformed := &Plan{
		Version: plan.Version,
		Title:   plan.Title,
		Issues:  issues,
	}
	return transformed, idMap
}

// DryRunApplyPlan validates the plan and returns what would be created, without writing any ops.
func DryRunApplyPlan(plan *Plan, state *materialize.State) (*DryRunResult, error) {
	return DryRunApplyPlanWithOptions(plan, state, ApplyOptions{})
}

// DryRunApplyPlanWithOptions is like DryRunApplyPlan but respects ApplyOptions.
func DryRunApplyPlanWithOptions(plan *Plan, state *materialize.State, opts ApplyOptions) (*DryRunResult, error) {
	warnings := ValidatePlan(plan)

	if opts.Strict && len(warnings) > 0 {
		return nil, fmt.Errorf("plan has %d advisory warning(s) (--strict mode): %s", len(warnings), warnings[0])
	}

	transformed, _ := preparePlan(plan, opts)

	result := &DryRunResult{Warnings: warnings}
	for _, issue := range transformed.Issues {
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
	return ApplyPlanWithOptions(plan, issuesDir, workerID, state, ApplyOptions{})
}

// ApplyPlanWithOptions is like ApplyPlan but respects ApplyOptions.
func ApplyPlanWithOptions(plan *Plan, issuesDir string, workerID string, state *materialize.State, opts ApplyOptions) (int, error) {
	warnings := ValidatePlan(plan)

	if opts.Strict && len(warnings) > 0 {
		return 0, fmt.Errorf("plan has %d advisory warning(s) (--strict mode): %s", len(warnings), warnings[0])
	}

	transformed, _ := preparePlan(plan, opts)

	logPath := filepath.Join(issuesDir, workerID+".log")
	count := 0

	for _, issue := range transformed.Issues {
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
				Acceptance:       issue.Acceptance,
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
