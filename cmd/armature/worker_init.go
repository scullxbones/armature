package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

func newWorkerInitCmd() *cobra.Command {
	var check bool
	var repoPath string

	cmd := &cobra.Command{
		Use:               "worker-init",
		Short:             "Generate or check worker identity",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}

			if check {
				ok, id := worker.CheckWorkerID(repoPath)
				if !ok {
					return fmt.Errorf("no worker ID configured — run 'trls worker-init'")
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Worker ID: %s\n", id)
				return nil
			}

			id, err := worker.InitWorker(repoPath)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Worker ID: %s\n", id)
			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "verify existing worker ID without modifying state")
	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")

	return cmd
}
