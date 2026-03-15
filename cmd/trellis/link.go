package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var repoPath, sourceID, dep, rel string

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Add a dependency link between issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpLink, TargetID: sourceID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Dep: dep, Rel: rel}}
			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"source": sourceID, "dep": dep, "rel": rel}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&sourceID, "source", "", "source issue ID")
	cmd.Flags().StringVar(&dep, "dep", "", "dependency issue ID")
	cmd.Flags().StringVar(&rel, "rel", "blocked_by", "relationship type")
	cmd.MarkFlagRequired("source")
	cmd.MarkFlagRequired("dep")
	return cmd
}
