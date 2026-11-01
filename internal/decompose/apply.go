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

type DryRunEntry struct {
	ID     string
	Title  string
	Type   string
	Status string
}

type ApplyOptions struct {
	GenerateIDs bool
	Root        string
	Sources     validate.SourceSet
}

func validatePlan(plan *Plan) []string {
	var warnings []string
	for _, issue := range plan.Issues {
		if issue.DoD == "" {
			warnings = append(warnings, fmt.Sprintf("issue %s (%s) is missing a definition of done", issue.ID, issue.Title))
		}
	}
	return warnings
}

func validateSources(plan *Plan, sources validate.SourceSet) error {
	for _, issue := range plan.Issues {
		if strings.TrimSpace(issue.Source) == "" {
			return fmt.Errorf("plan issue %s (%s) is missing source; apply is source-atomic", issue.ID, issue.Title)
		}
	}
	if !sources.Loaded() {
		return nil
	}
	if err := sources.ParseError(); err != nil {
		return fmt.Errorf("parse source manifest: %w", err)
	}
	for _, issue := range plan.Issues {
		if !sources.Contains(issue.Source) {
			return fmt.Errorf("plan issue %s (%s) cites unknown source %s", issue.ID, issue.Title, issue.Source)
		}
	}
	return nil
}

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

func preparePlan(plan *Plan, opts ApplyOptions) *Plan {
	issues := make([]PlanIssue, len(plan.Issues))
	copy(issues, plan.Issues)

	if opts.GenerateIDs {
		issues = remapPlanIDsToUUIDs(issues)
	}

	if opts.Root != "" {
		issues = attachRootToOrphans(issues, opts.Root)
	}

	return &Plan{
		Version: plan.Version,
		Title:   plan.Title,
		Issues:  issues,
	}
}

func remapPlanIDsToUUIDs(issues []PlanIssue) []PlanIssue {
	idMap := make(map[string]string)
	for i, issue := range issues {
		newID := uuid.New().String()
		idMap[issue.ID] = newID
		issues[i].ID = newID
	}
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
	return issues
}

func attachRootToOrphans(issues []PlanIssue, root string) []PlanIssue {
	for i := range issues {
		if issues[i].Parent == "" {
			issues[i].Parent = root
		}
	}
	return issues
}

// DryRunApplyPlan validates the plan and returns what would be created, without writing any ops.
func DryRunApplyPlan(plan *Plan, state *materialize.State, opts ApplyOptions) ([]DryRunEntry, error) {
	if err := validateTypes(plan); err != nil {
		return nil, err
	}
	if err := validateIssueIDs(plan); err != nil {
		return nil, err
	}
	if err := validateSources(plan, opts.Sources); err != nil {
		return nil, err
	}

	transformed := preparePlan(plan, opts)
	proposed, err := planOps(transformed, state, "dry-run", clock.System, opts)
	if err != nil {
		return nil, err
	}

	return entriesForCreates(proposed), nil
}

// ApplyPlan appends create ops for each issue in the plan to the op log.
// Skips issues that already exist in state (by ID).
// Returns the issues this invocation appended (create ops in the batch), not a
// repository-wide state diff.
func ApplyPlan(plan *Plan, issuesDir string, workerID string, state *materialize.State, opts ApplyOptions, clk clock.Clock) ([]DryRunEntry, error) {
	if err := validateTypes(plan); err != nil {
		return nil, err
	}
	if err := validateIssueIDs(plan); err != nil {
		return nil, err
	}
	if err := validateSources(plan, opts.Sources); err != nil {
		return nil, err
	}

	transformed := preparePlan(plan, opts)
	proposed, err := planOps(transformed, state, workerID, clk, opts)
	if err != nil {
		return nil, err
	}

	logPath := filepath.Join(issuesDir, workerID+".log")
	if err := ops.AppendOps(logPath, proposed); err != nil {
		return nil, fmt.Errorf("append plan ops: %w", err)
	}
	return entriesForCreates(proposed), nil
}

func entriesForCreates(proposed []ops.Op) []DryRunEntry {
	var created []DryRunEntry
	for _, op := range proposed {
		if op.Type != ops.OpCreate {
			continue
		}
		created = append(created, DryRunEntry{
			ID:     op.TargetID,
			Title:  op.Payload.Title,
			Type:   op.Payload.NodeType,
			Status: ops.StatusOpen,
		})
	}
	return created
}

func planOps(plan *Plan, state *materialize.State, workerID string, clk clock.Clock, opts ApplyOptions) ([]ops.Op, error) {
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
	if err := CheckGraphIntroduction(state, proposed, validate.Options{Strict: true, Sources: opts.Sources, Now: clk()}); err != nil {
		return nil, err
	}
	return proposed, nil
}
