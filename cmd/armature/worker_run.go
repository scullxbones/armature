package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/workerruntime"
	"github.com/spf13/cobra"
)

var newWorkerRuntime = func() *workerruntime.Runtime {
	return &workerruntime.Runtime{
		Ready: noReadyProvider{},
		Claim: passClaimer{},
		Exec:  noopOrchestrator{},
	}
}

type noReadyProvider struct{}

func (noReadyProvider) NextReady(context.Context) (string, bool, error) { return "", false, nil }

type passClaimer struct{}

func (passClaimer) Claim(context.Context, string) (bool, error) { return true, nil }

type noopOrchestrator struct{}

func (noopOrchestrator) Run(context.Context, string) error { return nil }

func newWorkerRunCmd() *cobra.Command {
	var (
		maxTasks int
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the worker runtime loop",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt := newWorkerRuntime()
			res, err := rt.Run(cmd.Context(), workerruntime.RuntimeOptions{
				WorkerID: "worker",
				MaxTasks: maxTasks,
				DryRun:   dryRun,
				Policy:   workerruntime.DefaultPolicy(),
			})
			if err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				data, _ := json.Marshal(map[string]any{
					"tasks_completed": res.TasksCompleted,
					"final_state":     res.FinalState,
					"dry_run":         dryRun,
					"max_tasks":       maxTasks,
				})
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "worker run: tasks_completed=%d final_state=%s dry_run=%t max_tasks=%d\n",
				res.TasksCompleted, res.FinalState, dryRun, maxTasks)
			return nil
		},
	}
	cmd.Flags().IntVar(&maxTasks, "max-tasks", 0, "maximum tasks to execute before stopping (0 = no limit)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "inspect runtime behavior without task mutation")
	return cmd
}
