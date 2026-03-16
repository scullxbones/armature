package ops

import "encoding/json"

// Op types — all 10 defined in architecture doc section 3.
const (
	OpCreate            = "create"
	OpClaim             = "claim"
	OpHeartbeat         = "heartbeat"
	OpTransition        = "transition"
	OpNote              = "note"
	OpLink              = "link"
	OpSourceLink        = "source-link"
	OpSourceFingerprint = "source-fingerprint"
	OpDAGTransition     = "dag-transition"
	OpDecision          = "decision"
)

// ValidOpTypes for validation.
var ValidOpTypes = map[string]bool{
	OpCreate: true, OpClaim: true, OpHeartbeat: true,
	OpTransition: true, OpNote: true, OpLink: true,
	OpSourceLink: true, OpSourceFingerprint: true,
	OpDAGTransition: true, OpDecision: true,
}

// Issue statuses.
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
	Title            string          `json:"title,omitempty"`
	Parent           string          `json:"parent,omitempty"`
	NodeType         string          `json:"type,omitempty"`
	Scope            []string        `json:"scope,omitempty"`
	Acceptance       json.RawMessage `json:"acceptance,omitempty"`
	DefinitionOfDone string          `json:"definition_of_done,omitempty"`
	Context          json.RawMessage `json:"context,omitempty"`
	SourceCitation   json.RawMessage `json:"source_citation,omitempty"`
	Priority         string          `json:"priority,omitempty"`
	EstComplexity    string          `json:"estimated_complexity,omitempty"`

	// claim
	TTL int `json:"ttl,omitempty"`

	// transition
	To      string `json:"to,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Branch  string `json:"branch,omitempty"`
	PR      string `json:"pr,omitempty"`

	// note
	Msg string `json:"msg,omitempty"`

	// link
	Dep string `json:"dep,omitempty"`
	Rel string `json:"rel,omitempty"`

	// source-link
	SourceID string `json:"source_id,omitempty"`
	Section  string `json:"section,omitempty"`
	Anchor   string `json:"anchor,omitempty"`
	Quote    string `json:"quote,omitempty"`

	// source-fingerprint
	SHA       string `json:"sha,omitempty"`
	VersionID string `json:"version_id,omitempty"`
	Provider  string `json:"provider,omitempty"`

	// dag-transition
	UncoveredAcknowledged []string `json:"uncovered_acknowledged,omitempty"`

	// decision
	Topic     string   `json:"topic,omitempty"`
	Choice    string   `json:"choice,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
	Affects   []string `json:"affects,omitempty"`
}
