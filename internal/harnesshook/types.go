package harnesshook

import (
	"context"
)

// EventKind identifies the phase of a hook event (pre-tool-use, post-tool-use, stop).
type EventKind string

// Event phase constants recognised by the harness hook runner.
const (
	EventPreToolUse  EventKind = "pre-tool-use"
	EventPostToolUse EventKind = "post-tool-use"
	EventStop        EventKind = "stop"
)

// Event carries the normalised fields extracted from a platform hook payload.
type Event struct {
	Kind      EventKind
	Tool      string
	Paths     []string
	Command   string
	Cwd       string         // Current working directory from the hook event payload (if available)
	ToolInput map[string]any // Raw tool input for binding resolution and other post-decode logic
}

// DecisionAction is the verdict returned by an Evaluator (allow, block, or no opinion).
type DecisionAction string

// Decision action constants.
const (
	DecisionAllow DecisionAction = "allow"
	DecisionBlock DecisionAction = "block"
	DecisionNone  DecisionAction = "none"
)

// Decision is the evaluator's verdict together with an optional human-readable message.
type Decision struct {
	Action  DecisionAction
	Message string
}

// PlatformCapabilities describes which hook events and interception modes a platform supports.
type PlatformCapabilities struct {
	PreToolUse          bool
	Stop                bool
	PostToolUse         bool
	ShellInterception   string
	BlockingStop        bool
	SupportedEditTools  []string
	SupportedShellTools []string
}

// PlatformAdapter is the interface that each harness platform must implement to participate
// in the hook lifecycle.
type PlatformAdapter interface {
	Name() string
	Capabilities() PlatformCapabilities
	WriteConfig(workdir string) error
	OwnsConfig(workdir string) (bool, error)
	Decode(input []byte) (Event, error)
	// Encode returns (payload, exitCode, err). exitCode is non-zero only when the platform uses exit status to signal blocking.
	Encode(event Event, decision Decision) ([]byte, int, error)
}

// Evaluator applies policy to a hook event and returns an allow/block Decision.
type Evaluator interface {
	Evaluate(ctx context.Context, event Event) (Decision, error)
}
