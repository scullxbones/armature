package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ProcessStatus is the outcome of a RunProcess call.
type ProcessStatus int

const (
	// ProcessClean indicates the process exited with code 0.
	ProcessClean ProcessStatus = iota
	// ProcessError indicates the process exited with a non-zero code.
	ProcessError
	// ProcessTimeout indicates the process was killed due to a context deadline.
	ProcessTimeout
)

// RunProcess launches cmd with args in workdir, streams stdout/stderr to the
// supplied writers, and returns a ProcessStatus plus any error.
// Context cancellation causes the process to be killed.
func RunProcess(ctx context.Context, workdir string, cmdArgs []string, stdout, stderr io.Writer) (ProcessStatus, error) {
	return RunProcessWithEnv(ctx, workdir, cmdArgs, nil, stdout, stderr)
}

// RunProcessWithEnv launches cmd with args in workdir using optional env
// overrides, streams stdout/stderr to the supplied writers, and returns a
// ProcessStatus plus any error.
func RunProcessWithEnv(ctx context.Context, workdir string, cmdArgs []string, extraEnv map[string]string, stdout, stderr io.Writer) (ProcessStatus, error) {
	if len(cmdArgs) == 0 {
		return ProcessError, fmt.Errorf("RunProcess: cmdArgs must not be empty")
	}
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...) //nolint:gosec
	cmd.Dir = workdir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if len(extraEnv) > 0 {
		base := os.Environ()
		for k, v := range extraEnv {
			base = append(base, k+"="+v)
		}
		cmd.Env = base
	}

	if err := cmd.Run(); err != nil {
		// Distinguish context-caused failures from ordinary exit errors.
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return ProcessTimeout, ctxErr
		}
		return ProcessError, err
	}
	return ProcessClean, nil
}

// GitConfig reads a git config value from a repo.
// repoPath is the repo root; key is the config key (e.g. "armature.worker-id").
// Returns error if the key is not set or git fails.
func GitConfig(repoPath, key string) (string, error) {
	cmd := NonInteractiveGitCommand(repoPath, "config", "--local", key)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git config %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// NonInteractiveGitCommand creates a git command with environment variables set
// to prevent blocking on credential prompts.
// fullArgs should be the complete argument list (including subcommand, options, and values).
func NonInteractiveGitCommand(repoPath string, args ...string) *exec.Cmd {
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(context.Background(), "git", fullArgs...) //nolint:gosec // G204: "git" is constant; args are internal
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_ASKPASS=true")
	return cmd
}

// GitLog runs git log with the given arguments and returns the output.
func GitLog(repoPath string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", repoPath, "log"}, args...)
	cmd := exec.CommandContext(context.Background(), "git", fullArgs...) //nolint:gosec // G204: "git" is constant; args are internal
	out, err := cmd.Output()
	if err != nil {
		// Not a git repo or no commits — return empty string
		return "", nil
	}
	return string(out), nil
}

// ===== Hook Execution (from hooks/runner.go) =====

// HookInput is the JSON input passed to a hook script.
type HookInput struct {
	IssueID    string `json:"issue_id"`
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	WorkerID   string `json:"worker_id"`
}

// HookResult is the JSON output expected from a hook script.
type HookResult struct {
	Allowed bool   `json:"allowed"`
	Message string `json:"message"`
}

// ExecuteHook runs a single hook command with the given input.
// Returns error if the command fails, output is invalid JSON, or the hook rejects.
func ExecuteHook(hookName string, hookCommand []string, input HookInput) error {
	if len(hookCommand) == 0 {
		return nil
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal hook input: %w", err)
	}

	cmd := exec.CommandContext(context.Background(), hookCommand[0], hookCommand[1:]...) //nolint:gosec
	cmd.Stdin = bytes.NewReader(inputJSON)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hook %q failed: %s", hookName, err)
	}

	var result HookResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return fmt.Errorf("hook %q failed: invalid output: %s", hookName, err)
	}

	if !result.Allowed {
		return fmt.Errorf("hook %q rejected transition: %s", hookName, result.Message)
	}

	return nil
}
