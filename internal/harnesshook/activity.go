package harnesshook

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ActivityEntry represents a single execution captured in the activity log.
type ActivityEntry struct {
	Command      string // The command that was executed
	ExitCode     int    // The exit status of the command
	OutputHead   string // First 1KB of output (or full output if shorter)
	OutputTail   string // Last 1KB of output (or empty if total output is short)
	OutputHash   string // SHA256 hash of full output for integrity checking
	WorktreeHead string // Worktree HEAD commit sha at execution time
	Timestamp    string // RFC3339 UTC timestamp
}

// TruncatedOutput represents the truncated output with marker.
type TruncatedOutput struct {
	Head   string // First 1KB
	Tail   string // Last 1KB
	Hash   string // Full output hash
	Marker string // Marker indicating truncation
}

const (
	// maxOutputChunkSize is the maximum number of bytes to include in head or tail
	maxOutputChunkSize = 1024
	// truncationMarker is the string inserted between head and tail to indicate truncation
	truncationMarker = "\n... [output truncated, see hash below] ...\n"
)

// truncateOutput truncates output to head+tail format with a marker and full hash.
func truncateOutput(output []byte) TruncatedOutput {
	hash := fmt.Sprintf("%x", sha256.Sum256(output))

	if len(output) <= maxOutputChunkSize*2 {
		// Output is short enough to keep in full
		return TruncatedOutput{
			Head:   string(output),
			Tail:   "",
			Hash:   hash,
			Marker: "",
		}
	}

	// Truncate to head+tail format
	head := string(output[:maxOutputChunkSize])
	tail := string(output[len(output)-maxOutputChunkSize:])
	return TruncatedOutput{
		Head:   head,
		Tail:   tail,
		Hash:   hash,
		Marker: truncationMarker,
	}
}

// getWorktreeHEAD retrieves the current HEAD sha of the worktree.
// It first tries to read the HEAD file directly, and if that fails or the ref doesn't exist yet,
// it falls back to extracting the git dir parent and using git rev-parse (which doesn't require the ref file to exist).
func getWorktreeHEAD(gitDir string) (string, error) {
	headFile := filepath.Join(gitDir, "HEAD")
	content, err := os.ReadFile(headFile) //nolint:gosec // G304: derived from trusted git directory
	if err != nil {
		return "", fmt.Errorf("read HEAD: %w", err)
	}

	headRef := strings.TrimSpace(string(content))

	// If HEAD is a detached state (starts with hash), return it directly
	if len(headRef) == 40 || len(headRef) == 64 {
		return headRef, nil
	}

	// Otherwise it's a ref like "ref: refs/heads/main", read the actual ref file
	if strings.HasPrefix(headRef, "ref: ") {
		refPath := strings.TrimPrefix(headRef, "ref: ")
		// Ref paths are relative to the git directory
		refFilePath := filepath.Join(gitDir, refPath)
		refContent, err := os.ReadFile(refFilePath) //nolint:gosec // G304: derived from ref in HEAD file
		if err == nil {
			return strings.TrimSpace(string(refContent)), nil
		}
		// If the ref file doesn't exist, fall back to git rev-parse (which doesn't require the ref file)
		// Extract the worktree root from the git dir
		// For a regular repo: gitDir is /repo/.git
		// For a worktree: gitDir is /repo/.git/worktrees/name, and the parent repo root is /repo
		return fallbackGetHEAD(gitDir)
	}

	return headRef, nil
}

