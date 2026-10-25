package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/spf13/cobra"
)

func issueBindingFromGitDirOrEnvLookup(gitDir string, env Env) string {
	if issueID, err := worktree.ReadBinding(gitDir); err == nil && issueID != "" {
		return issueID
	}
	return envLookup(env, "ARMATURE_ISSUE_ID")
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
	policy, err := newIssuePolicyLoader(
		appCtx.StateDir,
		filepath.Join(appCtx.IssuesDir, "sources"),
	).Resolve(resolvedBinding.IssueID)
	if err != nil {
		return
	}
	normalizedPaths := harnesshook.AbsolutizePaths(event.Paths, event.Cwd, resolvedBinding.Root)
	_, err = harnesshook.LogPassThroughScopeViolation(
		logGitDir, policy.Scope, resolvedBinding.Root, normalizedPaths, "stale binding")
	swallowErr(err)
}

func hasNoLiveClaim(snap *snapshot.Snapshot, taskID string, now int64) bool {
	issue, ok := snap.Issues[taskID]
	if !ok {
		return true
	}
	if issue.Status != ops.StatusClaimed && issue.Status != ops.StatusInProgress {
		return true
	}
	return issue.ClaimedBy == "" || issue.ClaimStale(now)
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

	paths, err := worktree.RegisteredPaths(repoPath)
	if err != nil {
		return false
	}
	for _, wtPath := range paths {
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

func failOpenPassThrough(w io.Writer, gitDir, userMsg, logReason string) error {
	fmt.Fprintf(w, "error: %s\n", userMsg)
	swallowErr(logPassThrough(gitDir, logReason))
	return nil
}

func shouldCaptureShellPostToolUseEvidence(event harnesshook.Event, caps harnesshook.PlatformCapabilities, issueID string) bool {
	return event.Kind == harnesshook.EventPostToolUse &&
		issueID != "" &&
		caps.PostToolUse &&
		slices.Contains(caps.SupportedShellTools, event.Tool)
}

func writeHarnessHookRunResult(out io.Writer, result harnesshook.RunResult) error {
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

func readHeartbeatRateLimitStateAt(tempDir, workerID, issueID string) time.Time {
	root, err := os.OpenRoot(tempDir)
	if err != nil {
		return time.Time{}
	}
	defer func() { swallowErr(root.Close()) }()
	data, err := root.ReadFile(rateLimitStateFileName(workerID, issueID))
	if err != nil {
		return time.Time{}
	}
	var state heartbeatRateLimitState
	if err := json.Unmarshal(data, &state); err != nil {
		return time.Time{}
	}
	return time.Unix(state.LastHeartbeatTime, 0)
}

func writeHeartbeatRateLimitStateAt(tempDir, workerID, issueID string, heartbeatTime time.Time) error {
	state := heartbeatRateLimitState{
		LastHeartbeatTime: heartbeatTime.Unix(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat state: %w", err)
	}
	root, err := os.OpenRoot(tempDir)
	if err != nil {
		return fmt.Errorf("failed to write heartbeat state file: %w", err)
	}
	defer func() { swallowErr(root.Close()) }()
	if err := root.WriteFile(rateLimitStateFileName(workerID, issueID), data, 0o600); err != nil {
		return fmt.Errorf("failed to write heartbeat state file: %w", err)
	}
	return nil
}

func rateLimitStateFileName(workerID, issueID string) string {
	return fmt.Sprintf("armature-heartbeat-%s-%s.json", workerID, issueID)
}

func rateLimitStateFilePathAt(tempDir, workerID, issueID string) string {
	return filepath.Join(tempDir, rateLimitStateFileName(workerID, issueID))
}

func tryEmitHeartbeatAt(repoPath, issuesDir, worktreePath, issueID string, eventKind harnesshook.EventKind, env Env, tempDir string, errOut io.Writer) {
	if eventKind != harnesshook.EventPreToolUse {
		return
	}

	workerID, err := worker.GetWorkerID(repoPath)
	if err != nil {
		return
	}

	ownerID, slotErr := slottedWorkerIDCheckedEnv(workerID, env)
	if slotErr != nil {
		fmt.Fprintf(errOut, "warning: invalid ARM_LOG_SLOT, skipping heartbeat: %v\n", slotErr)
		return
	}

	lastHeartbeatTime := readHeartbeatRateLimitStateAt(tempDir, ownerID, issueID)

	if !claimPkg.ShouldHeartbeat(lastHeartbeatTime, time.Now()) {
		return
	}

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
		gc = gitWith(env, worktreePath)
	}

	if err := ops.AppendAndCommit(logPath, worktreePath, heartbeatOp, gc); err != nil {
		fmt.Fprintf(errOut, "warning: failed to emit heartbeat op for %s: %v\n", issueID, err)
		return
	}

	if err := writeHeartbeatRateLimitStateAt(tempDir, ownerID, issueID, time.Now()); err != nil {
		fmt.Fprintf(errOut, "warning: failed to update heartbeat rate-limit state for %s: %v\n", issueID, err)
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
			env := envFrom(cmd)
			rawRepo := repoPathFromCmd(cmd)
			gitDir, err := worktree.ResolveGitDir(rawRepo)
			if err != nil {
				gitDir = filepath.Join(appCtx.RepoPath, ".git")
			}
			sessionBinding := issueBindingFromGitDirOrEnvLookup(gitDir, env)

			inputData, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return failOpenPassThrough(cmd.ErrOrStderr(), gitDir, fmt.Sprintf("failed to read hook input: %v", err), "stdin read failed")
			}

			adapter, err := harnesshook.NewAdapterForPlatform(envLookup(env, "ARMATURE_HOOK_PLATFORM"))
			if err != nil {
				return failOpenPassThrough(cmd.ErrOrStderr(), gitDir, fmt.Sprintf("failed to select hook adapter: %v", err), "adapter selection failed")
			}

			event, err := adapter.Decode(inputData)
			if err != nil {
				return failOpenPassThrough(cmd.ErrOrStderr(), gitDir, fmt.Sprintf("failed to decode hook event: %v", err), "event decode failed")
			}

			filePath := harnesshook.FirstPathFromToolInput(event.ToolInput)
			eventInfo := &harnesshook.DecodedEventInfo{
				Kind:     event.Kind,
				FilePath: filePath,
				Cwd:      event.Cwd,
				Tool:     event.Tool,
			}

			resolvedBinding, err := harnesshook.ResolveBindingFromEvent(eventInfo, sessionBinding, gitDir, adapter.Capabilities().SupportedShellTools)
			if err != nil {
				return failOpenPassThrough(cmd.ErrOrStderr(), gitDir, fmt.Sprintf("binding resolution failed: %v", err), "binding resolution failed")
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
				return failOpenPassThrough(cmd.ErrOrStderr(), logGitDir, fmt.Sprintf("failed to load snapshot: %v", err), "snapshot load failed")
			}
			emitSnapWarnings(cmd.ErrOrStderr(), snap.Warnings)

			if hasNoLiveClaim(snap, resolvedBinding.IssueID, time.Now().Unix()) {
				logStalePassThroughScopeViolation(appCtx, resolvedBinding, event, logGitDir)
				swallowErr(logPassThrough(logGitDir, "stale issue binding"))
				return nil
			}

			tryEmitHeartbeatAt(appCtx.RepoPath, appCtx.IssuesDir, appCtx.WorktreePath, resolvedBinding.IssueID, event.Kind, env, tempDirFrom(cmd), cmd.ErrOrStderr())

			hook := harnesshook.NewHook(newIssuePolicyLoader(
				appCtx.StateDir,
				filepath.Join(appCtx.IssuesDir, "sources"),
			))
			root := resolvedBinding.Root
			if root == "" {
				root = invocationDir(cmd)
			}
			result, err := hook.Evaluate(cmd.Context(), harnesshook.EvaluateInput{
				Input:    inputData,
				Binding:  resolvedBinding.IssueID,
				Platform: envLookup(env, "ARMATURE_HOOK_PLATFORM"),
				Root:     root,
			})
			if err != nil {
				return failOpenPassThrough(cmd.ErrOrStderr(), logGitDir, fmt.Sprintf("hook evaluation failed: %v", err), "hook evaluation failed")
			}

			blockReason := result.Decision.Message
			swallowErr(logDecision(
				logGitDir, resolvedBinding.IssueID, resolvedBinding.ResolutionStep,
				string(event.Kind), event.Tool, string(result.Decision.Action), blockReason))

			if shouldCaptureShellPostToolUseEvidence(event, adapter.Capabilities(), resolvedBinding.IssueID) {
				swallowErr(harnesshook.AppendActivity(
					logGitDir, event.Command, event.ExitCode, event.ExitCodeKnown, event.Output))
			}

			return writeHarnessHookRunResult(cmd.OutOrStdout(), result)
		},
	}
}
