package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
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

// logPassThrough logs a pass-through event to <git-dir>/armature-hook.log.
func logPassThrough(gitDir string, reason string) error {
	return harnesshook.AppendHookLogLine(gitDir, "pass-through: "+harnesshook.SanitizeLogField(reason))
}

// logDecision logs a complete decision to <git-dir>/armature-hook.log.
// Each entry includes: issue ID, resolution step, event kind, tool, decision, and optional block reason.
func logDecision(gitDir string, issueID string, resolutionStep string, eventKind string, tool string, decision string, blockReason string) error {
	entry := fmt.Sprintf("decision: issue_id=%s resolution_step=%s event=%s tool=%s decision=%s",
		harnesshook.SanitizeLogField(issueID), harnesshook.SanitizeLogField(resolutionStep),
		harnesshook.SanitizeLogField(eventKind), harnesshook.SanitizeLogField(tool),
		harnesshook.SanitizeLogField(decision))
	if blockReason != "" {
		entry += fmt.Sprintf(" block_reason=%s", harnesshook.SanitizeLogField(blockReason))
	}
	return harnesshook.AppendHookLogLine(gitDir, entry)
}

// logViolation logs a violation entry for file writes that resolve to no binding.
// Violations are distinguished from pass-throughs: they indicate an enforcement gap
// (unbound file write when enforcement was expected).
func logViolation(gitDir string, reason string) error {
	return harnesshook.AppendHookLogLine(gitDir, "violation: "+harnesshook.SanitizeLogField(reason))
}

