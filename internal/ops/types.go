// Package ops defines the op-log schema (typed, append-only events), and provides parsing, commit, push, and rate-limiting for writing and reading that log.
package ops

import (
	"bytes"
	"encoding/json"
)

const (
	OpCreate            = "create"
	OpClaim             = "claim"
	OpHeartbeat         = "heartbeat"
	OpTransition        = "transition"
	OpNote              = "note"
	OpNoteDelete        = "note-delete"
	OpLink              = "link"
	OpUnlink            = "unlink"
	OpSourceLink        = "source-link"
	OpSourceFingerprint = "source-fingerprint"
	OpDAGTransition     = "dag-transition"
	OpDecision          = "decision"
	OpAssign            = "assign"
	OpAmend             = "amend"
	OpCitationAccepted  = "citation-accepted"
	OpScopeRename       = "scope-rename"
	OpScopeDelete       = "scope-delete"

	// OpReparent moves an issue to a new parent.
	OpReparent = "reparent"

	// OpAssessmentAttested records a conformance assessment attestation.
	OpAssessmentAttested = "assessment-attested"
)

// IsAuditOnly reports ops that record evidence or metadata without mutating
// issue DAG state. Their target_id is not an issue ID (source UUID, gate
// profile name) and must not be treated as unknown or as an orphaned issue.
func IsAuditOnly(opType string) bool {
	switch opType {
	case OpSourceFingerprint, OpGateEvidence:
		return true
	default:
		return false
	}
}

// IsTerminalStatus reports issue statuses that are finished in the DAG
// (done, merged, cancelled). Worktree GC uses a stricter merged|cancelled
// check and must not call this helper.
func IsTerminalStatus(status string) bool {
	switch status {
	case StatusDone, StatusMerged, StatusCancelled:
		return true
	default:
		return false
	}
}

const (
	StatusOpen       = "open"
	StatusClaimed    = "claimed"
	StatusInProgress = "in-progress"
	StatusDone       = "done"
	StatusMerged     = "merged"
	StatusBlocked    = "blocked"
	StatusCancelled  = "cancelled"
)

// ValidTransitionTargets is the set of statuses accepted by the transition command.
var ValidTransitionTargets = map[string]bool{
	StatusOpen:       true,
	StatusInProgress: true,
	StatusDone:       true,
	StatusMerged:     true,
	StatusBlocked:    true,
	StatusCancelled:  true,
}

// Op represents a single parsed operation from the log.
type Op struct {
	Type      string
	TargetID  string
	Timestamp int64
	WorkerID  string
	Payload   Payload
}

