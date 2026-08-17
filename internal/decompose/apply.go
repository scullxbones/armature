package decompose

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/clock"
	"github.com/scullxbones/armature/internal/issueid"
	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/validate"
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
	// GenerateIDs replaces plan-specified IDs with system-generated UUIDs.
	GenerateIDs bool
	// Root, when non-empty, is used as the parent for any top-level plan
	// issues (those whose Parent field is empty).
	Root string
}

// ValidatePlan returns a list of advisory warnings for the plan.
// It does not report invalid issue types: those are always fatal, see
// validateTypes. Missing per-issue source is always fatal, see validateSources.
func ValidatePlan(plan *Plan) []string {
	var warnings []string
	for _, issue := range plan.Issues {
		if issue.DoD == "" {
			warnings = append(warnings, fmt.Sprintf("issue %s (%s) is missing a definition of done", issue.ID, issue.Title))
		}
	}
	return warnings
}

func validateSources(plan *Plan) error {
	for _, issue := range plan.Issues {
		if strings.TrimSpace(issue.Source) == "" {
			return fmt.Errorf("plan issue %s (%s) is missing source; apply is source-atomic", issue.ID, issue.Title)
		}
	}
	return nil
}

// validateTypes rejects a plan containing any issue with an unrecognized
// Type. Unlike the advisory warnings in ValidatePlan, this is always a hard
// error: an unrecognized type is never a legitimate, salvageable situation.
func validateTypes(plan *Plan) error {
	for _, issue := range plan.Issues {
		if !issuetype.IsValid(issue.Type) {
			return fmt.Errorf("issue %s (%s) has invalid type %q: valid types are %s",
				issue.ID, issue.Title, issue.Type, strings.Join(issuetype.All(), ", "))
		}
	}
	return nil
}

func validateIssueIDs(plan *Plan) error {
	for _, issue := range plan.Issues {
		if err := issueid.Validate(issue.ID); err != nil {
			return err
		}
	}
	return nil
}

// preparePlan applies the ApplyOptions transformations to a copy of the plan.
func preparePlan(plan *Plan, opts ApplyOptions) *Plan {
	// Deep-copy issues to avoid mutating the caller's plan.
	issues := make([]PlanIssue, len(plan.Issues))
	copy(issues, plan.Issues)

	if opts.GenerateIDs {
		idMap := make(map[string]string)
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

	return &Plan{
		Version: plan.Version,
		Title:   plan.Title,
		Issues:  issues,
	}
}

// DryRunApplyPlan validates the plan and returns what would be created, without writing any ops.
func DryRunApplyPlan(plan *Plan, state *materialize.State) (*DryRunResult, error) {
	return DryRunApplyPlanWithOptions(plan, state, ApplyOptions{})
}

// DryRunApplyPlanWithOptions is like DryRunApplyPlan but respects ApplyOptions.
func DryRunApplyPlanWithOptions(plan *Plan, state *materialize.State, opts ApplyOptions) (*DryRunResult, error) {
	if err := validateTypes(plan); err != nil {
		return nil, err
	}
	if err := validateIssueIDs(plan); err != nil {
		return nil, err
	}
	if err := validateSources(plan); err != nil {
		return nil, err
	}

	warnings := ValidatePlan(plan)

	transformed := preparePlan(plan, opts)
	if _, err := planOps(transformed, state, "dry-run", clock.System); err != nil {
		return nil, err
	}

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
	return ApplyPlanWithOptions(plan, issuesDir, workerID, state, ApplyOptions{}, clock.System)
}

// ApplyPlanWithOptions is like ApplyPlan but respects ApplyOptions and accepts a clock.Clock parameter.
func ApplyPlanWithOptions(plan *Plan, issuesDir string, workerID string, state *materialize.State, opts ApplyOptions, clk clock.Clock) (int, error) {
	if err := validateTypes(plan); err != nil {
		return 0, err
	}
	if err := validateIssueIDs(plan); err != nil {
		return 0, err
	}
	if err := validateSources(plan); err != nil {
		return 0, err
	}

	transformed := preparePlan(plan, opts)
	proposed, err := planOps(transformed, state, workerID, clk)
	if err != nil {
		return 0, err
	}

	logPath := filepath.Join(issuesDir, workerID+".log")
	count := 0
	for _, op := range proposed {
		if err := ops.AppendOp(logPath, op); err != nil {
			return count, fmt.Errorf("append op for issue %s: %w", op.TargetID, err)
		}
		if op.Type == ops.OpCreate {
			count++
		}
	}

	return count, nil
}

func planOps(plan *Plan, state *materialize.State, workerID string, clk clock.Clock) ([]ops.Op, error) {
	var proposed []ops.Op
	for _, issue := range plan.Issues {
		if _, exists := state.Issues[issue.ID]; exists {
			continue
		}

		scope := []string{}
		if issue.Scope != "" {
			scope = strings.Split(issue.Scope, ", ")
		}

		proposed = append(proposed, ops.Op{
			Type:      ops.OpCreate,
			TargetID:  issue.ID,
			Timestamp: clk(),
			WorkerID:  workerID,
			Payload: ops.Payload{
				Title:            issue.Title,
				NodeType:         issue.Type,
				Scope:            scope,
				ContextFiles:     issue.ContextFiles,
				Priority:         issue.Priority,
				DefinitionOfDone: issue.DoD,
				Parent:           issue.Parent,
				Acceptance:       issue.Acceptance,
				Confidence:       "draft",
			},
		})
		proposed = append(proposed, ops.Op{
			Type:      ops.OpSourceLink,
			TargetID:  issue.ID,
			Timestamp: clk(),
			WorkerID:  workerID,
			Payload: ops.Payload{
				SourceID: issue.Source,
			},
		})
		for _, dep := range issue.BlockedBy {
			proposed = append(proposed, ops.Op{
				Type:      ops.OpLink,
				TargetID:  issue.ID,
				Timestamp: clk(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					Dep: dep,
					Rel: "blocked_by",
				},
			})
		}
	}
	if err := validate.CheckIntroduction(state, proposed, validate.Options{Strict: true}); err != nil {
		return nil, err
	}
	return proposed, nil
}