// logStalePassThroughScopeViolation checks the event's paths against the stale
// binding's declared scope and, if any are out of scope, logs a violation
// entry. This covers the pass-through-with-violation case: the hook's
// enforcement is skipped for a stale claim (fail-open), but the out-of-scope
// operation still represents an enforcement gap worth recording, per
// docs/harness-hook.md's "Scope Violation Visibility" contract ("logged...even
// when the hook blocks or passes through the operation"). Best-effort: any
// resolution failure is swallowed since enforcement is already skipped here.
func logStalePassThroughScopeViolation(appCtx *config.Context, resolvedBinding harnesshook.ResolvedBinding, event harnesshook.Event, logGitDir string) {
	if len(event.Paths) == 0 {
		return
	}
	resolver := harnesspolicy.NewIssuePolicyResolver(harnesspolicy.ResolverConfig{
		RepoPath:   appCtx.RepoPath,
		StateDir:   appCtx.StateDir,
		SourcesDir: filepath.Join(appCtx.IssuesDir, "sources"),
	})
	policy, err := resolver.Resolve(resolvedBinding.IssueID)
	if err != nil {
		return
	}
	var scopePolicy harnesspolicy.ScopePolicy
	if resolvedBinding.Root != "" {
		scopePolicy = harnesspolicy.NewScopePolicyWithRoot(policy.Scope, resolvedBinding.Root)
	} else {
		scopePolicy = harnesspolicy.NewScopePolicy(policy.Scope)
	}
	// Normalize event.Paths against event.Cwd/resolvedBinding.Root before checking
	// scope: this event was decoded directly (line ~387) and never passed through
	// harnesshook.Hook.Evaluate's absolutization step, so a relative path with a
	// cwd below the worktree root would otherwise be checked textually against
	// the raw scope entries instead of the actual absolute write location.
	normalizedPaths := harnesshook.AbsolutizePaths(event.Paths, event.Cwd, resolvedBinding.Root)
	_, _ = harnesshook.LogPassThroughScopeViolation( //nolint:errcheck // logging only, error not actionable
		logGitDir, scopePolicy, normalizedPaths, "stale binding")
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

// heartbeatRateLimitState holds the last heartbeat time for debouncing.
type heartbeatRateLimitState struct {
	LastHeartbeatTime int64 `json:"last_heartbeat_time_unix"`
}

// readHeartbeatRateLimitState reads the rate-limit state from the OS temp directory
// for the given worker+issue combination. Returns a zero time if the state file
// doesn't exist or cannot be read.
func readHeartbeatRateLimitState(workerID, issueID string) time.Time {
	stateFile := rateLimitStateFilePath(workerID, issueID)
	// #nosec G304 - stateFile is derived from workerID and issueID; any ARM_LOG_SLOT
	// component is validated against a safe charset by workerIdentityWithSlot, so
	// this path is controlled by us.
	data, err := os.ReadFile(stateFile)
	if err != nil {
		// File doesn't exist or can't be read; return zero time (no prior heartbeat)
		return time.Time{}
	}
	var state heartbeatRateLimitState
	if err := json.Unmarshal(data, &state); err != nil {
		// Malformed state; return zero time (assume no prior heartbeat)
		return time.Time{}
	}
	return time.Unix(state.LastHeartbeatTime, 0)
}

// writeHeartbeatRateLimitState writes the rate-limit state to the OS temp directory
// for the given worker+issue combination.
func writeHeartbeatRateLimitState(workerID, issueID string, heartbeatTime time.Time) error {
	stateFile := rateLimitStateFilePath(workerID, issueID)
	state := heartbeatRateLimitState{
		LastHeartbeatTime: heartbeatTime.Unix(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat state: %w", err)
	}
	// #nosec G304 - stateFile is derived from workerID and issueID; any ARM_LOG_SLOT
	// component is validated against a safe charset by workerIdentityWithSlot, so
	// this path is controlled by us.
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write heartbeat state file: %w", err)
	}
	return nil
}

// rateLimitStateFilePath returns the path to the rate-limit state file in the OS
// temp directory for the given worker+issue combination.
func rateLimitStateFilePath(workerID, issueID string) string {
	// Create a unique filename based on worker ID and issue ID to avoid collisions
	filename := fmt.Sprintf("armature-heartbeat-%s-%s.json", workerID, issueID)
	return filepath.Join(os.TempDir(), filename)
}

// tryEmitHeartbeat attempts to emit a rate-limited heartbeat op for a bound claim
// on every PreToolUse event. Failures are logged as warnings and do not block execution.
// Returns silently if the event is not a PreToolUse, or if the heartbeat should not
// be emitted (debounce check). The op is written directly to the ops log file.
//
// repoPath is used to resolve the worker identity (git config lives at the repo
// root); issuesDir and worktreePath must come from the resolved config.Context
// (appCtx.IssuesDir / appCtx.WorktreePath) so the op lands in the same ops
// directory materialize/snapshot actually read, in both collapsed
// (worktreePath == "") and dual-branch layouts.
func tryEmitHeartbeat(repoPath, issuesDir, worktreePath, issueID string, eventKind harnesshook.EventKind) {
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

	// ownerID is the slotted identity (a no-op when ARM_LOG_SLOT is unset). It
	// must be used for the op's WorkerID and the log path: op.WorkerID has
	// to match whatever ClaimedBy was set to at claim time (always the slotted
	// identity), or applyHeartbeat's op.WorkerID == issue.ClaimedBy guard silently
	// discards the heartbeat. It's also used to key rate-limit state so that two
	// slots for the same base worker debounce independently, matching their
	// independent ops logs.
	ownerID := workerIdentityWithSlot(workerID)

	// Read the rate-limit state for this worker+issue
	lastHeartbeatTime := readHeartbeatRateLimitState(ownerID, issueID)

	// Check if we should emit a heartbeat using the pure decision function
	if !claimPkg.ShouldHeartbeat(lastHeartbeatTime, time.Now()) {
		return
	}

	// Emit the heartbeat op with Source="hook"
	heartbeatOp := ops.Op{
		Type:      ops.OpHeartbeat,
		TargetID:  issueID,
		Timestamp: nowEpoch(),
		WorkerID:  ownerID,
		Payload: ops.Payload{
			Source: "hook",
		},
	}

	logPath := opsLogPath(issuesDir, ownerID)

	var gc ops.GitCommitter
	if worktreePath != "" {
		gc = adapters.New(worktreePath)
	}

	if err := ops.AppendAndCommit(logPath, worktreePath, heartbeatOp, gc); err != nil {
		// Log a warning but don't block
		fmt.Fprintf(os.Stderr, "warning: failed to emit heartbeat op for %s: %v\n", issueID, err)
		return
	}

	// Update the rate-limit state file to record this heartbeat
	if err := writeHeartbeatRateLimitState(ownerID, issueID, time.Now()); err != nil {
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
		Annotations:   output.MarkProtocolOutput(nil),
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

			// If binding is stale, pass through. Enforcement is skipped for stale
			// claims, but out-of-scope paths on this event are still an
			// enforcement gap worth surfacing: check them against the task's
			// declared scope (if resolvable) and log a violation marker
			// alongside the pass-through entry so operators can see what
			// would have been blocked had the claim still been active.
			if isBindingStale(snap, resolvedBinding.IssueID, time.Now().Unix()) {
				logStalePassThroughScopeViolation(appCtx, resolvedBinding, event, logGitDir)
				_ = logPassThrough(logGitDir, "stale issue binding") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Emit rate-limited heartbeat on PreToolUse events for bound+non-stale claims.
			// Failures are swallowed and logged as warnings, not blocking execution.
			tryEmitHeartbeat(appCtx.RepoPath, appCtx.IssuesDir, appCtx.WorktreePath, resolvedBinding.IssueID, event.Kind)

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