// Payload holds all possible payload fields across op types.
// Only relevant fields are populated for each op type.
type Payload struct {
	Title                             string          `json:"title,omitempty"`
	Parent                            string          `json:"parent,omitempty"`
	NodeType                          string          `json:"type,omitempty"`
	Scope                             []string        `json:"scope,omitempty"`
	Acceptance                        json.RawMessage `json:"acceptance,omitempty"`
	DefinitionOfDone                  string          `json:"definition_of_done,omitempty"`
	ContextFiles                      []string        `json:"context_files,omitempty"`
	ClearContextFiles                 bool            `json:"clear_context_files,omitempty"`
	Context                           json.RawMessage `json:"context,omitempty"`
	SourceCitation                    json.RawMessage `json:"source_citation,omitempty"`
	Priority                          string          `json:"priority,omitempty"`
	EstComplexity                     string          `json:"estimated_complexity,omitempty"`
	TTL                               int             `json:"ttl,omitempty"`
	WorktreePath                      string          `json:"worktree_path,omitempty"`
	ClaimToken                        string          `json:"claim_token,omitempty"`
	RestoreClaim                      bool            `json:"restore_claim,omitempty"`
	RestoreClaimedBy                  string          `json:"restore_claimed_by,omitempty"`
	RestoreClaimedAt                  int64           `json:"restore_claimed_at,omitempty"`
	RestoreClaimTTL                   int             `json:"restore_claim_ttl,omitempty"`
	RestoreLastHeartbeat              int64           `json:"restore_last_heartbeat,omitempty"`
	RestoreLastClaimingWorkerActivity int64           `json:"restore_last_claiming_worker_activity,omitempty"`
	RestoreClaimToken                 string          `json:"restore_claim_token,omitempty"`
	ClearWorktreePath                 bool            `json:"clear_worktree_path,omitempty"`
	Source                            string          `json:"source,omitempty"`
	To                                string          `json:"to,omitempty"`
	Outcome                           string          `json:"outcome,omitempty"`
	InputTokens                       int             `json:"input_tokens,omitempty"`
	OutputTokens                      int             `json:"output_tokens,omitempty"`
	Branch                            string          `json:"branch,omitempty"`
	PR                                string          `json:"pr,omitempty"`
	SkippedDeliveryGate               bool            `json:"skipped_delivery_gate,omitempty"`
	SkippedValidateGate               bool            `json:"skipped_validate_gate,omitempty"`
	IfClaimToken                      string          `json:"if_claim_token,omitempty"`
	Msg                               string          `json:"msg,omitempty"`
	NoteID                            string          `json:"note_id,omitempty"`
	Dep                               string          `json:"dep,omitempty"`
	Rel                               string          `json:"rel,omitempty"`
	SourceID                          string          `json:"source_id,omitempty"`
	SourceURL                         string          `json:"source_url,omitempty"`
	Section                           string          `json:"section,omitempty"`
	Anchor                            string          `json:"anchor,omitempty"`
	Quote                             string          `json:"quote,omitempty"`
	SHA                               string          `json:"sha,omitempty"`
	VersionID                         string          `json:"version_id,omitempty"`
	Provider                          string          `json:"provider,omitempty"`
	IssueID                           string          `json:"issue_id,omitempty"`
	Confirmed                         bool            `json:"confirmed,omitempty"`
	ConfirmedNoninteractively         bool            `json:"confirmed_noninteractively,omitempty"`
	UncoveredAcknowledged             []string        `json:"uncovered_acknowledged,omitempty"`
	Topic                             string          `json:"topic,omitempty"`
	Choice                            string          `json:"choice,omitempty"`
	Rationale                         string          `json:"rationale,omitempty"`
	Affects                           []string        `json:"affects,omitempty"`
	AssignedTo                        string          `json:"assigned_to,omitempty"`
	Confidence                        string          `json:"confidence,omitempty"`
	OldPath                           string          `json:"old_path,omitempty"`
	NewPath                           string          `json:"new_path,omitempty"`
	DeletedPath                       string          `json:"deleted_path,omitempty"`
	SourceEntryID                     string          `json:"source_entry_id,omitempty"`
	PreferredModel                    string          `json:"preferred_model,omitempty"`
	Assessment                        json.RawMessage `json:"assessment,omitempty"`
}

// PayloadsEqual reports whether a and b marshal to identical JSON bytes.
// encoding/json omitempty means absent optional fields (including token
// counts on legacy transition ops) match explicit zeros and do not need a
// separate equality rule.
func PayloadsEqual(a, b Payload) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}

// LastTransitionPayload returns the payload of the last transition op for
// issueID, if any. Scan is in log order; last write wins.
func LastTransitionPayload(all []Op, issueID string) (Payload, bool) {
	var last Payload
	found := false
	for _, op := range all {
		if op.Type == OpTransition && op.TargetID == issueID {
			last = op.Payload
			found = true
		}
	}
	return last, found
}

// RecordedTransitionPayload is the payload that currently represents the
// issue's recorded transition state. When the last transition still names
// this status, that op's payload is used in full, so optional fields such as
// input_tokens/output_tokens participate in equality only if they were
// already recorded. Otherwise the payload is synthesized from materialized
// status fields without inventing token counts or other transition-only
// flags.
func RecordedTransitionPayload(status, outcome, branch, pr string, last Payload, hasLast bool) Payload {
	if hasLast && last.To == status {
		return last
	}
	return Payload{To: status, Outcome: outcome, Branch: branch, PR: pr}
}

// IdenticalTransition reports whether proposed is a no-op against issueID's
// current status and recorded transition payload.
func IdenticalTransition(all []Op, issueID, status, outcome, branch, pr string, proposed Payload) bool {
	if status != proposed.To {
		return false
	}
	last, hasLast := LastTransitionPayload(all, issueID)
	return PayloadsEqual(proposed, RecordedTransitionPayload(status, outcome, branch, pr, last, hasLast))
}
