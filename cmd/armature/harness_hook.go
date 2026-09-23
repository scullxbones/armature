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
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/spf13/cobra"
)

func resolveIssueBinding(gitDir string) string {
	if issueID := harnesshook.ReadIssueBindingFile(gitDir); issueID != "" {
		return issueID
	}
	return os.Getenv("ARMATURE_ISSUE_ID")
}

func logPassThrough(gitDir string, reason string) error {
	return harnesshook.AppendHookLogLine(gitDir, "pass-through: "+harnesshook.SanitizeLogField(reason))
}

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

func logViolation(gitDir string, reason string) error {
	return harnesshook.AppendHookLogLine(gitDir, "violation: "+harnesshook.SanitizeLogField(reason))
}

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
	normalizedPaths := harnesshook.AbsolutizePaths(event.Paths, event.Cwd, resolvedBinding.Root)
	_, err = harnesshook.LogPassThroughScopeViolation(
		logGitDir, scopePolicy, normalizedPaths, "stale binding")
	swallowErr(err)
}

func isBindingStale(snap *snapshot.Snapshot, taskID string, now int64) bool {
	issue, ok := snap.Issues[taskID]
	if !ok {
		return true
	}
	if issue.Status != ops.StatusClaimed && issue.Status != ops.StatusInProgress {
		return true
	}
	last := claimPkg.FoldLastActivity(issue.ClaimedAt, issue.LastHeartbeat, issue.LastClaimingWorkerActivity)
	return claimPkg.IsClaimStale(last, issue.ClaimTTL, now)
}

func isFileWriteEvent(eventKind harnesshook.EventKind, filePath string) bool {
	return (eventKind == harnesshook.EventPreToolUse || eventKind == harnesshook.EventPostToolUse) && filePath != ""
}

func isKnownWorktreeGitDir(repoPath, candidateGitDir string) bool {
	if candidateGitDir == "" {
		return false
	}
	candidateAbs := resolvePathForComparison(candidateGitDir)
	if candidateAbs == "" {
		return false
	}

	if mainGitDir, err := worktree.ResolveGitDir(repoPath); err == nil {
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
		wtGitDir, err := worktree.ResolveGitDir(wtPath)
		if err != nil {
			continue
		}
		if abs := resolvePathForComparison(wtGitDir); abs != "" && abs == candidateAbs {
			return true
		}
	}
	return false
}

func resolvePathForComparison(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return ""
}

func applyRunResult(out io.Writer, result harnesshook.RunResult) error {
	_, err := out.Write(result.Output)
	swallowErr(err)
	if result.ExitCode != 0 {
		return adapterExitError{code: result.ExitCode}
	}
	return nil
}

type heartbeatRateLimitState struct {
	LastHeartbeatTime int64 `json:"last_heartbeat_time_unix"`
}

func readHeartbeatRateLimitState(workerID, issueID string) time.Time {
	stateFile := rateLimitStateFilePath(workerID, issueID)
	// #nosec G304 - stateFile is derived from workerID and issueID; ARM_LOG_SLOT
	// charset is validated by SlottedWorkerID.
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return time.Time{}
	}
	var state heartbeatRateLimitState
	if err := json.Unmarshal(data, &state); err != nil {
		return time.Time{}
	}
	return time.Unix(state.LastHeartbeatTime, 0)
}

func writeHeartbeatRateLimitState(workerID, issueID string, heartbeatTime time.Time) error {
	stateFile := rateLimitStateFilePath(workerID, issueID)
	state := heartbeatRateLimitState{
		LastHeartbeatTime: heartbeatTime.Unix(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat state: %w", err)
	}
	// #nosec G304 - stateFile is derived from workerID and issueID; ARM_LOG_SLOT
	// charset is validated by SlottedWorkerID.
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write heartbeat state file: %w", err)
	}
	return nil
}

func rateLimitStateFilePath(workerID, issueID string) string {
	filename := fmt.Sprintf("armature-heartbeat-%s-%s.json", workerID, issueID)
	return filepath.Join(os.TempDir(), filename)
}

