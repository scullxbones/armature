package materialize

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
)

// State holds the complete materialized state built from op replay.
type State struct {
	Issues           map[string]*Issue
	SingleBranchMode bool
}

func NewState() *State {
	return &State{
		Issues: make(map[string]*Issue),
	}
}

// ApplyOp applies a single op to the materialized state.
func (s *State) ApplyOp(op ops.Op) error {
	switch op.Type {
	case ops.OpCreate:
		return s.applyCreate(op)
	case ops.OpClaim:
		return s.applyClaim(op)
	case ops.OpHeartbeat:
		return s.applyHeartbeat(op)
	case ops.OpTransition:
		return s.applyTransition(op)
	case ops.OpNote:
		return s.applyNote(op)
	case ops.OpLink:
		return s.applyLink(op)
	case ops.OpDecision:
		return s.applyDecision(op)
	case ops.OpSourceLink, ops.OpSourceFingerprint, ops.OpDAGTransition:
		return nil
	default:
		return fmt.Errorf("unknown op type: %s", op.Type)
	}
}

func (s *State) applyCreate(op ops.Op) error {
	if _, exists := s.Issues[op.TargetID]; exists {
		return nil
	}
	issue := &Issue{
		ID:               op.TargetID,
		Type:             op.Payload.NodeType,
		Status:           ops.StatusOpen,
		Title:            op.Payload.Title,
		Parent:           op.Payload.Parent,
		Scope:            op.Payload.Scope,
		Priority:         op.Payload.Priority,
		EstComplexity:    op.Payload.EstComplexity,
		DefinitionOfDone: op.Payload.DefinitionOfDone,
		Acceptance:       op.Payload.Acceptance,
		Context:          op.Payload.Context,
		SourceCitation:   op.Payload.SourceCitation,
		Provenance: Provenance{
			Method:       "decomposed",
			Confidence:   "verified",
			SourceWorker: op.WorkerID,
		},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
		Updated:      op.Timestamp,
	}
	s.Issues[op.TargetID] = issue
	if op.Payload.Parent != "" {
		if parent, ok := s.Issues[op.Payload.Parent]; ok {
			parent.Children = appendUnique(parent.Children, op.TargetID)
		}
	}
	return nil
}

func (s *State) applyClaim(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("claim: issue %s not found", op.TargetID)
	}
	issue.Status = ops.StatusClaimed
	issue.ClaimedBy = op.WorkerID
	issue.ClaimedAt = op.Timestamp
	issue.ClaimTTL = op.Payload.TTL
	issue.LastHeartbeat = op.Timestamp
	issue.Updated = op.Timestamp
	s.promoteParentToInProgress(issue.Parent)
	return nil
}

func (s *State) applyHeartbeat(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	issue.LastHeartbeat = op.Timestamp
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyTransition(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("transition: issue %s not found", op.TargetID)
	}
	newStatus := op.Payload.To
	if newStatus == ops.StatusOpen && issue.Status == ops.StatusDone {
		if issue.Outcome != "" {
			issue.PriorOutcomes = append(issue.PriorOutcomes, issue.Outcome)
			issue.Outcome = ""
		}
		issue.ClaimedBy = ""
		issue.ClaimedAt = 0
	}
	issue.Status = newStatus
	issue.Updated = op.Timestamp
	if op.Payload.Outcome != "" {
		issue.Outcome = op.Payload.Outcome
	}
	if op.Payload.Branch != "" {
		issue.Branch = op.Payload.Branch
	}
	if op.Payload.PR != "" {
		issue.PR = op.Payload.PR
	}
	if s.SingleBranchMode && newStatus == ops.StatusDone {
		issue.Status = ops.StatusMerged
	}
	return nil
}

func (s *State) applyNote(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	issue.Notes = append(issue.Notes, Note{
		WorkerID:  op.WorkerID,
		Timestamp: op.Timestamp,
		Msg:       op.Payload.Msg,
	})
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyLink(op ops.Op) error {
	source, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("link: source issue %s not found", op.TargetID)
	}
	if op.Payload.Rel == "blocked_by" {
		source.BlockedBy = appendUnique(source.BlockedBy, op.Payload.Dep)
		if dep, ok := s.Issues[op.Payload.Dep]; ok {
			dep.Blocks = appendUnique(dep.Blocks, op.TargetID)
		}
	}
	source.Updated = op.Timestamp
	return nil
}

func (s *State) applyDecision(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	issue.Decisions = append(issue.Decisions, Decision{
		Topic:     op.Payload.Topic,
		Choice:    op.Payload.Choice,
		Rationale: op.Payload.Rationale,
		Affects:   op.Payload.Affects,
		WorkerID:  op.WorkerID,
		Timestamp: op.Timestamp,
	})
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) promoteParentToInProgress(parentID string) {
	if parentID == "" {
		return
	}
	parent, ok := s.Issues[parentID]
	if !ok {
		return
	}
	if parent.Status == ops.StatusOpen {
		parent.Status = ops.StatusInProgress
	}
}

// RunRollup promotes stories/epics to done/merged when all children are merged.
func (s *State) RunRollup() {
	changed := true
	for changed {
		changed = false
		for _, issue := range s.Issues {
			if issue.Type == "task" {
				continue
			}
			if issue.Status == ops.StatusMerged || issue.Status == ops.StatusCancelled {
				continue
			}
			if len(issue.Children) == 0 {
				continue
			}
			allMerged := true
			for _, childID := range issue.Children {
				child, ok := s.Issues[childID]
				if !ok || child.Status != ops.StatusMerged {
					allMerged = false
					break
				}
			}
			if allMerged && issue.Status != ops.StatusMerged {
				issue.Status = ops.StatusMerged
				changed = true
			}
		}
	}
}

// BuildIndex creates the denormalized index from current state.
func (s *State) BuildIndex() Index {
	index := make(Index, len(s.Issues))
	for id, issue := range s.Issues {
		index[id] = IndexEntry{
			Status:    issue.Status,
			Type:      issue.Type,
			Parent:    issue.Parent,
			Children:  issue.Children,
			BlockedBy: issue.BlockedBy,
			Blocks:    issue.Blocks,
			Assignee:  issue.ClaimedBy,
			Updated:   issue.Updated,
			Title:     issue.Title,
			Outcome:   issue.Outcome,
			Scope:     issue.Scope,
		}
	}
	return index
}

// activeDecisionForTopic returns the latest decision for a given topic.
func activeDecisionForTopic(decisions []Decision, topic string) Decision {
	var latest Decision
	for _, d := range decisions {
		if d.Topic == topic && d.Timestamp > latest.Timestamp {
			latest = d
		}
	}
	return latest
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
