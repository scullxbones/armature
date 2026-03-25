package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scullxbones/trellis/internal/decompose"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/worker"
	"github.com/spf13/cobra"
)

func newDecomposeApplyCmd() *cobra.Command {
	var planPath string
	var exampleFlag bool

	cmd := &cobra.Command{
		Use:   "decompose-apply",
		Short: "Apply a decomposition plan to the issue graph",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if exampleFlag {
				return nil
			}
			// Fall through to root PersistentPreRunE for normal config loading.
			return cmd.Root().PersistentPreRunE(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if exampleFlag {
				example := decompose.Plan{
					Version: 1,
					Title:   "Example Decomposition Plan",
					Issues: []decompose.PlanIssue{
						{
							ID:    "STORY-001",
							Title: "User authentication story",
							Type:  "story",
						},
						{
							ID:        "TASK-001",
							Title:     "Implement login endpoint",
							Type:      "task",
							Parent:    "STORY-001",
							Priority:  "high",
							DoD:       "Login endpoint returns JWT on valid credentials",
							BlockedBy: []string{},
						},
						{
							ID:        "TASK-002",
							Title:     "Write login integration tests",
							Type:      "task",
							Parent:    "STORY-001",
							Priority:  "medium",
							DoD:       "Integration tests cover happy path and error cases",
							BlockedBy: []string{"TASK-001"},
						},
					},
				}
				out, err := json.MarshalIndent(example, "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			if planPath == "" {
				return fmt.Errorf("required flag \"plan\" not set")
			}

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
	cmd.Flags().BoolVar(&exampleFlag, "example", false, "print a minimal valid example plan JSON and exit")
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
	var sourcesFlag string
	var templateFlag string
	var outputFlag string
	var formatFlag string
	var existingDAGFlag bool

	cmd := &cobra.Command{
		Use:               "decompose-context",
		Short:             "Build decomposition context with template interpolation",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			var plan *decompose.Plan
			if planPath != "" {
				var err error
				plan, err = decompose.ParsePlan(planPath)
				if err != nil {
					return err
				}
			}

			var sourceIDs []string
			if sourcesFlag != "" {
				for _, s := range strings.Split(sourcesFlag, ",") {
					s = strings.TrimSpace(s)
					if s != "" {
						sourceIDs = append(sourceIDs, s)
					}
				}
			}

			issuesDir := ""
			if appCtx != nil {
				issuesDir = appCtx.IssuesDir
			}
			ctx, err := decompose.BuildContext(decompose.ContextParams{
				IssuesDir:   issuesDir,
				Plan:        plan,
				SourceIDs:   sourceIDs,
				Template:    templateFlag,
				ExistingDAG: existingDAGFlag,
			})
			if err != nil {
				return err
			}

			var out []byte
			if formatFlag == "json" {
				out, err = json.MarshalIndent(ctx, "", "  ")
				if err != nil {
					return err
				}
			} else {
				out = []byte(ctx.PromptTemplate)
			}

			if outputFlag != "" {
				return os.WriteFile(outputFlag, out, 0o644)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "path to plan JSON file")
	cmd.Flags().StringVar(&sourcesFlag, "sources", "", "comma-separated source IDs to include")
	cmd.Flags().StringVar(&templateFlag, "template", "", "prompt template with {{SOURCES}}/{{EXISTING_DAG}}/{{PLAN_SCHEMA}}/{{CONSTRAINTS}} placeholders")
	cmd.Flags().StringVar(&outputFlag, "output", "", "write output to file instead of stdout")
	cmd.Flags().StringVar(&formatFlag, "format", "text", "output format: text or json")
	cmd.Flags().BoolVar(&existingDAGFlag, "existing-dag", false, "include existing DAG issues in context")
	return cmd
}
