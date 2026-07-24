package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

// resolveIssueBinding reads the issue ID from <git-dir>/armature-issue-id
// (falling back to the legacy <git-dir>/armature-task-id file for worktrees
// claimed before the rename, commit d52d78be), then falls back to the
// ARMATURE_ISSUE_ID environment variable, and returns an empty string if
// none is present.
func resolveIssueBinding(gitDir string) string {
	if issueID := harnesshook.ReadIssueBindingFile(gitDir); issueID != "" {
		return issueID
	}
	return os.Getenv("ARMATURE_ISSUE_ID")
}

// sanitizeLogField strips newlines and carriage returns from a value before it
// is interpolated into a single-line log entry, preventing log injection where
// a crafted tool name or decision message could forge or mask a violation/decision
// line (finding 8).
func sanitizeLogField(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// appendHookLog appends a single line to <git-dir>/armature-hook.log, prefixed
// with an RFC3339 UTC timestamp so operators can correlate entries with specific
// git operations. This is the single shared implementation behind
// logPassThrough/logDecision/logViolation (finding 6).
func appendHookLog(gitDir string, line string) error {
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
	_, err = fmt.Fprintf(f, "%s %s\n", ts, line)
	return err
}

// logPassThrough logs a pass-through event to <git-dir>/armature-hook.log.
func logPassThrough(gitDir string, reason string) error {
	return appendHookLog(gitDir, "pass-through: "+sanitizeLogField(reason))
}

// logDecision logs a complete decision to <git-dir>/armature-hook.log.
// Each entry includes: issue ID, resolution step, event kind, tool, decision, and optional block reason.
func logDecision(gitDir string, issueID string, resolutionStep string, eventKind string, tool string, decision string, blockReason string) error {
	entry := fmt.Sprintf("decision: issue_id=%s resolution_step=%s event=%s tool=%s decision=%s",
		sanitizeLogField(issueID), sanitizeLogField(resolutionStep), sanitizeLogField(eventKind),
		sanitizeLogField(tool), sanitizeLogField(decision))
	if blockReason != "" {
		entry += fmt.Sprintf(" block_reason=%s", sanitizeLogField(blockReason))
	}
	return appendHookLog(gitDir, entry)
}

// logViolation logs a violation entry for file writes that resolve to no binding.
// Violations are distinguished from pass-throughs: they indicate an enforcement gap
// (unbound file write when enforcement was expected).
func logViolation(gitDir string, reason string) error {
	return appendHookLog(gitDir, "violation: "+sanitizeLogField(reason))
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
	return claimPkg.IsClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.LastClaimingWorkerActivity, issue.ClaimTTL, now)
}

// isFileWriteEvent checks if an event represents a file write operation.
// Requires a non-empty extracted file path: Bash and other tool-use events
// without a resolvable file path are not file writes and must not be logged
// as violations, even though they arrive as Pre/PostToolUse (finding 4).
func isFileWriteEvent(eventKind harnesshook.EventKind, filePath string) bool {
	return (eventKind == harnesshook.EventPreToolUse || eventKind == harnesshook.EventPostToolUse) && filePath != ""
}

// isKnownWorktreeGitDir reports whether candidateGitDir corresponds to a worktree
// of repoPath (including repoPath's own main .git), per `git worktree list`. This
// bounds trust in path-resolved git dirs: a maliciously crafted tool_input.file_path
// or cwd cannot cause the hook to read/write into an unrelated repository's git dir
// (finding 9). Resolution failures are treated as untrusted (fail closed on trust,
// not on hook execution — caller still fails open by falling back to session binding).
func isKnownWorktreeGitDir(repoPath, candidateGitDir string) bool {
	if candidateGitDir == "" {
		return false
	}
	candidateAbs := resolvePathForComparison(candidateGitDir)
	if candidateAbs == "" {
		return false
	}

	// The main repo's own .git always counts.
	if mainGitDir, err := resolveWorktreeGitDir(repoPath); err == nil {
		if abs := resolvePathForComparison(mainGitDir); abs != "" && abs == candidateAbs {
			return true
		}
	}

	// #nosec G204 - git binary and repoPath are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(output), "\n") {
		wtPath, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		wtGitDir, err := resolveWorktreeGitDir(wtPath)
		if err != nil {
			continue
		}
		if abs := resolvePathForComparison(wtGitDir); abs != "" && abs == candidateAbs {
			return true
		}
	}
	return false
}

