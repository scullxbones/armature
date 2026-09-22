// Package materialize replays the append-only op log into materialized task state: the
// DAG, checkpoints, history, and current snapshot the rest of armature reads from.
package materialize

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	claimpkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
)

// State holds the complete materialized state built from op replay.
type State struct {
	Issues map[string]*Issue
}

func NewState() *State {
	return &State{
		Issues: make(map[string]*Issue),
	}
}

// MissingTarget is handler-table metadata for a missing op.TargetID.
// Error fails replay; Ignore is success (no-op). Empty means the handler
// does not look up the target as an existing issue (create / no-ops).
type MissingTarget string

const (
	MissingTargetError  MissingTarget = "Error"
	MissingTargetIgnore MissingTarget = "Ignore"
)

type opHandler struct {
	apply         func(*State, ops.Op) error
	missingTarget MissingTarget
}

type missingTargetError struct {
	OpType   string
	TargetID string
}

func (e missingTargetError) Error() string {
	switch e.OpType {
	case ops.OpLink, ops.OpUnlink:
		return fmt.Sprintf("%s: source issue %s not found", e.OpType, e.TargetID)
	default:
		return fmt.Sprintf("%s: issue %s not found", e.OpType, e.TargetID)
	}
}

var opHandlers = map[string]opHandler{
	ops.OpCreate:             {apply: (*State).applyCreate},
	ops.OpClaim:              {apply: (*State).applyClaim, missingTarget: MissingTargetError},
	ops.OpHeartbeat:          {apply: (*State).applyHeartbeat, missingTarget: MissingTargetIgnore},
	ops.OpTransition:         {apply: (*State).applyTransition, missingTarget: MissingTargetError},
	ops.OpNote:               {apply: (*State).applyNote, missingTarget: MissingTargetIgnore},
	ops.OpNoteDelete:         {apply: (*State).applyNoteDelete, missingTarget: MissingTargetIgnore},
	ops.OpLink:               {apply: (*State).applyLink, missingTarget: MissingTargetError},
	ops.OpUnlink:             {apply: (*State).applyUnlink, missingTarget: MissingTargetError},
	ops.OpDecision:           {apply: (*State).applyDecision, missingTarget: MissingTargetIgnore},
	ops.OpAssign:             {apply: (*State).applyAssign, missingTarget: MissingTargetIgnore},
	ops.OpAmend:              {apply: (*State).applyAmend, missingTarget: MissingTargetIgnore},
	ops.OpSourceLink:         {apply: (*State).applySourceLink, missingTarget: MissingTargetIgnore},
	ops.OpSourceFingerprint:  {apply: func(_ *State, _ ops.Op) error { return nil }},
	ops.OpGateEvidence:       {apply: func(_ *State, _ ops.Op) error { return nil }},
	ops.OpCitationAccepted:   {apply: (*State).applyCitationAccepted, missingTarget: MissingTargetIgnore},
	ops.OpDAGTransition:      {apply: (*State).applyDAGTransition, missingTarget: MissingTargetIgnore},
	ops.OpScopeRename:        {apply: (*State).applyScopeRename, missingTarget: MissingTargetIgnore},
	ops.OpScopeDelete:        {apply: (*State).applyScopeDelete, missingTarget: MissingTargetIgnore},
	ops.OpReparent:           {apply: (*State).applyReparent, missingTarget: MissingTargetIgnore},
	ops.OpAssessmentAttested: {apply: (*State).applyAssessmentAttested, missingTarget: MissingTargetError},
}

// RegisteredOpTypes returns the set of supported op type strings.
func RegisteredOpTypes() []string {
	types := make([]string, 0, len(opHandlers))
	for opType := range opHandlers {
		types = append(types, opType)
	}
	return types
}

