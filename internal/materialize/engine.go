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

// opHandlers maps op type strings to their handler functions.
var opHandlers = map[string]func(*State, ops.Op) error{
	ops.OpCreate:             (*State).applyCreate,
	ops.OpClaim:              (*State).applyClaim,
	ops.OpHeartbeat:          (*State).applyHeartbeat,
	ops.OpTransition:         (*State).applyTransition,
	ops.OpNote:               (*State).applyNote,
	ops.OpNoteDelete:         (*State).applyNoteDelete,
	ops.OpLink:               (*State).applyLink,
	ops.OpUnlink:             (*State).applyUnlink,
	ops.OpDecision:           (*State).applyDecision,
	ops.OpAssign:             (*State).applyAssign,
	ops.OpAmend:              (*State).applyAmend,
	ops.OpSourceLink:         (*State).applySourceLink,
	ops.OpSourceFingerprint:  func(_ *State, _ ops.Op) error { return nil },
	ops.OpGateEvidence:       func(_ *State, _ ops.Op) error { return nil },
	ops.OpCitationAccepted:   (*State).applyCitationAccepted,
	ops.OpDAGTransition:      (*State).applyDAGTransition,
	ops.OpScopeRename:        (*State).applyScopeRename,
	ops.OpScopeDelete:        (*State).applyScopeDelete,
	ops.OpReparent:           (*State).applyReparent,
	ops.OpAssessmentAttested: (*State).applyAssessmentAttested,
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
	return handler(s, op)
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
		Scope:            normalizeScopeEntries(op.Payload.Scope),
		ContextFiles:     normalizeScopeEntries(op.Payload.ContextFiles),
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
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("claim: issue %s not found", op.TargetID)
	}
	if (issue.Status == ops.StatusClaimed || issue.Status == ops.StatusInProgress) &&
		issue.ClaimedBy != "" && issue.ClaimedBy != op.WorkerID {
		ttl := issue.ClaimTTL
		if ttl <= 0 {
			ttl = 60
		}
		if !claimpkg.IsClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.LastClaimingWorkerActivity, ttl, op.Timestamp) {
			// Keep existing active owner; this claim loses the race.
			return nil
		}
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
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	issue.Updated = op.Timestamp
	// LastHeartbeat feeds directly into claim.IsClaimStale / doctor's
	// claimExpired staleness formula, so only the claiming worker's heartbeat
	// may extend it — a non-claimant's heartbeat must not be able to mask a
	// genuinely stale claim.
	if op.WorkerID == issue.ClaimedBy {
		issue.LastHeartbeat = op.Timestamp
		issue.LastClaimingWorkerActivity = op.Timestamp
	}
	return nil
}

