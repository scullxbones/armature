package validate

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
)

func sourceSetFromJSON(t *testing.T, data []byte) SourceSet {
	t.Helper()
	if len(data) == 0 {
		return SourceSet{}
	}
	var m sources.Manifest
	if err := m.Unmarshal(data); err != nil {
		return UnreadableSources(err)
	}
	ids := make(map[string]struct{}, len(m.Entries))
	for id := range m.Entries {
		ids[id] = struct{}{}
	}
	return KnownSources(ids)
}

func factsFromIssues(issues map[string]*materialize.Issue, now int64) map[string]IssueFacts {
	out := make(map[string]IssueFacts, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		out[id] = issueFacts(issue, now)
	}
	return out
}

func issueFacts(issue *materialize.Issue, now int64) IssueFacts {
	f := IssueFacts{
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
		f.Decisions = append(f.Decisions, DecisionFact{Topic: d.Topic, Choice: d.Choice})
	}
	return f
}

func validateState(state *materialize.State, opts Options) Result {
	if state == nil {
		return Validate(nil, opts)
	}
	return Validate(factsFromIssues(state.Issues, opts.Now), opts)
}

func checkIntroduction(current *materialize.State, proposed []ops.Op, opts Options) error {
	if current == nil {
		current = materialize.NewState()
	}
	afterState, err := projectState(current, proposed)
	if err != nil {
		return err
	}
	extra, err := findingsAfterReopeningTerminalIssues(current, proposed, opts)
	if err != nil {
		return err
	}
	return CheckIntroduction(
		factsFromIssues(current.Issues, opts.Now),
		factsFromIssues(afterState.Issues, opts.Now),
		targetedIDs(proposed),
		extra,
		opts,
	)
}

func findingsAfterReopeningTerminalIssues(current *materialize.State, proposed []ops.Op, opts Options) ([]Finding, error) {
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
	projected, err := cloneState(current)
	if err != nil {
		return nil, err
	}
	if err := materialize.ApplyOpsSorted(projected, unsuppressing); err != nil {
		return nil, fmt.Errorf("project un-suppression baseline: %w", err)
	}
	return validateState(projected, opts).Findings, nil
}

func projectState(current *materialize.State, proposed []ops.Op) (*materialize.State, error) {
	after, err := cloneState(current)
	if err != nil {
		return nil, err
	}
	if err := materialize.ApplyOpsSorted(after, proposed); err != nil {
		return nil, fmt.Errorf("project proposed: %w", err)
	}
	return after, nil
}

func cloneState(src *materialize.State) (*materialize.State, error) {
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

func targetedIDs(proposed []ops.Op) []string {
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