// fallbackGetHEAD tries to get the HEAD sha using a git command as a last resort.
// This is needed when the ref file doesn't exist yet (e.g., on a freshly created branch).
func fallbackGetHEAD(gitDir string) (string, error) {
	// Try to find the working directory. For a worktree, we need to go up to find the repo root.
	// gitDir can be either:
	// - /repo/.git (regular repo)
	// - /repo/.git/worktrees/name (worktree)
	// We want to find /repo to pass to git -C
	var workDir string
	if strings.Contains(gitDir, ".git/worktrees/") {
		// It's a worktree: extract the parent repo root
		parts := strings.Split(gitDir, ".git/worktrees/")
		if len(parts) >= 1 {
			workDir = strings.TrimSuffix(parts[0], "/")
		}
	} else if strings.HasSuffix(gitDir, ".git") {
		// Regular repo
		workDir = strings.TrimSuffix(gitDir, ".git")
	}

	if workDir == "" {
		return "", fmt.Errorf("unable to determine work directory from git dir: %s", gitDir)
	}

	// Use git rev-parse to get the HEAD sha, which works regardless of whether the ref file exists
	//nolint:gosec // G204: git binary and workDir are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", workDir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// activityLoggingConfigKey is the repo-level git config key that disables activity
// capture. It follows the same repo-local git config pattern used elsewhere in
// Armature (e.g. armature.worker-id, armature.ops-worktree-path): a boolean value
// of "true" disables capture, anything else (including unset) leaves it enabled.
const activityLoggingConfigKey = "armature.disable-activity-logging"

// shouldCaptureActivity checks if activity logging is enabled (not disabled by kill-switch).
// Capture is on by default. It can be disabled two ways, either of which is sufficient:
//   - Repo-level config: `git config --local armature.disable-activity-logging true`
//     (the Definition-of-Done kill-switch; travels with the repo/worktree, not the shell).
//   - Environment variable override: ARMATURE_DISABLE_ACTIVITY_LOGGING (handy for one-off
//     shell sessions without touching repo config).
func shouldCaptureActivity(gitDir string) bool {
	// Environment variable override (checked first; either mechanism disabling is sufficient).
	disableEnv := os.Getenv("ARMATURE_DISABLE_ACTIVITY_LOGGING")
	if disableEnv != "" && disableEnv != "0" && disableEnv != "false" {
		return false
	}

	// Repo-level config kill-switch.
	if isActivityLoggingDisabledByRepoConfig(gitDir) {
		return false
	}

	return true
}

// isActivityLoggingDisabledByRepoConfig reads the repo-level git config kill-switch
// directly via --git-dir (rather than requiring a working-tree path), since callers
// only have the resolved git dir (which may be a worktree's private git dir, e.g.
// <repo>/.git/worktrees/<name>) available at this point. All worktrees of a repo
// share the same --local config store, so this reads the same value regardless of
// which worktree's hook invocation triggered it.
// Returns false (capture enabled) if the key is unset or git fails for any reason,
// so a missing/misconfigured git binary fails open rather than silently disabling
// capture.
func isActivityLoggingDisabledByRepoConfig(gitDir string) bool {
	if gitDir == "" {
		return false
	}
	//nolint:gosec // G204: git binary is constant, gitDir/key are internal, not user input
	cmd := exec.CommandContext(context.Background(), "git", "--git-dir="+gitDir, "config", "--local", "--bool", activityLoggingConfigKey)
	out, err := cmd.Output()
	if err != nil {
		// Unset key (or any other git config error) leaves capture enabled.
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// AppendActivity appends an execution event to the worktree-local armature-activity.log.
// It captures: command, exit status, truncated output, full-output hash, worktree HEAD sha, timestamp.
// Respects a kill-switch, either the repo-level git config
// armature.disable-activity-logging or the ARMATURE_DISABLE_ACTIVITY_LOGGING
// environment variable.
// Fails open on any capture error with stderr warning.
func AppendActivity(gitDir string, command string, exitCode int, output []byte) error {
	// Check if activity capture is disabled
	if !shouldCaptureActivity(gitDir) {
		return nil
	}

	// Get the worktree HEAD sha
	headSha, err := getWorktreeHEAD(gitDir)
	if err != nil {
		// Fail open: log warning to stderr and return nil
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to get worktree HEAD for activity logging: %v\n", err)
		return nil
	}

	// Truncate output and compute hash
	truncated := truncateOutput(output)

	// Create activity entry
	entry := ActivityEntry{
		Command:      command,
		ExitCode:     exitCode,
		OutputHead:   truncated.Head,
		OutputTail:   truncated.Tail,
		OutputHash:   truncated.Hash,
		WorktreeHead: headSha,
		Timestamp:    time.Now().UTC().Format(time.RFC3339), //nolint:forbidigo // required for activity log timestamps
	}

	// Format the log line
	logLine := formatActivityLogEntry(entry)

	// Append to armature-activity.log
	logPath := filepath.Join(gitDir, "armature-activity.log")
	//nolint:gosec // G304: logPath is derived from trusted git directory
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		// Fail open: log warning to stderr and return nil
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to open activity log for writing: %v\n", err)
		return nil
	}
	defer func() {
		_ = f.Close() //nolint:errcheck // closing log file, error is not actionable
	}()

	_, err = fmt.Fprintf(f, "%s\n", logLine)
	if err != nil {
		// Fail open: log warning to stderr and return nil
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to write activity log entry: %v\n", err)
		return nil
	}

	return nil
}

// formatActivityLogEntry formats an ActivityEntry for the log file.
func formatActivityLogEntry(entry ActivityEntry) string {
	// Build the base entry with essential fields
	logEntry := fmt.Sprintf("%s activity: command=%q exit_code=%d head_sha=%s output_hash=%s",
		entry.Timestamp,
		entry.Command,
		entry.ExitCode,
		entry.WorktreeHead,
		entry.OutputHash,
	)

	// Add truncated output if available
	if entry.OutputHead != "" {
		// Escape newlines in output for log formatting
		escapedHead := strings.ReplaceAll(entry.OutputHead, "\n", "\\n")
		if entry.OutputTail != "" {
			escapedTail := strings.ReplaceAll(entry.OutputTail, "\n", "\\n")
			logEntry += fmt.Sprintf(" output_truncated=%q...%q", escapedHead, escapedTail)
		} else {
			logEntry += fmt.Sprintf(" output=%q", escapedHead)
		}
	}

	return logEntry
}
