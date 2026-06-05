package harnesshook

import "context"

type EventKind string

const (
	EventPreToolUse  EventKind = "pre-tool-use"
	EventPostToolUse EventKind = "post-tool-use"
	EventStop        EventKind = "stop"
)

type Event struct {
	Kind    EventKind
	Tool    string
	Paths   []string
	Command string
}

type DecisionAction string

const (
	DecisionAllow DecisionAction = "allow"
	DecisionBlock DecisionAction = "block"
	DecisionNone  DecisionAction = "none"
)

type Decision struct {
	Action  DecisionAction
	Message string
}

type PlatformCapabilities struct {
	PreToolUse          bool
	Stop                bool
	PostToolUse         bool
	ShellInterception   string
	BlockingStop        bool
	SupportedEditTools  []string
	SupportedShellTools []string
}

type PlatformAdapter interface {
	Name() string
	Capabilities() PlatformCapabilities
	WriteConfig(workdir string) error
	Decode(input []byte) (Event, error)
	Encode(decision Decision) ([]byte, int, error)
}

type Evaluator interface {
	Evaluate(ctx context.Context, event Event) (Decision, error)
}
