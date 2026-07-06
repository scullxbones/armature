package harnesshook

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// ActivityEntry represents a single execution captured in the activity log.
type ActivityEntry struct {
	Command       string // The command that was executed
	ExitCode      int    // The exit status of the command (only meaningful when ExitCodeKnown is true)
	ExitCodeKnown bool   // Whether the harness reported an exit code for this command
	OutputHead    string // First 1KB of output (or full output if shorter)
	OutputTail    string // Last 1KB of output (or empty if total output is short)
	OutputHash    string // SHA256 hash of full output for integrity checking
	WorktreeHead  string // Worktree HEAD commit sha at execution time
	Timestamp     string // RFC3339 UTC timestamp
}

// TruncatedOutput represents the truncated output.
type TruncatedOutput struct {
	Head string // First 1KB
	Tail string // Last 1KB (empty if output was not truncated)
	Hash string // Full output hash
}

const (
	// maxOutputChunkSize is the maximum number of bytes to include in head or tail
	maxOutputChunkSize = 1024
	// maxCommandSize caps the recorded command text so a single activity log
	// line (and the O_APPEND write that produces it) stays bounded. Note this
	// does not guarantee atomic writes: a full line (command + head/tail output
	// + JSON overhead) can still exceed PIPE_BUF-based atomic-write guarantees
	// for regular files on POSIX systems. A malformed/interleaved line is
	// simply skipped by parseActivityLogFile, so the blast radius is low, but
	// this cap alone does not make concurrent appends safe.
	maxCommandSize = 4096
)

// truncateOutput truncates output to head+tail format and computes the full hash.
// Truncation points are adjusted backward (for head) or forward (for tail) to the
// nearest UTF-8 rune boundary so multi-byte characters are never split.
func truncateOutput(output []byte) TruncatedOutput {
	hash := fmt.Sprintf("%x", sha256.Sum256(output))

	if len(output) <= maxOutputChunkSize*2 {
		// Output is short enough to keep in full
		return TruncatedOutput{
			Head: string(output),
			Tail: "",
			Hash: hash,
		}
	}

	headEnd := runeBoundaryAtOrBefore(output, maxOutputChunkSize)
	tailStart := runeBoundaryAtOrAfter(output, len(output)-maxOutputChunkSize)

	return TruncatedOutput{
		Head: string(output[:headEnd]),
		Tail: string(output[tailStart:]),
		Hash: hash,
	}
}

// runeBoundaryAtOrBefore returns the largest index <= n that is not in the
// middle of a UTF-8 multi-byte sequence.
func runeBoundaryAtOrBefore(b []byte, n int) int {
	if n >= len(b) {
		return len(b)
	}
	for n > 0 && !utf8.RuneStart(b[n]) {
		n--
	}
	return n
}

// runeBoundaryAtOrAfter returns the smallest index >= n that is not in the
// middle of a UTF-8 multi-byte sequence.
func runeBoundaryAtOrAfter(b []byte, n int) int {
	if n <= 0 {
		return 0
	}
	for n < len(b) && !utf8.RuneStart(b[n]) {
		n++
	}
	return n
}

// truncateCommand caps command text at maxCommandSize bytes (on a rune boundary)
// so a single logged command can't blow past the append write size that a
// concurrent worktree hook process could interleave with (m5).
func truncateCommand(command string) string {
	if len(command) <= maxCommandSize {
		return command
	}
	end := runeBoundaryAtOrBefore([]byte(command), maxCommandSize)
	return command[:end]
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
		return fallbackGetHEAD(gitDir)
	}

	return headRef, nil
}

// fallbackGetHEAD gets the HEAD sha using `git --git-dir=<gitDir> rev-parse HEAD`
// as a last resort. This is needed when the ref file doesn't exist yet (e.g., on
// a freshly created branch). Using --git-dir directly (rather than deriving a
// working directory via path-string surgery) works uniformly for both a regular
// repo's git dir and a worktree's private git dir, and doesn't require guessing
// where the worktree's working tree lives.
func fallbackGetHEAD(gitDir string) (string, error) {
	//nolint:gosec // G204: git binary is constant, gitDir is internal, not user input
	cmd := exec.CommandContext(context.Background(), "git", "--git-dir="+gitDir, "rev-parse", "HEAD")
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

// shouldCaptureActivity checks if activity logging is enabled (not disabled by
// the repo-level kill-switch). Capture is on by default.
//
// There is intentionally no environment-variable override: an env var would be
// settable by the worker process mid-session (export …=1; run failing test;
// unset), letting the worker curate failure-then-success sequences out of the
// log — exactly the selection bias ADR-0008 rule 2 forbids. The only supported
// kill-switch is the repo-level git config, which travels with the repo/worktree
// rather than the shell.
func shouldCaptureActivity(gitDir string) bool {
	return !isActivityLoggingDisabledByRepoConfig(gitDir)
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
// It captures: command, exit status (or its absence), truncated output, full-output hash,
// worktree HEAD sha, timestamp. Respects the repo-level git config kill-switch
// armature.disable-activity-logging.
// Fails open on any capture error with stderr warning.
func AppendActivity(gitDir string, command string, exitCode int, exitCodeKnown bool, output []byte) error {
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
		Command:       truncateCommand(command),
		ExitCode:      exitCode,
		ExitCodeKnown: exitCodeKnown,
		OutputHead:    truncated.Head,
		OutputTail:    truncated.Tail,
		OutputHash:    truncated.Hash,
		WorktreeHead:  headSha,
		Timestamp:     time.Now().UTC().Format(time.RFC3339), //nolint:forbidigo // required for activity log timestamps
	}

	// Format the log line (JSONL: one JSON object per line)
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

// activityLogLine is the JSONL record written for each activity entry. Field
// names match what internal/review/fingerprint.go (and the
// armature-activity-indexer skill) expect when parsing the log.
type activityLogLine struct {
	Timestamp     string `json:"timestamp"`
	Command       string `json:"command"`
	ExitCode      int    `json:"exit_code"`
	ExitCodeKnown bool   `json:"exit_code_known"`
	HeadSHA       string `json:"head_sha"`
	OutputHash    string `json:"output_hash"`
	OutputHead    string `json:"output_head,omitempty"`
	OutputTail    string `json:"output_tail,omitempty"`
}

// formatActivityLogEntry formats an ActivityEntry as a single JSON line.
func formatActivityLogEntry(entry ActivityEntry) string {
	line := activityLogLine{
		Timestamp:     entry.Timestamp,
		Command:       entry.Command,
		ExitCode:      entry.ExitCode,
		ExitCodeKnown: entry.ExitCodeKnown,
		HeadSHA:       entry.WorktreeHead,
		OutputHash:    entry.OutputHash,
		OutputHead:    entry.OutputHead,
		OutputTail:    entry.OutputTail,
	}

	data, err := json.Marshal(line)
	if err != nil {
		// Should never happen for well-formed entries; fall back to a minimal
		// valid JSON line rather than corrupting the log.
		return fmt.Sprintf(`{"timestamp":%q,"command":"","exit_code":0,"exit_code_known":false,"head_sha":%q,"output_hash":""}`,
			entry.Timestamp, entry.WorktreeHead)
	}
	return string(data)
}
