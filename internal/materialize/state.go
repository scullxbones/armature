package materialize

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Issue represents the full materialized state of a single work item.
type Issue struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Status           string          `json:"status"`
	Title            string          `json:"title"`
	Parent           string          `json:"parent,omitempty"`
	Children         []string        `json:"children"`
	BlockedBy        []string        `json:"blocked_by"`
	Blocks           []string        `json:"blocks"`
	Assignee         string          `json:"assignee,omitempty"`
	Priority         string          `json:"priority,omitempty"`
	EstComplexity    string          `json:"estimated_complexity,omitempty"`
	DefinitionOfDone string          `json:"definition_of_done,omitempty"`
	Scope            []string        `json:"scope"`
	ContextFiles     []string        `json:"context_files,omitempty"`
	Acceptance       json.RawMessage `json:"acceptance,omitempty"`
	Context          json.RawMessage `json:"context,omitempty"`
	SourceCitation   json.RawMessage `json:"source_citation,omitempty"`
	Provenance       Provenance      `json:"provenance"`
	DecisionRefs     []string        `json:"decision_refs"`
	Outcome          string          `json:"outcome,omitempty"`
	PriorOutcomes    []string        `json:"prior_outcomes,omitempty"`
	Notes            []Note          `json:"notes,omitempty"`
	Decisions        []Decision      `json:"decisions,omitempty"`
	SourceLinks      []SourceLink    `json:"source_links,omitempty"`
	ClaimedBy        string          `json:"claimed_by,omitempty"`
	ClaimedAt        int64           `json:"claimed_at,omitempty"`
	ClaimTTL         int             `json:"claim_ttl,omitempty"`
	LastHeartbeat    int64           `json:"last_heartbeat,omitempty"`
	Branch           string          `json:"branch,omitempty"`
	PR               string          `json:"pr,omitempty"`
	AssignedWorker   string          `json:"assigned_worker,omitempty"`
	Updated          int64           `json:"updated"`
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
	WorkerID  string `json:"worker_id"`
	Timestamp int64  `json:"timestamp"`
	Msg       string `json:"msg"`
}

type Decision struct {
	Topic     string   `json:"topic"`
	Choice    string   `json:"choice"`
	Rationale string   `json:"rationale"`
	Affects   []string `json:"affects"`
	WorkerID  string   `json:"worker_id"`
	Timestamp int64    `json:"timestamp"`
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
	data, err := json.MarshalIndent(issue, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal issue: %w", err)
	}
	path := filepath.Join(issuesDir, issue.ID+".json")
	return os.WriteFile(path, data, 0644)
}

func LoadIssue(path string) (Issue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Issue{}, err
	}
	var issue Issue
	return issue, json.Unmarshal(data, &issue)
}

func WriteIndex(path string, index Index) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func LoadIndex(path string) (Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(Index), nil
		}
		return nil, err
	}
	var index Index
	return index, json.Unmarshal(data, &index)
}
