package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/decompose"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/worker"
	"github.com/spf13/cobra"
)

func newDecomposeApplyCmd() *cobra.Command {
	var planPath string

	cmd := &cobra.Command{
		Use:   "decompose-apply",
		Short: "Apply a decomposition plan to the issue graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			workerID, err := worker.GetWorkerID(appCtx.RepoPath)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			plan, err := decompose.ParsePlan(planPath)
			if err != nil {
				return err
			}

			state, _, err := materialize.MaterializeAndReturn(issuesDir, true)
			if err != nil {
				return err
			}

			opsDir := issuesDir + "/ops"
			count, err := decompose.ApplyPlan(plan, opsDir, workerID, state)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Applied %d issues from plan\n", count)
			return nil
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "path to plan JSON file")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

func newDecomposeRevertCmd() *cobra.Command {
	var planPath string

	cmd := &cobra.Command{
		Use:   "decompose-revert",
		Short: "Revert a decomposition plan from the issue graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			workerID, err := worker.GetWorkerID(appCtx.RepoPath)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			plan, err := decompose.ParsePlan(planPath)
			if err != nil {
				return err
			}

			state, _, err := materialize.MaterializeAndReturn(issuesDir, true)
			if err != nil {
				return err
			}

			opsDir := issuesDir + "/ops"
			count, err := decompose.RevertPlan(plan, opsDir, workerID, state)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reverted %d issues from plan\n", count)
			return nil
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "path to plan JSON file")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

func newDecomposeContextCmd() *cobra.Command {
	var planPath string

	cmd := &cobra.Command{
		Use:               "decompose-context",
		Short:             "Print context summary for a decomposition plan",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := decompose.ParsePlan(planPath)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), decompose.PlanContext(plan))
			return nil
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "path to plan JSON file")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}