// ApplyOp applies a single op to the materialized state by dispatching
// through the registered handler table. Unknown op types return an error.
func (s *State) ApplyOp(op ops.Op) error {
	handler, exists := opHandlers[op.Type]
	if !exists {
		return fmt.Errorf("unknown op type: %s", op.Type)
	}
	if handler.missingTarget != "" {
		if _, ok := s.Issues[op.TargetID]; !ok {
			if handler.missingTarget == MissingTargetIgnore {
				return nil
			}
			return missingTargetError{OpType: op.Type, TargetID: op.TargetID}
		}
	}
	return handler.apply(s, op)
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
		Scope:            ops.DecodeScope(op.Payload.Scope),
		ContextFiles:     ops.DecodeScope(op.Payload.ContextFiles),
		Priority:         op.Payload.Priority,
		EstComplexity:    op.Payload.EstComplexity,
		DefinitionOfDone: op.Payload.DefinitionOfDone,
		Acceptance:       op.Payload.Acceptance,
		Context:          op.Payload.Context,
		SourceCitation:   op.Payload.SourceCitation,
		PreferredModel:   op.Payload.PreferredModel,
		Provenance: Provenance{
			Method:       "decomposed",
			Confidence:   confidenceOrDefault(op.Payload.Confidence),
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
	issue := s.Issues[op.TargetID]
	if claimpkg.ForeignLiveLeaseBlocksChallenger(claimpkg.HeldClaim{
		Status:                     issue.Status,
		ClaimedBy:                  issue.ClaimedBy,
		ClaimedAt:                  issue.ClaimedAt,
		LastHeartbeat:              issue.LastHeartbeat,
		LastClaimingWorkerActivity: issue.LastClaimingWorkerActivity,
		TTLMinutes:                 issue.ClaimTTL,
	}, op.WorkerID, op.Timestamp) {
		return nil
	}
	issue.Status = ops.StatusClaimed
	issue.ClaimedBy = op.WorkerID
	issue.ClaimedAt = op.Timestamp
	issue.ClaimToken = op.Payload.ClaimToken
	issue.ClaimTTL = op.Payload.TTL
	issue.WorktreePath = op.Payload.WorktreePath
	issue.LastHeartbeat = op.Timestamp
	issue.Updated = op.Timestamp
	issue.LastClaimingWorkerActivity = op.Timestamp
	s.promoteParentToInProgress(issue.Parent)
	return nil
}

func (s *State) applyHeartbeat(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	issue.Updated = op.Timestamp
	if claimpkg.ClaimantHeartbeatClocks(issue.ClaimedBy, op.WorkerID) {
		issue.LastHeartbeat = op.Timestamp
		issue.LastClaimingWorkerActivity = op.Timestamp
	}
	return nil
}

func (s *State) applyTransition(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	if !compensationApplies(issue, op) {
		return nil
	}
	newStatus := op.Payload.To
	wasClaimant := op.WorkerID == issue.ClaimedBy
	if newStatus == ops.StatusOpen {
		issue.ClaimedBy = ""
		issue.ClaimedAt = 0
		issue.ClaimToken = ""
		if issue.Status == ops.StatusDone && issue.Outcome != "" {
			issue.PriorOutcomes = append(issue.PriorOutcomes, issue.Outcome)
			issue.Outcome = ""
		}
	}
	if wasClaimant {
		issue.LastClaimingWorkerActivity = op.Timestamp
	}
	applyStatusAssert(issue, newStatus, op.Timestamp)
	issue.WorktreePath = ops.DecodeWorktreeRestore(op.Payload).Apply(issue.WorktreePath)
	restoreLeaseIfMarked(issue, op.Payload)
	if op.Payload.Outcome != "" {
		issue.Outcome = op.Payload.Outcome
	}
	if op.Payload.Branch != "" {
		issue.Branch = op.Payload.Branch
	}
	if op.Payload.PR != "" {
		issue.PR = op.Payload.PR
	}
	return nil
}

func compensationApplies(issue *Issue, op ops.Op) bool {
	if op.Payload.IfClaimToken == "" {
		return true
	}
	return issue.HeldByExactWorkerAndClaimToken(op.WorkerID, op.Payload.IfClaimToken)
}

func applyStatusAssert(issue *Issue, status string, timestamp int64) {
	issue.Status = status
	issue.RollupStatusBefore = ""
	issue.Updated = timestamp
}

func restoreLeaseIfMarked(issue *Issue, p ops.Payload) {
	if !p.RestoreClaim {
		return
	}
	issue.ClaimedBy = p.RestoreClaimedBy
	issue.ClaimedAt = p.RestoreClaimedAt
	issue.ClaimTTL = p.RestoreClaimTTL
	issue.LastHeartbeat = p.RestoreLastHeartbeat
	issue.LastClaimingWorkerActivity = p.RestoreLastClaimingWorkerActivity
	issue.ClaimToken = p.RestoreClaimToken
}

func (s *State) applyNote(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	if hasAppliedNote(issue.Notes, op) {
		return nil
	}
	issue.Notes = append(issue.Notes, Note{
		ID:        resolveNoteID(issue, op),
		WorkerID:  op.WorkerID,
		Timestamp: op.Timestamp,
		Msg:       op.Payload.Msg,
	})
	issue.Updated = op.Timestamp
	return nil
}

func hasAppliedNote(notes []Note, op ops.Op) bool {
	for _, note := range notes {
		if note.WorkerID != op.WorkerID || note.Timestamp != op.Timestamp || note.Msg != op.Payload.Msg {
			continue
		}
		if op.Payload.NoteID == "" || note.ID == op.Payload.NoteID {
			return true
		}
	}
	return false
}

func (s *State) applyNoteDelete(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	for i := range issue.Notes {
		if issue.Notes[i].ID == op.Payload.NoteID {
			issue.Notes[i].Deleted = true
			issue.Updated = op.Timestamp
			break
		}
	}
	return nil
}

func resolveNoteID(issue *Issue, op ops.Op) string {
	if op.Payload.NoteID != "" {
		return op.Payload.NoteID
	}

	base := fmt.Sprintf("note-%d-%s", op.Timestamp, op.WorkerID)
	id := base
	suffix := 2
	for noteIDExists(issue.Notes, id) {
		id = fmt.Sprintf("%s-%d", base, suffix)
		suffix++
	}
	return id
}

func noteIDExists(notes []Note, id string) bool {
	for _, note := range notes {
		if note.ID == id {
			return true
		}
	}
	return false
}

func (s *State) applyLink(op ops.Op) error {
	source := s.Issues[op.TargetID]
	if op.Payload.Rel == "blocked_by" {
		source.BlockedBy = appendUnique(source.BlockedBy, op.Payload.Dep)
		if dep, ok := s.Issues[op.Payload.Dep]; ok {
			dep.Blocks = appendUnique(dep.Blocks, op.TargetID)
		}
	}
	source.Updated = op.Timestamp
	return nil
}

func (s *State) applyUnlink(op ops.Op) error {
	source := s.Issues[op.TargetID]
	if op.Payload.Rel == "blocked_by" {
		source.BlockedBy = removeString(source.BlockedBy, op.Payload.Dep)
		if dep, ok := s.Issues[op.Payload.Dep]; ok {
			dep.Blocks = removeString(dep.Blocks, op.TargetID)
		}
	}
	source.Updated = op.Timestamp
	return nil
}

func (s *State) applyAssign(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	issue.AssignedWorker = op.Payload.AssignedTo
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyDecision(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	decision := Decision{
		Topic:     op.Payload.Topic,
		Choice:    op.Payload.Choice,
		Rationale: op.Payload.Rationale,
		Affects:   op.Payload.Affects,
		WorkerID:  op.WorkerID,
		Timestamp: op.Timestamp,
	}
	if hasDecision(issue.Decisions, decision) {
		return nil
	}
	issue.Decisions = append(issue.Decisions, decision)
	issue.Updated = op.Timestamp
	return nil
}

func hasDecision(decisions []Decision, want Decision) bool {
	for _, decision := range decisions {
		if decision.Topic != want.Topic ||
			decision.Choice != want.Choice ||
			decision.Rationale != want.Rationale ||
			decision.WorkerID != want.WorkerID ||
			decision.Timestamp != want.Timestamp {
			continue
		}
		if slices.Equal(decision.Affects, want.Affects) {
			return true
		}
	}
	return false
}

func (s *State) applyAmend(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	if op.Payload.NodeType != "" {
		issue.Type = op.Payload.NodeType
	}
	if len(op.Payload.Scope) > 0 {
		issue.Scope = ops.DecodeScope(op.Payload.Scope)
	}
	if op.Payload.ClearContextFiles {
		issue.ContextFiles = []string{}
	}
	if op.Payload.ContextFiles != nil {
		issue.ContextFiles = ops.DecodeScope(op.Payload.ContextFiles)
	}
	if len(op.Payload.Acceptance) > 0 && string(op.Payload.Acceptance) != "null" {
		issue.Acceptance = op.Payload.Acceptance
	}
	if op.Payload.DefinitionOfDone != "" {
		issue.DefinitionOfDone = op.Payload.DefinitionOfDone
	}
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applySourceLink(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	link := SourceLink{
		SourceEntryID: op.Payload.SourceID,
		SourceURL:     op.Payload.SourceURL,
		Title:         op.Payload.Title,
	}
	if slices.Contains(issue.SourceLinks, link) {
		return nil
	}
	issue.SourceLinks = append(issue.SourceLinks, link)
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyCitationAccepted(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	acceptance := CitationAcceptance{
		WorkerID:                  op.WorkerID,
		Timestamp:                 op.Timestamp,
		ConfirmedNoninteractively: op.Payload.ConfirmedNoninteractively,
		SourceEntryID:             op.Payload.SourceEntryID,
	}
	if slices.Contains(issue.CitationAcceptances, acceptance) {
		return nil
	}
	issue.CitationAcceptances = append(issue.CitationAcceptances, acceptance)
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyDAGTransition(op ops.Op) error {
	mode := ops.DecodeDAGMode(op)
	switch mode.Era {
	case ops.DAGEraCanonical:
		s.promoteSubtreeConfidence(mode.RootID, mode.Confidence, op.Timestamp)
	default:
		issue, ok := s.Issues[mode.RootID]
		if !ok {
			return nil
		}
		issue.Provenance.DAGConfirmed = mode.Confirmed
		issue.Updated = op.Timestamp
	}
	return nil
}

func (s *State) applyScopeRename(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	updated := make([]string, len(issue.Scope))
	for i, entry := range issue.Scope {
		updated[i] = strings.ReplaceAll(entry, op.Payload.OldPath, op.Payload.NewPath)
	}
	issue.Scope = updated
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyScopeDelete(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	result := make([]string, 0, len(issue.Scope))
	matched := false
	for _, entry := range issue.Scope {
		if entry == op.Payload.DeletedPath {
			matched = true
		} else {
			result = append(result, entry)
		}
	}
	if matched {
		issue.Scope = result
		issue.Updated = op.Timestamp
	}
	return nil
}

func (s *State) applyReparent(op ops.Op) error {
	issue := s.Issues[op.TargetID]
	oldParentID := issue.Parent
	newParentID := op.Payload.Parent

	if oldParentID != "" {
		if oldParent, ok := s.Issues[oldParentID]; ok {
			oldParent.Children = removeString(oldParent.Children, op.TargetID)
			oldParent.Updated = op.Timestamp
		}
	}

	if newParentID != "" {
		if newParent, ok := s.Issues[newParentID]; ok {
			newParent.Children = appendUnique(newParent.Children, op.TargetID)
			newParent.Updated = op.Timestamp
		}
	}

	issue.Parent = newParentID
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyAssessmentAttested(op ops.Op) error {
	issue := s.Issues[op.TargetID]

	var att review.AssessmentAttestation
	if err := json.Unmarshal(op.Payload.Assessment, &att); err != nil {
		return fmt.Errorf("unmarshal assessment attestation: %w", err)
	}

	for _, existing := range issue.AssessmentAttestations {
		if existing.ResultFingerprint == att.ResultFingerprint {
			return nil
		}
	}

	issue.AssessmentAttestations = append(issue.AssessmentAttestations, att)
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) promoteSubtreeConfidence(rootID, targetConfidence string, timestamp int64) {
	root, ok := s.Issues[rootID]
	if !ok {
		return
	}
	root.Provenance.Confidence = targetConfidence
	root.Updated = timestamp
	for _, childID := range root.Children {
		s.promoteSubtreeConfidence(childID, targetConfidence, timestamp)
	}
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

func rollupSatisfied(status string) bool {
	return status == ops.StatusMerged || status == ops.StatusCancelled
}

// RetractDerivedPromotions restores every issue that a previous RunRollup
// promoted to the status that promotion replaced, clearing the marker.
func (s *State) RetractDerivedPromotions() {
	for _, issue := range s.Issues {
		if issue.RollupStatusBefore != "" {
			issue.Status = issue.RollupStatusBefore
			issue.RollupStatusBefore = ""
		}
	}
}

// RunRollup promotes stories/epics to merged when every child has reached a
// terminal state and at least one of them actually shipped.
func (s *State) RunRollup() {
	s.RetractDerivedPromotions()

	inDegree := make(map[string]int)
	queue := make([]string, 0)

	for _, issue := range s.Issues {
		if issue.Type == "task" || issue.Status == ops.StatusMerged || issue.Status == ops.StatusCancelled || len(issue.Children) == 0 {
			continue
		}

		unresolvedCount := 0
		hasMerged := false
		for _, childID := range issue.Children {
			child, ok := s.Issues[childID]
			if !ok || !rollupSatisfied(child.Status) {
				unresolvedCount++
				continue
			}
			if child.Status == ops.StatusMerged {
				hasMerged = true
			}
		}
		inDegree[issue.ID] = unresolvedCount

		if unresolvedCount == 0 && hasMerged {
			queue = append(queue, issue.ID)
		}
	}

	for len(queue) > 0 {
		issueID := queue[0]
		queue = queue[1:]

		issue, ok := s.Issues[issueID]
		if !ok {
			continue
		}

		if issue.Status != ops.StatusMerged {
			issue.RollupStatusBefore = issue.Status
			issue.Status = ops.StatusMerged

			if issue.Parent != "" {
				parent, ok := s.Issues[issue.Parent]
				if !ok {
					continue
				}

				if parent.Type == "task" || parent.Status == ops.StatusMerged || parent.Status == ops.StatusCancelled || len(parent.Children) == 0 {
					continue
				}

				if count, ok := inDegree[parent.ID]; ok {
					inDegree[parent.ID] = count - 1

					if inDegree[parent.ID] == 0 {
						queue = append(queue, parent.ID)
					}
				}
			}
		}
	}
}

// BuildIndex creates the denormalized index from current state.
func (s *State) BuildIndex() Index {
	index := make(Index, len(s.Issues))
	for id, issue := range s.Issues {
		index[id] = IndexEntry{
			Status:         issue.Status,
			Type:           issue.Type,
			Parent:         issue.Parent,
			Children:       issue.Children,
			BlockedBy:      issue.BlockedBy,
			Blocks:         issue.Blocks,
			Assignee:       issue.ClaimedBy,
			AssignedWorker: issue.AssignedWorker,
			Updated:        issue.Updated,
			Title:          issue.Title,
			Outcome:        issue.Outcome,
			Scope:          issue.Scope,
			Branch:         issue.Branch,
			PR:             issue.PR,
		}
	}
	return index
}

func confidenceOrDefault(confidence string) string {
	if confidence == "" {
		return "verified"
	}
	return confidence
}

func appendUnique(slice []string, item string) []string {
	if slices.Contains(slice, item) {
		return slice
	}
	return append(slice, item)
}

func removeString(slice []string, item string) []string {
	result := []string{}
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
