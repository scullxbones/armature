package materialize

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	claimpkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
)

// ClaimStale reports whether this issue's claim has expired as of now (Unix
// seconds), delegating to the shared claim.IsClaimStale expiry formula so every
// caller — worktree reconciliation, doctor, claim races — agrees on staleness
// from a single source of truth. Exposed here (rather than importing claim
// directly from packages like internal/worktree, whose depguard boundary forbids
// it) because materialize already owns the claim sub-domain.
func (i *Issue) ClaimStale(now int64) bool {
	return claimpkg.IsClaimStale(i.ClaimedAt, i.LastHeartbeat, i.LastClaimingWorkerActivity, i.ClaimTTL, now)
}

// ClaimHeldBy reports whether this issue is, right now, held by exactly the
// claim identified by workerID and claimToken.
//
// This is the SINGLE canonical "do I still own this claim?" predicate for
// the whole codebase. Before this method existed, the same check was
// written out by hand in two places -- cmd/armature's claimStillOwnedBy and
// this package's applyTransition IfClaimToken guard -- and the two copies
// had already drifted (only one of them checked for a terminal status).
// Every review round before this one patched whichever copy the round's
// finding happened to land in, and the next round found the next field the
// OTHER copy was still missing. Delegating both call sites to this one
// method is the fix for the class of bug, not just the latest instance:
// there is now exactly one place this logic can be written, so it cannot
// disagree with itself again. Do not reintroduce a second, ad-hoc field
// comparison anywhere else -- call this method instead.
//
// Requiring Status == ops.StatusClaimed is what makes this safe, and it
// deliberately SUBSUMES the old "not terminal" check: done, merged, and
// cancelled are all simply not ops.StatusClaimed, so a separate terminal
// check is redundant and must not be "restored" alongside this method.
// It also closes the gap the old terminal-only check missed: an issue that
// has moved on to in-progress or blocked -- a live, non-terminal transition
// made by a DIFFERENT command after this claim was won -- is just as much
// "no longer this claim" as a terminal one, because claim-owning commands
// (arm transition et al.) do not take the per-issue claim lock
// (acquireClaimLock has exactly one caller, in cmd/armature/claim.go) and so
// can race a claim's own post-claim provisioning/rollback path.
//
// This is safe on the legitimate path because applyClaim sets
// Status = ops.StatusClaimed unconditionally on a WON claim, and a claim
// that LOSES the race returns early -- the command exits before any
// provisioning or rollback runs, so a rollback for a losing claim is never
// even attempted.
//
// A nil receiver or an empty claimToken always reports false (fail safe):
// an empty token can never legitimately match (ClaimToken is always
// non-empty once a claim is won), so treating it as "no claim" here rather
// than matching a zero-valued/legacy ClaimToken field closes that edge case
// at the one place it can ever matter, instead of relying on every caller to
// avoid passing an empty token.
func (i *Issue) ClaimHeldBy(workerID, claimToken string) bool {
	if i == nil || claimToken == "" {
		return false
	}
	return i.Status == ops.StatusClaimed &&
		i.ClaimedBy == workerID &&
		i.ClaimToken == claimToken
}

// Issue represents the full materialized state of a single work item.
type Issue struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	// RollupStatusBefore records the status RunRollup replaced when it promoted
	// this issue to merged, marking that promotion as derived rather than
	// asserted by an op. RunRollup restores it before recomputing, so a promoted
	// parent whose child later leaves a terminal state is retracted instead of
	// latching merged. Empty for every issue whose status came from the log —
	// including one an op actually transitioned to merged, which rollup must
	// never walk back. See TOPTIER-B1.
	RollupStatusBefore     string                         `json:"rollup_status_before,omitempty"`
	Title                  string                         `json:"title"`
	Parent                 string                         `json:"parent,omitempty"`
	Children               []string                       `json:"children"`
	BlockedBy              []string                       `json:"blocked_by"`
	Blocks                 []string                       `json:"blocks"`
	Assignee               string                         `json:"assignee,omitempty"`
	Priority               string                         `json:"priority,omitempty"`
	EstComplexity          string                         `json:"estimated_complexity,omitempty"`
	DefinitionOfDone       string                         `json:"definition_of_done,omitempty"`
	Scope                  []string                       `json:"scope"`
	ContextFiles           []string                       `json:"context_files,omitempty"`
	Acceptance             json.RawMessage                `json:"acceptance,omitempty"`
	Context                json.RawMessage                `json:"context,omitempty"`
	SourceCitation         json.RawMessage                `json:"source_citation,omitempty"`
	Provenance             Provenance                     `json:"provenance"`
	DecisionRefs           []string                       `json:"decision_refs"`
	Outcome                string                         `json:"outcome,omitempty"`
	PriorOutcomes          []string                       `json:"prior_outcomes,omitempty"`
	Notes                  []Note                         `json:"notes,omitempty"`
	Decisions              []Decision                     `json:"decisions,omitempty"`
	SourceLinks            []SourceLink                   `json:"source_links,omitempty"`
	CitationAcceptances    []CitationAcceptance           `json:"citation_acceptances,omitempty"`
	AssessmentAttestations []review.AssessmentAttestation `json:"assessment_attestations,omitempty"`
	ClaimedBy              string                         `json:"claimed_by,omitempty"`
	ClaimedAt              int64                          `json:"claimed_at,omitempty"`
	// ClaimToken is the unique per-claim nonce (see ops.Payload.ClaimToken) of
	// the claim currently held. Cleared whenever ClaimedBy/ClaimedAt are
	// cleared (transition to open). Used by applyTransition to validate a
	// compensating rollback op is still targeting the exact claim it was
	// written to compensate for, rather than a later one. Omitted (empty) for
	// issues materialized from op logs written before this field existed.
	ClaimToken    string `json:"claim_token,omitempty"`
	ClaimTTL      int    `json:"claim_ttl,omitempty"`
	LastHeartbeat int64  `json:"last_heartbeat,omitempty"`
	// LastClaimingWorkerActivity is bumped only by applyClaim, applyHeartbeat,
	// and applyTransition, and only when the op's WorkerID matches ClaimedBy.
	// Unlike Updated (which every op handler bumps, regardless of which worker
	// authored the op), this field isolates liveness signal attributable to the
	// worker that actually holds the claim, so a third party's note/link/etc.
	// on the issue can never be mistaken for the claiming worker still being
	// alive. See docs/design/recovery-state-machine.md.
	LastClaimingWorkerActivity int64  `json:"last_claiming_worker_activity,omitempty"`
	WorktreePath               string `json:"worktree_path,omitempty"`
	Branch                     string `json:"branch,omitempty"`
	PR                         string `json:"pr,omitempty"`
	AssignedWorker             string `json:"assigned_worker,omitempty"`
	PreferredModel             string `json:"preferred_model,omitempty"`
	Updated                    int64  `json:"updated"`
}

