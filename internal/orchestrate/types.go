package orchestrate

import (
	"context"
	"time"
)

// ExitStatus represents the outcome of a harness invocation.
type ExitStatus int

const (
	// ExitSuccess indicates the command completed without errors.
	ExitSuccess ExitStatus = iota
	// ExitFailure indicates the command exited with a non-zero status.
	ExitFailure
	// ExitTimeout indicates the command exceeded its deadline.
	ExitTimeout
	// ExitCancelled indicates the command was cancelled via context.
	ExitCancelled
)

// String returns a human-readable representation of the ExitStatus.
func (e ExitStatus) String() string {
	switch e {
	case ExitSuccess:
		return "success"
	case ExitFailure:
		return "failure"
	case ExitTimeout:
		return "timeout"
	case ExitCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// CheckSeverity classifies the importance of a check result.
type CheckSeverity int

const (
	// SeverityInfo is informational only; does not block progression.
	SeverityInfo CheckSeverity = iota
	// SeverityWarning flags a potential problem but does not fail the run.
	SeverityWarning
	// SeverityError flags a problem that causes the run to fail.
	SeverityError
)

// String returns a human-readable representation of the CheckSeverity.
func (s CheckSeverity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// InvocationResult holds the outcome of a single harness command execution.
type InvocationResult struct {
	// Status is the high-level exit status of the invocation.
	Status ExitStatus
	// ExitCode is the raw process exit code (0 on success).
	ExitCode int
	// Stdout contains the standard output of the command.
	Stdout string
	// Stderr contains the standard error output of the command.
	Stderr string
	// DurationMs is how long the command ran in milliseconds.
	DurationMs int64
}

// HarnessConfig holds configuration for a single harness adapter instance.
type HarnessConfig struct {
	// Adapter selects the harness implementation: "claude", "codex", or "devin".
	Adapter string `json:"adapter,omitempty"`
	// Model is the LLM model name to pass to the harness CLI (e.g. "claude-haiku-4-5").
	Model string `json:"model,omitempty"`
	// Timeout is the per-invocation timeout in seconds (0 = no limit).
	Timeout int `json:"timeout,omitempty"`
	// BuildCmd is the shell command used to build the project.
	BuildCmd string `json:"build_cmd,omitempty"`
	// LintCmd is the shell command used to lint the project.
	LintCmd string `json:"lint_cmd,omitempty"`
	// TestCmd is the shell command used to run tests.
	TestCmd string `json:"test_cmd,omitempty"`
	// CoverageCmd is the shell command used to measure coverage.
	CoverageCmd string `json:"coverage_cmd,omitempty"`
	// MutateCmd is the shell command used for mutation testing.
	MutateCmd string `json:"mutate_cmd,omitempty"`
	// WorkDir is the working directory in which commands are run.
	WorkDir string `json:"work_dir,omitempty"`
	// TimeoutSeconds is the per-command timeout in seconds (0 = no limit).
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// Env are optional process-level environment overrides for harness execution.
	Env map[string]string `json:"-"`
	// AuthSource reports which auth path was selected (api-key or oauth-session).
	AuthSource string `json:"auth_source,omitempty"`
}

// CheckResult holds the outcome of a single verification check.
type CheckResult struct {
	// Name identifies the check (e.g. "build", "lint", "test").
	Name string
	// Severity classifies how important a failure of this check is.
	Severity CheckSeverity
	// Passed is true when the check produced no errors.
	Passed bool
	// Message is a human-readable summary of the check result.
	Message string
	// Invocation is the raw result from running the check command.
	Invocation InvocationResult
}

// RunOptions controls a single orchestration run.
type RunOptions struct {
	// DryRun skips actual command execution when true.
	DryRun bool
	// MaxParallel limits the number of concurrently running agents (0 = unlimited).
	MaxParallel int
	// WorkDir overrides the default working directory for this run.
	WorkDir string
	// Env holds additional environment variables to inject into commands.
	Env map[string]string
	// Progress receives phase/heartbeat updates during orchestration.
	Progress func(ProgressEvent)
	// HeartbeatInterval controls heartbeat cadence while the harness runs.
	// Zero means 15s.
	HeartbeatInterval time.Duration
}

// ProgressEvent is a user-facing progress update emitted during orchestration.
type ProgressEvent struct {
	Kind      string
	Phase     string
	Message   string
	Elapsed   time.Duration
	Harness   string
	Retry     int
	Timestamp time.Time
}

// OrchestrateState captures the runtime state of an orchestration run.
// It is used both for live orchestration and for crash-resume state derivation.
type OrchestrateState struct {
	// RunID is a unique identifier for this orchestration run.
	RunID string
	// Phase is the current phase of the run (e.g. "pending", "dispatched", "running", "verify-failed", "retrying", "escalated", "complete").
	Phase string
	// Checks is the ordered list of check results accumulated so far.
	Checks []CheckResult
	// Failed is true if any error-severity check has failed.
	Failed bool
	// CompletionMessage explains complete-phase outcomes that need operator attention.
	CompletionMessage string
	// StartedAt is the Unix timestamp (seconds) when the run began.
	StartedAt int64
	// FinishedAt is the Unix timestamp (seconds) when the run ended (0 if still running).
	FinishedAt int64

	// Crash-resume fields derived from replaying the op log.

	// Run is the 1-based count of how many dispatch cycles have been attempted.
	Run int
	// PreDispatchRef is the git commit ref recorded just before the last dispatch.
	PreDispatchRef string
	// WorktreePath is the path to the agent worktree for this task.
	WorktreePath string
	// RetryBudget is the number of retry attempts remaining.
	RetryBudget int
	// TransitionWritten is true if an OpTransition targeting this task has been durably recorded.
	// Used to detect the case where OpOrchestrateComplete was written but the subsequent
	// lifecycle transition op was lost (e.g. network error), so it can be re-attempted.
	TransitionWritten bool
}

// HarnessAdapter is the interface that every verification adapter must satisfy.
// Implementations are responsible for running a verification phase (build, lint,
// test, etc.) and returning a structured result.
type HarnessAdapter interface {
	// Name returns the human-readable name of this adapter (e.g. "go-build").
	Name() string

	// Run executes the verification phase described by this adapter and returns
	// a CheckResult. Implementations must honour ctx cancellation/deadline.
	Run(ctx context.Context, cfg HarnessConfig, opts RunOptions) (CheckResult, error)
}
