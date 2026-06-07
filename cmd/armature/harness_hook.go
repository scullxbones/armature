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

func newHarnessHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "harness-hook",
		Short:  "Internal harness hook entrypoint",
		Hidden: true,
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

			if _, err := cmd.OutOrStdout().Write(result.Output); err != nil {
				return err
			}

			// Handle exit code: if Block decision, return error that classifyError will map to exit code 1
			if result.ExitCode != 0 {
				return fmt.Errorf("hook blocked")
			}
			return nil
		},
	}
}
