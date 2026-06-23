package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/harnesspolicy"
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

// logPassThrough logs a pass-through event to <git-dir>/armature-hook.log
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
	_, err = fmt.Fprintf(f, "pass-through: %s\n", reason)
	return err
}

// isBindingStale checks if the task binding's status is not "claimed" or "in-progress"
func isBindingStale(snap *snapshot.Snapshot, taskID string) bool {
	issue, ok := snap.Issues[taskID]
	if !ok {
		return true // Missing issue = stale
	}
	return issue.Status != "claimed" && issue.Status != "in-progress"
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
			gitDir := filepath.Join(appCtx.RepoPath, ".git")
			taskID := resolveTaskBinding(gitDir)

			// If no task binding is found, pass through with exit 0
			if taskID == "" {
				_ = logPassThrough(gitDir, "no task binding found") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			// Load snapshot to check if binding is stale
			snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir, appCtx.Mode == "single-branch")
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			// If binding is stale (status != claimed/in-progress), pass through
			if isBindingStale(snap, taskID) {
				_ = logPassThrough(gitDir, "stale binding") //nolint:errcheck // logging only, error not actionable
				return nil
			}

			adapter, err := harnesshook.NewAdapterForPlatform(os.Getenv("ARMATURE_HOOK_PLATFORM"))
			if err != nil {
				return err
			}

			input, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read hook input: %w", err)
			}

			resolver := harnesspolicy.NewTaskPolicyResolver(harnesspolicy.ResolverConfig{
				RepoPath:   appCtx.RepoPath,
				StateDir:   appCtx.StateDir,
				SourcesDir: filepath.Join(appCtx.IssuesDir, "sources"),
			})

			runner := harnesshook.NewRunner(&harnesshook.RunnerConfig{
				Adapter:   adapter,
				Resolver:  resolver,
				Evaluator: nil, // Will be created in runner based on resolved policy
				TaskID:    taskID,
				IssuesDir: appCtx.IssuesDir,
				StateDir:  appCtx.StateDir,
			})

			result, err := runner.Run(cmd.Context(), input)
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