func tryEmitHeartbeat(repoPath, issuesDir, worktreePath, issueID string, eventKind harnesshook.EventKind) {
	if eventKind != harnesshook.EventPreToolUse {
		return
	}

	workerID, err := worker.GetWorkerID(repoPath)
	if err != nil {
		return
	}

	ownerID := slottedWorkerID(workerID)

	lastHeartbeatTime := readHeartbeatRateLimitState(ownerID.String(), issueID)

	if !claimPkg.ShouldHeartbeat(lastHeartbeatTime, time.Now()) {
		return
	}

	heartbeatOp := ops.Op{
		Type:      ops.OpHeartbeat,
		TargetID:  issueID,
		Timestamp: nowEpoch(),
		WorkerID:  ownerID.String(),
		Payload: ops.Payload{
			Source: "hook",
		},
	}

	logPath := opsLogPath(issuesDir, ownerID.String())

	var gc ops.GitCommitter
	if worktreePath != "" {
		gc = adapters.New(worktreePath)
	}

	if err := ops.AppendAndCommit(logPath, worktreePath, heartbeatOp, gc); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to emit heartbeat op for %s: %v\n", issueID, err)
		return
	}

	if err := writeHeartbeatRateLimitState(ownerID.String(), issueID, time.Now()); err != nil {
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
			rawRepo, _ := cmd.Root().PersistentFlags().GetString("repo")
			if rawRepo == "" {
				rawRepo = "."
			}
			gitDir, err := worktree.ResolveGitDir(rawRepo)
			if err != nil {
				gitDir = filepath.Join(appCtx.RepoPath, ".git")
			}
			sessionBinding := resolveIssueBinding(gitDir)

			// Read hook input from stdin (before binding resolution per ADR-0007).
			// A read failure fails open: warn loudly and pass through rather than
			// blocking the platform on a nonzero exit (finding 3).
			inputData, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to read hook input: %v\n", err)
				swallowErr(logPassThrough(gitDir, "stdin read failed"))
				return nil
			}

			adapter, err := harnesshook.NewAdapterForPlatform(os.Getenv("ARMATURE_HOOK_PLATFORM"))
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to select hook adapter: %v\n", err)
				swallowErr(logPassThrough(gitDir, "adapter selection failed"))
				return nil
			}

			event, err := adapter.Decode(inputData)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to decode hook event: %v\n", err)
				swallowErr(logPassThrough(gitDir, "event decode failed"))
				return nil
			}

			filePath := harnesshook.FirstPathFromToolInput(event.ToolInput)
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
				fmt.Fprintf(cmd.ErrOrStderr(), "error: binding resolution failed: %v\n", err)
				swallowErr(logPassThrough(gitDir, "binding resolution failed"))
				return nil
			}

			logGitDir := resolvedBinding.GitDir
			pathResolved := resolvedBinding.ResolutionStep == "file_path" ||
				resolvedBinding.ResolutionStep == "event_cwd" ||
				(resolvedBinding.ResolutionStep == "" && logGitDir != gitDir)
			if pathResolved {
				if !isKnownWorktreeGitDir(rawRepo, logGitDir) {
					fmt.Fprintf(cmd.ErrOrStderr(), "error: path-resolved git dir %q is not a known worktree of %q; falling back to session binding\n", logGitDir, rawRepo)
					swallowErr(logViolation(gitDir, fmt.Sprintf("path-resolved git dir %q rejected as untrusted", logGitDir)))
					resolvedBinding = harnesshook.ResolvedBinding{
						IssueID:        sessionBinding,
						GitDir:         gitDir,
						ResolutionStep: "session",
					}
					logGitDir = gitDir
				}
			}

			if resolvedBinding.IssueID == "" {
				if isFileWriteEvent(event.Kind, filePath) {
					swallowErr(logViolation(logGitDir, "file write with no resolved binding"))
				} else {
					swallowErr(logPassThrough(logGitDir, "no issue binding found"))
				}
				return nil
			}

			store := snapshot.NewStore(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir)
			snap, err := store.Load(cmd.Context())
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to load snapshot: %v\n", err)
				swallowErr(logPassThrough(logGitDir, "snapshot load failed"))
				return nil
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			if isBindingStale(snap, resolvedBinding.IssueID, time.Now().Unix()) {
				logStalePassThroughScopeViolation(appCtx, resolvedBinding, event, logGitDir)
				swallowErr(logPassThrough(logGitDir, "stale issue binding"))
				return nil
			}

			tryEmitHeartbeat(appCtx.RepoPath, appCtx.IssuesDir, appCtx.WorktreePath, resolvedBinding.IssueID, event.Kind)

			resolver := harnesspolicy.NewIssuePolicyResolver(harnesspolicy.ResolverConfig{
				RepoPath:   appCtx.RepoPath,
				StateDir:   appCtx.StateDir,
				SourcesDir: filepath.Join(appCtx.IssuesDir, "sources"),
			})

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
				swallowErr(logPassThrough(logGitDir, "hook evaluation failed"))
				return nil
			}

			blockReason := result.Decision.Message
			swallowErr(logDecision(
				logGitDir, resolvedBinding.IssueID, resolvedBinding.ResolutionStep,
				string(event.Kind), event.Tool, string(result.Decision.Action), blockReason))

			// Capture execution evidence for shell PostToolUse events (ADR-0008).
			// Each platform names its shell tool differently (Claude: "Bash", Codex:
			// "shell"/"local_shell", Devin: "exec"), so match against the resolved
			// adapter's capability matrix rather than hardcoding "Bash" (finding: P1,
			// PR #71 review — this hardcoding silently discarded Codex/Devin evidence).
			if event.Kind == harnesshook.EventPostToolUse && resolvedBinding.IssueID != "" &&
				adapter.Capabilities().PostToolUse && slices.Contains(adapter.Capabilities().SupportedShellTools, event.Tool) {
				swallowErr(harnesshook.AppendActivity(
					logGitDir, event.Command, event.ExitCode, event.ExitCodeKnown, event.Output))
			}

			return applyRunResult(cmd.OutOrStdout(), result)
		},
	}
}