// resolvePathForComparison resolves path to an absolute form suitable for
// comparing against `git worktree list --porcelain` output, which emits
// symlink-resolved paths. It prefers EvalSymlinks (matching isWorktreeOf's
// approach in claim.go) and falls back to Abs when EvalSymlinks fails (e.g.
// the path doesn't exist yet), so symlinked worktrees aren't falsely
// rejected as untrusted.
func resolvePathForComparison(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return ""
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

// tryEmitHeartbeat attempts to emit a rate-limited heartbeat op for a bound claim
// on every PreToolUse event. Failures are logged as warnings and do not block execution.
// Returns silently if the event is not a PreToolUse, or if the heartbeat should not
// be emitted (debounce check). The op is written directly to the ops log file.
func tryEmitHeartbeat(repoPath, issueID string, eventKind harnesshook.EventKind) {
	// Only emit heartbeats on PreToolUse events
	if eventKind != harnesshook.EventPreToolUse {
		return
	}

	// Try to get the worker ID
	workerID, err := worker.GetWorkerID(repoPath)
	if err != nil {
		// Worker not initialized; skip heartbeat emission (fail-open)
		return
	}

	// Read the rate-limit state for this worker+issue
	lastHeartbeatTime := readHeartbeatRateLimitState(workerID, issueID)

	// Check if we should emit a heartbeat using the pure decision function
	if !claimPkg.ShouldHeartbeat(lastHeartbeatTime, time.Now()) {
		return
	}

	// Emit the heartbeat op with Source="hook"
	heartbeatOp := ops.Op{
		Type:      ops.OpHeartbeat,
		TargetID:  issueID,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Source: "hook",
		},
	}

	// Try to write the op to the ops log
	ownerID := workerIdentityWithSlot(workerID)
	// Construct the log path directly using the standard .armature layout
	issuesDir := filepath.Join(repoPath, ".armature", "issues")
	logPath := filepath.Join(issuesDir, "ops", ownerID+".log")

	if err := ops.AppendAndCommit(logPath, "", heartbeatOp, nil); err != nil {
		// Log a warning but don't block
		fmt.Fprintf(os.Stderr, "warning: failed to emit heartbeat op for %s: %v\n", issueID, err)
		return
	}

	// Update the rate-limit state file to record this heartbeat
	if err := writeHeartbeatRateLimitState(workerID, issueID, time.Now()); err != nil {
		// Log a warning but don't block
		fmt.Fprintf(os.Stderr, "warning: failed to update heartbeat rate-limit state for %s: %v\n", issueID, err)
		return
	}
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

			// Read hook input from stdin (before binding resolution per ADR-0007).
			// A read failure fails open: warn loudly and pass through rather than
			// blocking the platform on a nonzero exit (finding 3).
			inputData, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to read hook input: %v\n", err)
				_ = logPassThrough(gitDir, "stdin read failed") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Decode the event to determine if path-based binding resolution is needed
			adapter, err := harnesshook.NewAdapterForPlatform(os.Getenv("ARMATURE_HOOK_PLATFORM"))
			if err != nil {
				// If we can't select an adapter, fail open and pass through with loud stderr warning
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to select hook adapter: %v\n", err)
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
			filePath := harnesshook.ExtractFilePathFromToolInput(event.ToolInput)
			eventInfo := &harnesshook.DecodedEventInfo{
				Kind:     event.Kind,
				FilePath: filePath,
				Cwd:      event.Cwd,
				Tool:     event.Tool,
			}

			// Resolve binding from event and session binding (single resolution per ADR-0007);
			// also get the git dir where it was resolved.
			// Pass the platform's supported shell tools so shell tool events skip path-based resolution.
			resolvedBinding, err := harnesshook.ResolveBindingFromEvent(eventInfo, sessionBinding, gitDir, adapter.Capabilities().SupportedShellTools)
			if err != nil {
				// If binding resolution fails, fail open and pass through with loud stderr warning
				fmt.Fprintf(cmd.ErrOrStderr(), "error: binding resolution failed: %v\n", err)
				_ = logPassThrough(gitDir, "binding resolution failed") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// If path-based resolution (steps 1-2) landed on a git dir outside the
			// invoking repo's own worktrees, don't trust it: fall back to logging
			// against the session's own git dir instead (finding 9).
			logGitDir := resolvedBinding.GitDir
			pathResolved := resolvedBinding.ResolutionStep == "file_path" ||
				resolvedBinding.ResolutionStep == "event_cwd" ||
				(resolvedBinding.ResolutionStep == "" && logGitDir != gitDir)
			if pathResolved {
				if !isKnownWorktreeGitDir(rawRepo, logGitDir) {
					fmt.Fprintf(cmd.ErrOrStderr(), "error: path-resolved git dir %q is not a known worktree of %q; falling back to session binding\n", logGitDir, rawRepo)
					_ = logViolation(gitDir, fmt.Sprintf("path-resolved git dir %q rejected as untrusted", logGitDir)) //nolint:errcheck // logging only, error not actionable
					resolvedBinding = harnesshook.ResolvedBinding{
						IssueID:        sessionBinding,
						GitDir:         gitDir,
						ResolutionStep: "session",
					}
					logGitDir = gitDir
				}
			}

			// If no binding is found:
			// - File writes are violations (enforcement gap)
			// - Other events are pass-throughs (no enforcement expected)
			if resolvedBinding.IssueID == "" {
				if isFileWriteEvent(event.Kind, filePath) {
					_ = logViolation(logGitDir, "file write with no resolved binding") //nolint:errcheck // logging only, error not actionable
				} else {
					_ = logPassThrough(logGitDir, "no issue binding found") //nolint:errcheck // logging only, error not actionable
				}
				return nil
			}

			// Load snapshot to check if binding is stale
			store := snapshot.NewStore(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir)
			snap, err := store.Load(cmd.Context())
			if err != nil {
				// Snapshot load errors are fail-open with loud stderr warning
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to load snapshot: %v\n", err)
				_ = logPassThrough(logGitDir, "snapshot load failed") //nolint:errcheck // logging only, error not actionable
				return nil
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			// If binding is stale, pass through
			if isBindingStale(snap, resolvedBinding.IssueID, time.Now().Unix()) {
				_ = logPassThrough(logGitDir, "stale issue binding") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Emit rate-limited heartbeat on PreToolUse events for bound+non-stale claims.
			// Failures are swallowed and logged as warnings, not blocking execution.
			tryEmitHeartbeat(appCtx.RepoPath, resolvedBinding.IssueID, event.Kind)

			// Create policy resolver
			resolver := harnesspolicy.NewIssuePolicyResolver(harnesspolicy.ResolverConfig{
				RepoPath:   appCtx.RepoPath,
				StateDir:   appCtx.StateDir,
				SourcesDir: filepath.Join(appCtx.IssuesDir, "sources"),
			})

			// Create hook and evaluate with the already-resolved binding.
			// For path-resolved bindings, pass the worktree root so the scope policy
			// uses it for path normalization instead of os.Getwd().
			hook := harnesshook.NewHook(resolver)
			result, err := hook.Evaluate(cmd.Context(), harnesshook.EvaluateInput{
				Input:    inputData,
				Binding:  resolvedBinding.IssueID,
				Platform: os.Getenv("ARMATURE_HOOK_PLATFORM"),
				Root:     resolvedBinding.Root,
			})
			if err != nil {
				// Evaluation errors (policy resolution, evaluator, encode) are fail-open
				// with loud stderr warning, per ADR-0007's "fail-open everywhere" (finding 3).
				fmt.Fprintf(cmd.ErrOrStderr(), "error: hook evaluation failed: %v\n", err)
				_ = logPassThrough(logGitDir, "hook evaluation failed") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Log the decision with complete information to the resolved worktree's git dir
			blockReason := result.Decision.Message
			_ = logDecision( //nolint:errcheck // logging error not actionable
				logGitDir, resolvedBinding.IssueID, resolvedBinding.ResolutionStep,
				string(event.Kind), event.Tool, string(result.Decision.Action), blockReason)

			// Capture execution evidence for shell PostToolUse events (ADR-0008).
			// Each platform names its shell tool differently (Claude: "Bash", Codex:
			// "shell"/"local_shell", Devin: "exec"), so match against the resolved
			// adapter's capability matrix rather than hardcoding "Bash" (finding: P1,
			// PR #71 review — this hardcoding silently discarded Codex/Devin evidence).
			if event.Kind == harnesshook.EventPostToolUse && resolvedBinding.IssueID != "" &&
				adapter.Capabilities().PostToolUse && slices.Contains(adapter.Capabilities().SupportedShellTools, event.Tool) {
				_ = harnesshook.AppendActivity( //nolint:errcheck // activity logging failure is fail-open
					logGitDir, event.Command, event.ExitCode, event.ExitCodeKnown, event.Output)
			}

			// If the adapter returned a non-zero exit code, propagate it to the process exit.
			// Exit-status-based blocking platforms (e.g., exit-status-signal) use this to
			// communicate blocking decisions to the platform's process exit mechanism.
			// Output is written before the error is returned.
			return applyRunResult(cmd.OutOrStdout(), result)
		},
	}
}