func (s *State) applyTransition(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("transition: issue %s not found", op.TargetID)
	}
	// A non-empty IfClaimToken marks this op as a conditional compensating
	// rollback (see ops.Payload.IfClaimToken). Validate it BEFORE any mutation:
	// replay order is not under this op's control (append-only log, last-write-
	// wins), so the only way to make a stale rollback harmless regardless of
	// where it lands is to have replay itself refuse to apply it once the claim
	// it targets no longer holds. Delegate the "is this still the exact claim
	// being compensated for" question to Issue.ClaimHeldBy -- the single
	// canonical predicate shared with cmd/armature's claimStillOwnedBy -- rather
	// than re-deriving field comparisons here. ClaimHeldBy requires
	// Status == StatusClaimed, which subsumes the old terminal-only check
	// (done/merged/cancelled are all simply not "claimed") and additionally
	// covers a live, non-terminal transition (in-progress, blocked) made by a
	// different command in the meantime, since claim-owning commands do not hold
	// the per-issue claim lock against transition commands (acquireClaimLock has
	// exactly one caller). Any mismatch makes this a deterministic no-op: return
	// nil without touching the issue at all.
	if op.Payload.IfClaimToken != "" {
		if !issue.ClaimHeldBy(op.WorkerID, op.Payload.IfClaimToken) {
			return nil
		}
	}
	newStatus := op.Payload.To
	// Capture claimant-ness before any field clearing below: a transition to
	// `open` zeroes ClaimedBy as part of this same op, but the claimant's own
	// release of their claim is still claimant activity and must still bump
	// LastClaimingWorkerActivity.
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
	issue.Status = newStatus
	issue.Updated = op.Timestamp
	// A transition op may carry a WorktreePath ONLY when it is a claim rollback
	// restoring the path that was overwritten by the (now-failed) claim attempt.
	// Normal transitions never set Payload.WorktreePath, so this never clobbers a
	// live path; restoring it keeps an active same-worker retry's claim pointing
	// at its real (possibly legacy) worktree instead of a just-removed canonical one.
	//
	// ClearWorktreePath is the explicit clear-signal a rollback uses when the
	// path to restore is empty: an empty Payload.WorktreePath is indistinguishable
	// from "no change" (an omitted field), so a rollback that must put back an
	// EMPTY pre-claim path sets ClearWorktreePath instead, and it takes precedence.
	switch {
	case op.Payload.ClearWorktreePath:
		issue.WorktreePath = ""
	case op.Payload.WorktreePath != "":
		issue.WorktreePath = op.Payload.WorktreePath
	}
	if op.Payload.RestoreClaim {
		issue.ClaimedBy = op.Payload.RestoreClaimedBy
		issue.ClaimedAt = op.Payload.RestoreClaimedAt
		issue.ClaimTTL = op.Payload.RestoreClaimTTL
		issue.LastHeartbeat = op.Payload.RestoreLastHeartbeat
		issue.LastClaimingWorkerActivity = op.Payload.RestoreLastClaimingWorkerActivity
		issue.ClaimToken = op.Payload.RestoreClaimToken
	}
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

