package materialize

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/review"
)

// Issue represents the full materialized state of a single work item.
type Issue struct {
	ID                     string                         `json:"id"`
	Type                   string                         `json:"type"`
	Status                 string                         `json:"status"`
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
	ClaimTTL               int                            `json:"claim_ttl,omitempty"`
	LastHeartbeat          int64                          `json:"last_heartbeat,omitempty"`
	// LastClaimingWorkerActivity is bumped only by applyClaim, applyHeartbeat,
	// and applyTransition, and only when the op's WorkerID matches ClaimedBy.
	// Unlike Updated (which every op handler bumps, regardless of which worker
	// authored the op), this field isolates liveness signal attributable to the
	// worker that actually holds the claim, so a third party's note/link/etc.
	// on the issue can never be mistaken for the claiming worker still being
	// alive. See docs/design/recovery-state-machine.md.
	LastClaimingWorkerActivity int64  `json:"last_claiming_worker_activity,omitempty"`
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
