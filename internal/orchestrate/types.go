package orchestrate

import "context"

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
}

// OrchestrateState captures the runtime state of an orchestration run.
type OrchestrateState struct {
	// RunID is a unique identifier for this orchestration run.
	RunID string
	// Phase is the current phase of the run (e.g. "build", "lint", "test").
	Phase string
	// Checks is the ordered list of check results accumulated so far.
	Checks []CheckResult
	// Failed is true if any error-severity check has failed.
	Failed bool
	// StartedAt is the Unix timestamp (seconds) when the run began.
	StartedAt int64
	// FinishedAt is the Unix timestamp (seconds) when the run ended (0 if still running).
	FinishedAt int64
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
