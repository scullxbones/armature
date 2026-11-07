package harnesshook

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
	Kind          EventKind
	Tool          string
	Paths         []string
	Command       string
	Cwd           string         // Current working directory from the hook event payload (if available)
	ToolInput     map[string]any // Raw tool input for binding resolution and other post-decode logic
	ExitCode      int            // Exit code from PostToolUse events (only meaningful when ExitCodeKnown is true)
	ExitCodeKnown bool           // Whether the harness actually reported an exit code (false for PreToolUse events or harnesses that omit it)
	Output        []byte         // Output from PostToolUse events (empty if not available)
}

// decisionAction is the verdict returned by an evaluator (allow, block, or no opinion).
// Callers read Decision.Action as a string; the constants stay inside this package.
type decisionAction string

const (
	decisionAllow decisionAction = "allow"
	decisionBlock decisionAction = "block"
	decisionNone  decisionAction = "none"
)

// Decision is the evaluator's verdict together with an optional human-readable message.
type Decision struct {
	Action  decisionAction
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

// platformAdapter is the per-platform hook lifecycle. Callers select one with
// NewAdapterForPlatform and use its methods; the interface is not part of the
// package surface.
type platformAdapter interface {
	Name() string
	Capabilities() PlatformCapabilities
	WriteConfig(workdir string) error
	OwnsConfig(workdir string) (bool, error)
	Decode(input []byte) (Event, error)
	// Encode returns (payload, exitCode, err). exitCode is non-zero only when the platform uses exit status to signal blocking.
	Encode(event Event, decision Decision) ([]byte, int, error)
}
