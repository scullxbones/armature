package ops

import "encoding/json"

// Op types — all 10 defined in architecture doc section 3, plus OpAssign for E3.
const (
	OpCreate            = "create"
	OpClaim             = "claim"
	OpHeartbeat         = "heartbeat"
	OpTransition        = "transition"
	OpNote              = "note"
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

	// Orchestration op types for E7 orchestrator.
	OpOrchestrateStart            = "orchestrate-start"
	OpOrchestrateDispatch         = "orchestrate-dispatch"
	OpOrchestrateDispatchComplete = "orchestrate-dispatch-complete"
	OpOrchestrateVerifyFail       = "orchestrate-verify-fail"
	OpOrchestrateRetry            = "orchestrate-retry"
	OpOrchestrateEscalate         = "orchestrate-escalate"
	OpOrchestrateComplete         = "orchestrate-complete"
	OpOrchestrateCheckResult      = "orchestrate-check-result"
	OpWorkerRuntimeDecision       = "worker-runtime-decision"
)

// ValidOpTypes for validation.
var ValidOpTypes = map[string]bool{
	OpCreate: true, OpClaim: true, OpHeartbeat: true,
	OpTransition: true, OpNote: true, OpLink: true, OpUnlink: true,
	OpSourceLink: true, OpSourceFingerprint: true,
	OpDAGTransition: true, OpDecision: true,
	OpAssign:           true,
	OpAmend:            true,
	OpCitationAccepted: true,
	OpScopeRename:      true,
	OpScopeDelete:      true,

	OpOrchestrateStart:            true,
	OpOrchestrateDispatch:         true,
	OpOrchestrateDispatchComplete: true,
	OpOrchestrateVerifyFail:       true,
	OpOrchestrateRetry:            true,
	OpOrchestrateEscalate:         true,
	OpOrchestrateComplete:         true,
	OpOrchestrateCheckResult:      true,
	OpWorkerRuntimeDecision:       true,
}

// FailureRecord captures a single orchestration failure event.
type FailureRecord struct {
	IssueID   string `json:"issue_id"`
	Reason    string `json:"reason"`
	Timestamp int64  `json:"timestamp"`
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

	// orchestrate-start, orchestrate-dispatch, orchestrate-retry payload fields
	WorktreePath   string `json:"worktree_path,omitempty"`
	PreDispatchRef string `json:"pre_dispatch_ref,omitempty"`
	RetryBudget    int    `json:"retry_budget,omitempty"`
	Run            int    `json:"run,omitempty"`

	// worker-runtime durable audit fields
	CorrelationID string `json:"correlation_id,omitempty"`
	CausationID   string `json:"causation_id,omitempty"`
	DecisionClass string `json:"decision_class,omitempty"`
}
