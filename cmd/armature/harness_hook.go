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

// resolveTaskBinding reads the task ID from <git-dir>/armature-task-id,
// falls back to ARMATURE_TASK_ID environment variable, and returns an empty
// string if neither is present.
func resolveTaskBinding(gitDir string) string {
	taskIDPath := filepath.Join(gitDir, "armature-task-id")
	// #nosec G304 - taskIDPath is derived from a trusted git directory
	if data, err := os.ReadFile(taskIDPath); err == nil {
		return strings.TrimSpace(string(data))
	}
	return os.Getenv("ARMATURE_TASK_ID")
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

// isBindingStale checks if the task binding's status is not claimed or in-progress,
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
			taskID := resolveTaskBinding(gitDir)

			// If no task binding is found, pass through with exit 0
			if taskID == "" {
				_ = logPassThrough(gitDir, "no task binding found") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Load snapshot to check if binding is stale
			snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir)
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			// If binding is stale (status != claimed/in-progress or claim TTL expired), pass through
			if isBindingStale(snap, taskID, time.Now().Unix()) {
				_ = logPassThrough(gitDir, "stale binding") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Read hook input from stdin
			inputData, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read hook input: %w", err)
			}

			// Create policy resolver
			resolver := harnesspolicy.NewTaskPolicyResolver(harnesspolicy.ResolverConfig{
				RepoPath:   appCtx.RepoPath,
				StateDir:   appCtx.StateDir,
				SourcesDir: filepath.Join(appCtx.IssuesDir, "sources"),
			})

			// Create hook and evaluate
			hook := harnesshook.NewHook(resolver)
			result, err := hook.Evaluate(cmd.Context(), harnesshook.EvaluateInput{
				Input:    inputData,
				TaskID:   taskID,
				Platform: os.Getenv("ARMATURE_HOOK_PLATFORM"),
			})
			if err != nil {
				return err
			}

			// If the adapter returned a non-zero exit code, propagate it to the process exit.
			// Exit-status-based blocking platforms (e.g., exit-status-signal) use this to
			// communicate blocking decisions to the platform's process exit mechanism.
			// Output is written before the error is returned.
			return applyRunResult(cmd.OutOrStdout(), result)
		},
	}
}
