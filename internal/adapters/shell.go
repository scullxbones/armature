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

// ===== Shell Execution (from hooks/runner.go, worker/identity.go, config/context.go, doctor/doctor.go) =====

// LookPath reports whether a binary is available in PATH.
// It wraps exec.LookPath so callers do not need to import os/exec directly.
func LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// RunCommand executes a shell command with args and returns combined output.
// Returns error if exit code is non-zero.
func RunCommand(name string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), name, args...) //nolint:gosec // G204: callers are internal; name is never raw user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %q: %s: %w", name, out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RunCommandOutput executes a shell command with args and returns stdout output.
// Returns error if exit code is non-zero.
func RunCommandOutput(name string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), name, args...) //nolint:gosec // G204: callers are internal; name is never raw user input
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
	cmd := exec.CommandContext(context.Background(), "sh", "-c", scriptCmd) //nolint:gosec // G204: script is caller-controlled hook command, not user input
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

// GitWorktreeBranches returns the set of branch names that currently have a live
// worktree registered against repoPath (via `git worktree list --porcelain`).
// Used to detect claims whose task worktree was torn down (or never existed)
// out from under an active claim.
//
// If repoPath is empty, this is treated as a deliberate "no repo to check"
// request: it returns an empty set with a nil error. Any other failure (git
// not found, `-C repoPath` not a git repo, a transient git error) is
// propagated as a non-nil error — callers MUST NOT treat an error as "no live
// worktrees exist"; a git failure means liveness could not be determined, and
// a caller that used that empty map to conclude "no worktree" for every
// branch would misfire the same way for a transient failure as for a real
// missing worktree.
func GitWorktreeBranches(repoPath string) (map[string]bool, error) {
	if repoPath == "" {
		return map[string]bool{}, nil
	}
	//nolint:gosec // G204: "git" is constant; args are internal
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list --porcelain in %s: %w", repoPath, err)
	}
	return parseWorktreePorcelain(string(out)), nil
}

// parseWorktreePorcelain parses `git worktree list --porcelain` output into
// the set of branch names with a live (non-prunable) worktree.
//
// Porcelain output is a sequence of blocks separated by blank lines, one per
// worktree, e.g.:
//
//	worktree /path/to/worktree
//	branch refs/heads/task/foo
//
// A worktree whose directory was deleted without `git worktree remove`/
// `prune` gets an additional `prunable ...` line in its block:
//
//	worktree /path/to/deleted/worktree
//	branch refs/heads/task/foo
//	prunable gitdir file points to non-existent location
//
// Branches belonging to a block containing a `prunable` line are excluded —
// their worktree is stale and should not be treated as live.
func parseWorktreePorcelain(out string) map[string]bool {
	const branchPrefix = "branch refs/heads/"
	const prunablePrefix = "prunable "

	branches := make(map[string]bool)
	var blockBranch string
	var blockPrunable bool

	flush := func() {
		if blockBranch != "" && !blockPrunable {
			branches[blockBranch] = true
		}
		blockBranch = ""
		blockPrunable = false
	}

	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			flush()
			continue
		}
		if after, ok := strings.CutPrefix(line, branchPrefix); ok {
			blockBranch = after
			continue
		}
		if strings.HasPrefix(line, prunablePrefix) {
			blockPrunable = true
		}
	}
	flush()

	return branches
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