type Provenance struct {
	Method       string `json:"method"`
	Confidence   string `json:"confidence"`
	SourceWorker string `json:"source_worker"`
	DAGConfirmed bool   `json:"dag_confirmed,omitempty"`
}

// SourceLink records a reference from an issue to an external source entry.
type SourceLink struct {
	SourceEntryID string `json:"source_entry_id"`
	SourceURL     string `json:"source_url,omitempty"`
	Title         string `json:"title,omitempty"`
}

type Note struct {
	ID        string `json:"id,omitempty"`
	WorkerID  string `json:"worker_id"`
	Timestamp int64  `json:"timestamp"`
	Msg       string `json:"msg"`
	Deleted   bool   `json:"deleted,omitempty"`
}

type Decision struct {
	Topic     string   `json:"topic"`
	Choice    string   `json:"choice"`
	Rationale string   `json:"rationale"`
	Affects   []string `json:"affects"`
	WorkerID  string   `json:"worker_id"`
	Timestamp int64    `json:"timestamp"`
}

// CitationAcceptance records that a worker accepted a source citation for an issue.
type CitationAcceptance struct {
	WorkerID                  string `json:"worker_id"`
	Timestamp                 int64  `json:"timestamp"`
	ConfirmedNoninteractively bool   `json:"confirmed_noninteractively,omitempty"`
	SourceEntryID             string `json:"source_entry_id,omitempty"`
}

// IndexEntry is the denormalized summary stored in index.json.
type IndexEntry struct {
	Status         string   `json:"status"`
	Type           string   `json:"type"`
	Parent         string   `json:"parent,omitempty"`
	Children       []string `json:"children,omitempty"`
	BlockedBy      []string `json:"blocked_by,omitempty"`
	Blocks         []string `json:"blocks,omitempty"`
	Assignee       string   `json:"assignee,omitempty"`
	AssignedWorker string   `json:"assigned_worker,omitempty"`
	Updated        int64    `json:"updated"`
	Title          string   `json:"title"`
	Outcome        string   `json:"outcome,omitempty"`
	Scope          []string `json:"scope,omitempty"`
	Branch         string   `json:"branch,omitempty"`
	PR             string   `json:"pr,omitempty"`
}

type Index map[string]IndexEntry

func WriteIssue(issuesDir string, issue Issue) error {
	return adapters.WriteIssueJSON(issuesDir, issue.ID, issue)
}

func LoadIssue(path string) (Issue, error) {
	var issue Issue
	err := adapters.LoadIssueJSON(path, &issue)
	if err != nil {
		return Issue{}, err
	}
	issue.Scope = normalizeScopeEntries(issue.Scope)
	issue.ContextFiles = normalizeScopeEntries(issue.ContextFiles)
	return issue, nil
}

// LoadAllIssues loads all previously materialized issues from the given directory.
// Returns a map of issueID -> *Issue and an error if reading fails.
func LoadAllIssues(issuesDir string) (map[string]*Issue, error) {
	issues := make(map[string]*Issue)

	issueIDs, err := adapters.ReadIssuesDir(issuesDir)
	if err != nil {
		return nil, err
	}

	for _, issueID := range issueIDs {
		issuePath := filepath.Join(issuesDir, issueID+".json")
		issue, err := LoadIssue(issuePath)
		if err != nil {
			return nil, fmt.Errorf("load issue %s: %w", issueID, err)
		}
		issues[issueID] = &issue
	}

	return issues, nil
}

func WriteIndex(path string, index Index) error {
	return adapters.WriteCheckpointJSON(path, index)
}

func LoadIndex(path string) (Index, error) {
	var index Index
	if err := adapters.LoadCheckpointJSON(path, &index); err != nil {
		return nil, err
	}
	if index == nil {
		index = make(Index)
	}
	return index, nil
}
