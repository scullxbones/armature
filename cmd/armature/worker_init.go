package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/config"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

func newWorkerInitCmd() *cobra.Command {
	var check, reuse bool
	var repoPath, explicitID string

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
					return fmt.Errorf("no worker ID configured — run 'arm worker-init'")
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Worker ID: %s\n", id)
				return nil
			}

			issuesDir := ""
			if ctx, err := config.ResolveContext(repoPath); err == nil {
				issuesDir = ctx.IssuesDir
			}
			ident, err := worker.Init(worker.InitWorkerInput{
				RepoPath:  repoPath,
				IssuesDir: issuesDir,
				ID:        explicitID,
				Reuse:     reuse,
			})
			if err != nil {
				return mapWorkerInitError(err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Worker ID: %s\n", ident.ID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "verify existing worker ID without modifying state")
	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	cmd.Flags().StringVar(&explicitID, "id", "", "worker id to write (worktree-scoped); omit to generate a UUID")
	cmd.Flags().BoolVar(&reuse, "reuse", false, "allow --id that already has a published log")

	return cmd
}

func mapWorkerInitError(err error) error {
	if err == nil {
		return nil
	}
	return armerrors.Wrap("WORKER-INIT-1", err.Error(), []string{"arm worker-init", "arm doctor"}, err)
}