func (s *State) applyNote(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
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
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
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

func (s *State) applyUnlink(op ops.Op) error {
	source, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("unlink: source issue %s not found", op.TargetID)
	}
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
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		// Tolerate unknown issues (e.g. assign op before create op in log)
		return nil
	}
	issue.AssignedWorker = op.Payload.AssignedTo
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyDecision(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
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
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	if op.Payload.NodeType != "" {
		issue.Type = op.Payload.NodeType
	}
	if len(op.Payload.Scope) > 0 {
		issue.Scope = normalizeScopeEntries(op.Payload.Scope)
	}
	if op.Payload.ClearContextFiles {
		issue.ContextFiles = []string{}
	}
	if op.Payload.ContextFiles != nil {
		issue.ContextFiles = normalizeScopeEntries(op.Payload.ContextFiles)
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
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
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
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
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
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	// New behavior: when IssueID is set, walk the subtree and promote confidence.
	if op.Payload.IssueID != "" {
		targetConfidence := op.Payload.To
		if targetConfidence == "" {
			targetConfidence = "verified"
		}
		s.promoteSubtreeConfidence(op.Payload.IssueID, targetConfidence, op.Timestamp)
		return nil
	}
	// Legacy behavior: set DAGConfirmed flag on the single target issue.
	issue.Provenance.DAGConfirmed = op.Payload.Confirmed
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyScopeRename(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	updated := make([]string, len(issue.Scope))
	for i, entry := range issue.Scope {
		updated[i] = strings.ReplaceAll(entry, op.Payload.OldPath, op.Payload.NewPath)
	}
	issue.Scope = updated
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyScopeDelete(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
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

// applyReparent moves an issue to a new parent, updating the children lists
// of both the old parent (removing the issue) and the new parent (adding it).
func (s *State) applyReparent(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	oldParentID := issue.Parent
	newParentID := op.Payload.Parent

	// Remove from old parent's children list.
	if oldParentID != "" {
		if oldParent, ok := s.Issues[oldParentID]; ok {
			oldParent.Children = removeString(oldParent.Children, op.TargetID)
			oldParent.Updated = op.Timestamp
		}
	}

	// Add to new parent's children list.
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

// applyAssessmentAttested replays an assessment attestation op by unmarshaling
// the assessment, deduplicating by ResultFingerprint, and appending to the issue's
// assessment attestations list.
func (s *State) applyAssessmentAttested(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("assessment-attested: issue %s not found", op.TargetID)
	}

	// Unmarshal the assessment attestation from op.Payload.Assessment
	var att review.AssessmentAttestation
	if err := json.Unmarshal(op.Payload.Assessment, &att); err != nil {
		return fmt.Errorf("unmarshal assessment attestation: %w", err)
	}

	// Deduplicate by ResultFingerprint
	for _, existing := range issue.AssessmentAttestations {
		if existing.ResultFingerprint == att.ResultFingerprint {
			return nil // duplicate, skip
		}
	}

	// Append the attestation
	issue.AssessmentAttestations = append(issue.AssessmentAttestations, att)
	issue.Updated = op.Timestamp
	return nil
}

// promoteSubtreeConfidence walks the subtree rooted at rootID and sets
// Provenance.Confidence to targetConfidence on every node in the subtree.
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

// rollupSatisfied reports whether a child's status no longer blocks its parent
// from rolling up. Both merged and cancelled are terminal; treating cancelled as
// outstanding would leave the parent unresolvable for the life of the repo.
func rollupSatisfied(status string) bool {
	return status == ops.StatusMerged || status == ops.StatusCancelled
}

// RunRollup promotes stories/epics to merged when every child has reached a
// terminal state and at least one of them actually shipped.
//
// A cancelled child satisfies rollup: cancelled is terminal, so counting it as
// outstanding would strand its parent permanently (see TOPTIER-B1). This mirrors
// the predicate already used by internal/worktree/reconcile.go. The
// at-least-one-merged guard keeps a wholly-descoped parent from claiming
// delivery when nothing was built.
// Uses a single-pass topological sort algorithm achieving O(n) time complexity.
// Algorithm: compute in-degree (unmerged children count) for each parent,
// start with parents that have all merged children, and propagate promotions upward.
func (s *State) RunRollup() {
	// Compute initial in-degree (unmerged children count) for each parent
	inDegree := make(map[string]int)
	queue := make([]string, 0)

	// First pass: count unmerged children for all non-task parents
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

		// Every child is terminal and something shipped: ready to promote.
		if unresolvedCount == 0 && hasMerged {
			queue = append(queue, issue.ID)
		}
	}

	// Second pass: process queue (topological sort, bottom-up)
	// Each issue in the queue has all children merged
	for len(queue) > 0 {
		issueID := queue[0]
		queue = queue[1:]

		issue, ok := s.Issues[issueID]
		if !ok {
			continue
		}

		// Promote this issue to merged
		if issue.Status != ops.StatusMerged {
			issue.Status = ops.StatusMerged

			// Check parent: decrement its in-degree
			if issue.Parent != "" {
				parent, ok := s.Issues[issue.Parent]
				if !ok {
					continue
				}

				// Skip if parent is a task or already terminal
				if parent.Type == "task" || parent.Status == ops.StatusMerged || parent.Status == ops.StatusCancelled || len(parent.Children) == 0 {
					continue
				}

				// Decrement parent's in-degree
				if count, ok := inDegree[parent.ID]; ok {
					inDegree[parent.ID] = count - 1

					// If parent now has all children merged, add to queue
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

// confidenceOrDefault returns the confidence value from an op payload,
// defaulting to "verified" when the field is absent or empty.
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

// normalizeScopeEntries splits any comma-separated scope entries into individual paths.
// Legacy ops stored scope as a single joined string (e.g. "a.go, b.go"); this ensures
// the materialized state always holds one path per element.
func normalizeScopeEntries(scope []string) []string {
	result := make([]string, 0, len(scope))
	for _, entry := range scope {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, ", ") {
			for part := range strings.SplitSeq(entry, ", ") {
				if part = strings.TrimSpace(part); part != "" {
					result = append(result, part)
				}
			}
		} else if entry = strings.TrimSpace(entry); entry != "" {
			result = append(result, entry)
		}
	}
	return result
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
