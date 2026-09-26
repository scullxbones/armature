package harnesshook

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/scullxbones/armature/internal/adapters"
)

// ActivityEntry represents a single execution captured in the activity log.
type ActivityEntry struct {
	Command       string
	ExitCode      int
	ExitCodeKnown bool
	OutputHead    string
	OutputTail    string
	OutputHash    string
	WorktreeHead  string
	Timestamp     string
}

type TruncatedOutput struct {
	Head string
	Tail string
	Hash string
}

const (
	maxOutputChunkSize = 1024
	maxCommandSize     = 4096
)

func truncateOutput(output []byte) TruncatedOutput {
	hash := fmt.Sprintf("%x", sha256.Sum256(output))

	if len(output) <= maxOutputChunkSize*2 {
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

func runeBoundaryAtOrBefore(b []byte, n int) int {
	if n >= len(b) {
		return len(b)
	}
	for n > 0 && !utf8.RuneStart(b[n]) {
		n--
	}
	return n
}

func runeBoundaryAtOrAfter(b []byte, n int) int {
	if n <= 0 {
		return 0
	}
	for n < len(b) && !utf8.RuneStart(b[n]) {
		n++
	}
	return n
}

func truncateCommand(command string) string {
	if len(command) <= maxCommandSize {
		return command
	}
	end := runeBoundaryAtOrBefore([]byte(command), maxCommandSize)
	return command[:end]
}

func getWorktreeHEAD(gitDir string) (string, error) {
	headFile := filepath.Join(gitDir, "HEAD")
	content, err := adapters.ReadFile(headFile)
	if err != nil {
		return "", fmt.Errorf("read HEAD: %w", err)
	}

	headRef := strings.TrimSpace(string(content))
	if headRefLooksLikeSHA(headRef) {
		return headRef, nil
	}
	return fallbackGetHEAD(gitDir)
}

func headRefLooksLikeSHA(headRef string) bool {
	n := len(headRef)
	return n == 40 || n == 64
}

func fallbackGetHEAD(gitDir string) (string, error) {
	cmd := adapters.NonInteractiveGitCommand(gitDir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

const activityLoggingConfigKey = "armature.disable-activity-logging"

func shouldCaptureActivity(gitDir string) bool {
	return !isActivityLoggingDisabledByRepoConfig(gitDir)
}

func isActivityLoggingDisabledByRepoConfig(gitDir string) bool {
	if gitDir == "" {
		return false
	}
	cmd := adapters.NonInteractiveGitCommand(gitDir, "config", "--local", "--bool", activityLoggingConfigKey)
	out, err := cmd.Output()
	if err != nil {
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
	if strings.TrimSpace(command) == "" {
		return nil
	}
	if !shouldCaptureActivity(gitDir) {
		return nil
	}

	headSha, err := getWorktreeHEAD(gitDir)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to get worktree HEAD for activity logging: %v\n", err)
		return nil
	}

	truncated := truncateOutput(output)

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

	logLine := formatActivityLogEntry(entry)
	if logLine == "" {
		return nil
	}

	logPath := filepath.Join(gitDir, "armature-activity.log")
	//nolint:gosec // G304: logPath is derived from trusted git directory
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to open activity log for writing: %v\n", err)
		return nil
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to close activity log: %v\n", closeErr)
		}
	}()

	_, err = fmt.Fprintf(f, "%s\n", logLine)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to write activity log entry: %v\n", err)
		return nil
	}

	return nil
}

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

func formatActivityLogEntry(entry ActivityEntry) string {
	if strings.TrimSpace(entry.Command) == "" {
		return ""
	}
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
		return fallbackActivityJSONL(entry)
	}
	return string(data)
}

func fallbackActivityJSONL(entry ActivityEntry) string {
	if strings.TrimSpace(entry.Command) == "" {
		return ""
	}
	return fmt.Sprintf(
		`{"timestamp":%q,"command":%q,"exit_code":%d,"exit_code_known":%t,"head_sha":%q,"output_hash":%q}`,
		entry.Timestamp, entry.Command, entry.ExitCode, entry.ExitCodeKnown, entry.WorktreeHead, entry.OutputHash,
	)
}
