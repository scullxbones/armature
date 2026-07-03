package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/spf13/cobra"
)

// resolveIssueBinding reads the issue ID from <git-dir>/armature-issue-id,
// falls back to ARMATURE_ISSUE_ID environment variable, and returns an empty
// string if neither is present.
func resolveIssueBinding(gitDir string) string {
	issueIDPath := filepath.Join(gitDir, "armature-issue-id")
	// #nosec G304 - issueIDPath is derived from a trusted git directory
	if data, err := os.ReadFile(issueIDPath); err == nil {
		return strings.TrimSpace(string(data))
	}
	return os.Getenv("ARMATURE_ISSUE_ID")
}

// logPassThrough logs a pass-through event to <git-dir>/armature-hook.log.
// Each entry is prefixed with an RFC3339 UTC timestamp so operators can
// correlate entries with specific git operations.
func logPassThrough(gitDir string, reason string) error {
	logPath := filepath.Join(gitDir, "armature-hook.log")
	// #nosec G304 - logPath is derived from a trusted git directory
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close() //nolint:errcheck // closing log file, error is not actionable
	}()
	ts := time.Now().UTC().Format(time.RFC3339)
	_, err = fmt.Fprintf(f, "%s pass-through: %s\n", ts, reason)
	return err
}

// logDecision logs a complete decision to <git-dir>/armature-hook.log.
// Each entry includes: timestamp, issue ID, resolution step, event kind, tool, decision, and optional block reason.
func logDecision(gitDir string, issueID string, eventKind string, tool string, decision string, blockReason string) error {
	logPath := filepath.Join(gitDir, "armature-hook.log")
	// #nosec G304 - logPath is derived from a trusted git directory
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close() //nolint:errcheck // closing log file, error is not actionable
	}()
	ts := time.Now().UTC().Format(time.RFC3339)
	// Format: timestamp issue_id resolution_step event tool decision [block_reason]
	entry := fmt.Sprintf("%s decision: issue_id=%s resolution_step=evaluated event=%s tool=%s decision=%s", ts, issueID, eventKind, tool, decision)
	if blockReason != "" {
		entry += fmt.Sprintf(" block_reason=%s", blockReason)
	}
	_, err = fmt.Fprintf(f, "%s\n", entry)
	return err
}

// logViolation logs a violation entry for file writes that resolve to no binding.
// Violations are distinguished from pass-throughs: they indicate an enforcement gap
// (unbound file write when enforcement was expected).
func logViolation(gitDir string, reason string) error {
	logPath := filepath.Join(gitDir, "armature-hook.log")
	// #nosec G304 - logPath is derived from a trusted git directory
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close() //nolint:errcheck // closing log file, error is not actionable
	}()
	ts := time.Now().UTC().Format(time.RFC3339)
	_, err = fmt.Fprintf(f, "%s violation: %s\n", ts, reason)
	return err
}

// isBindingStale checks if the issue binding's status is not claimed or in-progress,
// or if the claim's TTL has expired.
func isBindingStale(snap *snapshot.Snapshot, taskID string, now int64) bool {
	issue, ok := snap.Issues[taskID]
	if !ok {
		return true // Missing issue = stale
	}
	// If status is not claimed/in-progress, it's stale
	if issue.Status != ops.StatusClaimed && issue.Status != ops.StatusInProgress {
		return true
	}
	// Check if the claim's TTL has expired
	return claimPkg.IsClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, now)
}

// extractFilePathFromToolInput extracts the file path from the raw tool_input map.
// It checks for common file path keys in the order they're likely to be used.
func extractFilePathFromToolInput(toolInput map[string]any) string {
	if toolInput == nil {
		return ""
	}

	// Check for direct file_path or path keys
	for _, key := range []string{"file_path", "path"} {
		if value, ok := toolInput[key].(string); ok && value != "" {
			return value
		}
	}

	// Check for changes array (common in Edit/Write events)
	if changes, ok := toolInput["changes"].([]any); ok && len(changes) > 0 {
		if change, ok := changes[0].(map[string]any); ok {
			if path, ok := change["path"].(string); ok && path != "" {
				return path
			}
		}
	}

	return ""
}

// isFileWriteEvent checks if an event represents a file write operation.
// This is used to distinguish violations (unbound file writes) from pass-throughs.
func isFileWriteEvent(eventKind harnesshook.EventKind) bool {
	// Only PreToolUse events can be file writes; Stop and Bash events are not file operations
	return eventKind == harnesshook.EventPreToolUse || eventKind == harnesshook.EventPostToolUse
}

// applyRunResult writes the output to the provided writer and returns an adapterExitError
// if the result's ExitCode is non-zero.
func applyRunResult(out io.Writer, result harnesshook.RunResult) error {
	_, _ = out.Write(result.Output) //nolint:errcheck // stdout write not actionable in CLI
	if result.ExitCode != 0 {
		return adapterExitError{code: result.ExitCode}
	}
	return nil
}

func newHarnessHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "harness-hook",
		Short:         "Internal harness hook entrypoint",
		Hidden:        true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			appCtx := currentCtx(cmd)
			// Resolve the worktree's own git dir (e.g., <parent>/.git/worktrees/<name>),
			// not the parent repo's .git. This ensures we read the binding file that
			// claim --worktree wrote into the worktree-specific git directory.
			//
			// appCtx.RepoPath is already resolved to the parent repo root when invoked
			// from a worktree, so we read the raw --repo flag to get the path the user
			// actually passed (which may be the worktree directory itself).
			rawRepo, _ := cmd.Root().PersistentFlags().GetString("repo")
			if rawRepo == "" {
				rawRepo = "."
			}
			gitDir, err := resolveWorktreeGitDir(rawRepo)
			if err != nil {
				// Fall back to the conventional path if resolution fails (e.g., bare repo or
				// unusual layout); the binding file may not exist but we degrade gracefully.
				gitDir = filepath.Join(appCtx.RepoPath, ".git")
			}
			// Resolve session-level binding (from git dir or env) for use as fallback
			sessionBinding := resolveIssueBinding(gitDir)

			// Read hook input from stdin (before binding resolution per ADR-0007)
			inputData, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read hook input: %w", err)
			}

			// Decode the event to determine if path-based binding resolution is needed
			adapter, err := harnesshook.NewAdapterForPlatform(os.Getenv("ARMATURE_HOOK_PLATFORM"))
			if err != nil {
				// If we can't select an adapter, fail open and pass through
				_ = logPassThrough(gitDir, "adapter selection failed") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			event, err := adapter.Decode(inputData)
			if err != nil {
				// If we can't decode the event, fail open and pass through with loud stderr warning
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to decode hook event: %v\n", err)
				_ = logPassThrough(gitDir, "event decode failed") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Extract file path from tool input for path-based resolution
			filePath := extractFilePathFromToolInput(event.ToolInput)
			eventInfo := &harnesshook.DecodedEventInfo{
				Kind:     event.Kind,
				FilePath: filePath,
			}

			// Resolve binding from event and session binding
			finalBinding, err := harnesshook.ResolveBindingFromEvent(eventInfo, sessionBinding)
			if err != nil {
				// If binding resolution fails, fail open and pass through
				_ = logPassThrough(gitDir, "binding resolution failed") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// If no binding is found:
			// - File writes are violations (enforcement gap)
			// - Other events are pass-throughs (no enforcement expected)
			if finalBinding == "" {
				if isFileWriteEvent(event.Kind) {
					_ = logViolation(gitDir, "file write with no resolved binding") //nolint:errcheck // logging only, error not actionable
				} else {
					_ = logPassThrough(gitDir, "no issue binding found") //nolint:errcheck // logging only, error not actionable
				}
				return nil
			}

			// Load snapshot to check if binding is stale
			snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir)
			if err != nil {
				// Snapshot load errors are fail-open with loud stderr warning
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to load snapshot: %v\n", err)
				_ = logPassThrough(gitDir, "snapshot load failed") //nolint:errcheck // logging only, error not actionable
				return nil
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			// If binding is stale, pass through
			if isBindingStale(snap, finalBinding, time.Now().Unix()) {
				_ = logPassThrough(gitDir, "stale issue binding") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Create policy resolver
			resolver := harnesspolicy.NewIssuePolicyResolver(harnesspolicy.ResolverConfig{
				RepoPath:   appCtx.RepoPath,
				StateDir:   appCtx.StateDir,
				SourcesDir: filepath.Join(appCtx.IssuesDir, "sources"),
			})

			// Create hook and evaluate with resolved binding
			hook := harnesshook.NewHook(resolver)
			result, err := hook.Evaluate(cmd.Context(), harnesshook.EvaluateInput{
				Input:          inputData,
				TaskID:         finalBinding,
				Platform:       os.Getenv("ARMATURE_HOOK_PLATFORM"),
				SessionBinding: sessionBinding,
			})
			if err != nil {
				return err
			}

			// Log the decision with complete information
			blockReason := result.Decision.Message
			_ = logDecision(gitDir, finalBinding, string(event.Kind), event.Tool, //nolint:errcheck // logging error not actionable
				string(result.Decision.Action), blockReason)

			// If the adapter returned a non-zero exit code, propagate it to the process exit.
			// Exit-status-based blocking platforms (e.g., exit-status-signal) use this to
			// communicate blocking decisions to the platform's process exit mechanism.
			// Output is written before the error is returned.
			return applyRunResult(cmd.OutOrStdout(), result)
		},
	}
}
