package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/spf13/cobra"
)

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
			taskID := os.Getenv("ARMATURE_TASK_ID")
			if taskID == "" {
				return fmt.Errorf("ARMATURE_TASK_ID is required")
			}

			adapter, err := harnesshook.NewAdapterForPlatform(os.Getenv("ARMATURE_HOOK_PLATFORM"))
			if err != nil {
				return err
			}

			input, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read hook input: %w", err)
			}

			appCtx := currentCtx(cmd)

			// Materialize state from ops (required before resolver can work)
			allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(appCtx.IssuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}
			if _, err := materialize.Materialize(appCtx.StateDir, allOps, appCtx.Mode == "single-branch", offsets); err != nil {
				return fmt.Errorf("materialize: %w", err)
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
