package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ===== Shell Execution (from hooks/runner.go, worker/identity.go, config/context.go, doctor/doctor.go) =====

// RunCommand executes a shell command with args and returns combined output.
// Returns error if exit code is non-zero.
func RunCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %q: %s: %w", name, out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RunCommandOutput executes a shell command with args and returns stdout output.
// Returns error if exit code is non-zero.
func RunCommandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run %q: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RunShellScript executes a shell script (-c) with stdin input and returns stdout output.
// scriptCmd is the command to pass to sh -c.
// stdin provides input to the script's stdin.
// Returns error if the command fails or output cannot be parsed as JSON.
func RunShellScript(scriptCmd string, stdin []byte) ([]byte, error) {
	cmd := exec.Command("sh", "-c", scriptCmd)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("script execution failed: %w", err)
	}

	return stdout.Bytes(), nil
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

// GitConfigMode reads armature.mode from git config, defaulting to "single-branch" if unset.
func GitConfigMode(repoPath string) (string, error) {
	cmd := NonInteractiveGitCommand(repoPath, "config", "armature.mode")
	out, err := cmd.Output()
	if err != nil {
		// Exit code 1 means key not set — default to single-branch
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "single-branch", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// NonInteractiveGitCommand creates a git command with environment variables set
// to prevent blocking on credential prompts.
// fullArgs should be the complete argument list (including subcommand, options, and values).
func NonInteractiveGitCommand(repoPath string, args ...string) *exec.Cmd {
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_ASKPASS=true")
	return cmd
}

// GitLog runs git log with the given arguments and returns the output.
func GitLog(repoPath string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", repoPath, "log"}, args...)
	cmd := exec.Command("git", fullArgs...)
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

// ExecuteHook runs a single hook script command with the given input.
// Returns error if the script fails, output is invalid JSON, or the hook rejects.
func ExecuteHook(hookName string, hookCommand string, input HookInput) error {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal hook input: %w", err)
	}

	cmd := exec.Command("sh", "-c", hookCommand)
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
