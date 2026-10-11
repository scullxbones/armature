package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/validate"
)

func validateFacts(issues map[string]*materialize.Issue, now int64) map[string]validate.IssueFacts {
	out := make(map[string]validate.IssueFacts, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		out[id] = validateIssueFacts(issue, now)
	}
	return out
}

func validateIssueFacts(issue *materialize.Issue, now int64) validate.IssueFacts {
	f := validate.IssueFacts{
		ID:                  issue.ID,
		Type:                issue.Type,
		Status:              issue.Status,
		Title:               issue.Title,
		Parent:              issue.Parent,
		DefinitionOfDone:    issue.DefinitionOfDone,
		EstComplexity:       issue.EstComplexity,
		Outcome:             issue.Outcome,
		ClaimedBy:           issue.ClaimedBy,
		ClaimStale:          issue.ClaimStale(now),
		CitationAcceptances: len(issue.CitationAcceptances),
		Scope:               append([]string(nil), issue.Scope...),
		ContextFiles:        append([]string(nil), issue.ContextFiles...),
		Children:            append([]string(nil), issue.Children...),
		BlockedBy:           append([]string(nil), issue.BlockedBy...),
		Blocks:              append([]string(nil), issue.Blocks...),
		Acceptance:          append([]byte(nil), issue.Acceptance...),
		Context:             append([]byte(nil), issue.Context...),
	}
	for _, link := range issue.SourceLinks {
		f.SourceEntryIDs = append(f.SourceEntryIDs, link.SourceEntryID)
	}
	for _, d := range issue.Decisions {
		f.Decisions = append(f.Decisions, validate.DecisionFact{Topic: d.Topic, Choice: d.Choice})
	}
	return f
}

func checkGraphIntroduction(state *materialize.State, proposed []ops.Op, opts validate.Options) error {
	if state == nil {
		state = materialize.NewState()
	}
	afterState, err := projectValidateState(state, proposed)
	if err != nil {
		return err
	}
	extra, err := reopenIntroductionFindings(state, proposed, opts)
	if err != nil {
		return err
	}
	return validate.CheckIntroduction(
		validateFacts(state.Issues, opts.Now),
		validateFacts(afterState.Issues, opts.Now),
		introductionTargetIDs(proposed),
		extra,
		opts,
	)
}

func reopenIntroductionFindings(current *materialize.State, proposed []ops.Op, opts validate.Options) ([]validate.Finding, error) {
	var unsuppressing []ops.Op
	for _, op := range proposed {
		if op.Type != ops.OpTransition {
			continue
		}
		issue, ok := current.Issues[op.TargetID]
		if !ok || !ops.IsTerminalStatus(issue.Status) {
			continue
		}
		if ops.IsTerminalStatus(op.Payload.To) {
			continue
		}
		unsuppressing = append(unsuppressing, op)
	}
	if len(unsuppressing) == 0 {
		return nil, nil
	}
	projected, err := cloneValidateState(current)
	if err != nil {
		return nil, err
	}
	if err := materialize.ApplyOpsSorted(projected, unsuppressing); err != nil {
		return nil, fmt.Errorf("project un-suppression baseline: %w", err)
	}
	result := validate.Validate(validateFacts(projected.Issues, opts.Now), opts)
	return result.Findings, nil
}

func projectValidateState(current *materialize.State, proposed []ops.Op) (*materialize.State, error) {
	after, err := cloneValidateState(current)
	if err != nil {
		return nil, err
	}
	if err := materialize.ApplyOpsSorted(after, proposed); err != nil {
		return nil, fmt.Errorf("project proposed: %w", err)
	}
	return after, nil
}

func cloneValidateState(src *materialize.State) (*materialize.State, error) {
	if src == nil {
		return materialize.NewState(), nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("clone state: %w", err)
	}
	dst := materialize.NewState()
	if err := json.Unmarshal(data, dst); err != nil {
		return nil, fmt.Errorf("clone state: %w", err)
	}
	if dst.Issues == nil {
		dst.Issues = make(map[string]*materialize.Issue)
	}
	return dst, nil
}

func introductionTargetIDs(proposed []ops.Op) []string {
	seen := make(map[string]struct{})
	var ids []string
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, op := range proposed {
		add(op.TargetID)
		add(op.Payload.Dep)
		switch op.Type {
		case ops.OpCreate, ops.OpReparent:
			add(op.Payload.Parent)
		}
	}
	return ids
}
