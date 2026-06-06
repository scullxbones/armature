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

			adapter, err := hookAdapterForPlatform(os.Getenv("ARMATURE_HOOK_PLATFORM"))
			if err != nil {
				return err
			}

			input, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read hook input: %w", err)
			}
			event, err := adapter.Decode(input)
			if err != nil {
				return fmt.Errorf("decode hook input: %w", err)
			}

			appCtx := currentCtx(cmd)
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
			task, err := resolver.Resolve(taskID)
			if err != nil {
				return err
			}

			service := harnesspolicy.NewVerificationService()
			evaluator := harnesshook.NewEvaluator(harnesshook.EvaluatorConfig{
				ScopePolicy:         harnesspolicy.NewScopePolicy(task.Scope),
				VerificationService: &service,
				VerificationInput: harnesspolicy.VerificationRequest{
					Acceptance: task.Acceptance,
					Citations:  task.Citations,
				},
			})

			decision, err := evaluator.Evaluate(cmd.Context(), event)
			if err != nil {
				return err
			}

			output, exitCode, err := adapter.Encode(event, decision)
			if err != nil {
				return err
			}
			if _, err := cmd.OutOrStdout().Write(output); err != nil {
				return err
			}
			if exitCode != 0 {
				return fmt.Errorf("hook blocked: %s", decision.Message)
			}
			return nil
		},
	}
}

func hookAdapterForPlatform(platform string) (harnesshook.PlatformAdapter, error) {
	switch platform {
	case "", "claude":
		return harnesshook.NewClaudeAdapter(), nil
	case "codex":
		return harnesshook.NewCodexAdapter(), nil
	case "devin":
		return harnesshook.NewDevinAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown harness hook platform %q", platform)
	}
}
