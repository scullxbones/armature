// Package ops defines the op-log schema (typed, append-only events), and provides parsing, commit, push, and rate-limiting for writing and reading that log.
package ops

import "encoding/json"

// Op types — all 10 defined in architecture doc section 3, plus OpAssign for E3.
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

// Issue statuses.
// Note: the op type handler registry (mapping op type string to handler function)
// lives in materialize.RegisteredOpTypes() — see internal/materialize/engine.go.
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
	// create
	Title             string          `json:"title,omitempty"`
	Parent            string          `json:"parent,omitempty"`
	NodeType          string          `json:"type,omitempty"`
	Scope             []string        `json:"scope,omitempty"`
	Acceptance        json.RawMessage `json:"acceptance,omitempty"`
	DefinitionOfDone  string          `json:"definition_of_done,omitempty"`
	ContextFiles      []string        `json:"context_files,omitempty"`
	ClearContextFiles bool            `json:"clear_context_files,omitempty"`
	Context           json.RawMessage `json:"context,omitempty"`
	SourceCitation    json.RawMessage `json:"source_citation,omitempty"`
	Priority          string          `json:"priority,omitempty"`
	EstComplexity     string          `json:"estimated_complexity,omitempty"`

	// claim
	TTL          int    `json:"ttl,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	// ClaimToken is a unique per-claim nonce generated at claim time (see
	// cmd/armature's newClaimToken). It gives a claim an identity that survives
	// same-worker re-claims within the same wall-clock second (ClaimedAt alone
	// has only 1-second resolution) and lets a compensating transition (below)
	// name the exact claim it is rolling back, rather than "the current claim
	// by this worker" which op-log replay ordering can make ambiguous.
	ClaimToken string `json:"claim_token,omitempty"`
	// RestoreClaim marks a transition as an explicit compensating claim
	// snapshot. It is used by a failed claim retry to restore every lease field
	// captured before the retry appended its claim op. A marker is required so
	// zero values are still meaningful and ordinary transitions remain
	// backward-compatible.
	RestoreClaim                      bool   `json:"restore_claim,omitempty"`
	RestoreClaimedBy                  string `json:"restore_claimed_by,omitempty"`
	RestoreClaimedAt                  int64  `json:"restore_claimed_at,omitempty"`
	RestoreClaimTTL                   int    `json:"restore_claim_ttl,omitempty"`
	RestoreLastHeartbeat              int64  `json:"restore_last_heartbeat,omitempty"`
	RestoreLastClaimingWorkerActivity int64  `json:"restore_last_claiming_worker_activity,omitempty"`
	// RestoreClaimToken restores the pre-claim-attempt ClaimToken alongside the
	// other Restore* lease fields, for the same reason: an active same-worker
	// retry's rollback must put back its OWN prior claim's token, not leave the
	// just-superseded token in place.
	RestoreClaimToken string `json:"restore_claim_token,omitempty"`
	// ClearWorktreePath is an explicit clear-signal for a transition op (used by
	// claim rollback): when true, applyTransition sets the issue's WorktreePath
	// back to empty. This distinguishes "restore to empty" from "no change" —
	// an empty WorktreePath string alone means "no change" because it is
	// indistinguishable from an absent field in the op log. Append-only,
	// backward compatible: absent in every legacy op, decoding to false.
	ClearWorktreePath bool `json:"clear_worktree_path,omitempty"`

	// heartbeat
	Source string `json:"source,omitempty"`

	// transition
	To                  string `json:"to,omitempty"`
	Outcome             string `json:"outcome,omitempty"`
	Branch              string `json:"branch,omitempty"`
	PR                  string `json:"pr,omitempty"`
	SkippedDeliveryGate bool   `json:"skipped_delivery_gate,omitempty"`
	// IfClaimToken marks a transition op as a conditional compensating rollback
	// (see cmd/armature's rollbackClaim). When non-empty, materialize.applyTransition
	// applies the op ONLY IF materialize.Issue.ClaimHeldBy(WorkerID, IfClaimToken)
	// reports true for the target issue — the single canonical ownership
	// predicate shared with cmd/armature's claimStillOwnedBy, requiring the
	// issue's current Status to be exactly "claimed" AND its ClaimedBy/ClaimToken
	// to match this op's WorkerID/IfClaimToken exactly; otherwise the op is a
	// deterministic no-op. Requiring Status == "claimed" subsumes the older
	// terminal-only check (done/merged/cancelled are all simply not "claimed")
	// and additionally guards against a live, non-terminal transition (e.g. to
	// in-progress or blocked) landing between the claim and the rollback, since
	// claim-owning commands do not hold the per-issue claim lock against
	// transition commands. This is what makes the compensating op safe
	// regardless of where it lands in the append-only, last-write-wins op log: a
	// second worker's legitimate takeover (a new claim op with a different
	// token), or any other command's status change, can never be erased by a
	// stale rollback, no matter the order ops are appended or replayed in.
	// Absent (empty string) on every ordinary transition and every legacy op,
	// which replay unconditionally as before (backward compatible).
	IfClaimToken string `json:"if_claim_token,omitempty"`

	// note
	Msg    string `json:"msg,omitempty"`
	NoteID string `json:"note_id,omitempty"`

	// link
	Dep string `json:"dep,omitempty"`
	Rel string `json:"rel,omitempty"`

	// source-link
	SourceID  string `json:"source_id,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
	Section   string `json:"section,omitempty"`
	Anchor    string `json:"anchor,omitempty"`
	Quote     string `json:"quote,omitempty"`

	// source-fingerprint
	SHA       string `json:"sha,omitempty"`
	VersionID string `json:"version_id,omitempty"`
	Provider  string `json:"provider,omitempty"`

	// dag-transition (confidence promotion)
	IssueID                   string   `json:"issue_id,omitempty"`
	Confirmed                 bool     `json:"confirmed,omitempty"`
	ConfirmedNoninteractively bool     `json:"confirmed_noninteractively,omitempty"`
	UncoveredAcknowledged     []string `json:"uncovered_acknowledged,omitempty"`

	// decision
	Topic     string   `json:"topic,omitempty"`
	Choice    string   `json:"choice,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
	Affects   []string `json:"affects,omitempty"`

	// assign
	AssignedTo string `json:"assigned_to,omitempty"`

	// create — confidence level: "draft" or "verified" (default "verified" when absent)
	Confidence string `json:"confidence,omitempty"`

	// scope-rename
	OldPath string `json:"old_path,omitempty"`
	NewPath string `json:"new_path,omitempty"`

	// scope-delete
	DeletedPath string `json:"deleted_path,omitempty"`

	// citation-accepted — source entry ID from the accept-citation command
	// (populated when --source is passed to arm accept-citation)
	SourceEntryID string `json:"source_entry_id,omitempty"`

	// create — preferred model hint for the assigned agent
	PreferredModel string `json:"preferred_model,omitempty"`

	// assessment-attested
	Assessment json.RawMessage `json:"assessment,omitempty"`
}
