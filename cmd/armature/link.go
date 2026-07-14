package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var sourceID, dep, rel string

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Add a dependency link between issues",
		Long: `Create a dependency relationship between two issues.

Link one issue (source) to another (dependency) with a specified relationship type.
The only supported --rel value is blocked_by (source is blocked by dependency); the
inverse "blocks" relationship is derived automatically on the dependency and cannot
be set directly. Links establish the DAG structure and drive ready-queue eligibility.`,
		Example: `  # Source is blocked by another issue
  $ arm link --source E6-S4-T2 --dep E6-S4-T1 --rel blocked_by`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpLink, TargetID: sourceID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Dep: dep, Rel: rel}}
			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"source": sourceID, "dep": dep, "rel": rel}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Linked %s → %s (%s)\n", sourceID, dep, rel)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceID, "source", "", "source issue ID")
	cmd.Flags().StringVar(&dep, "dep", "", "dependency issue ID")
	cmd.Flags().StringVar(&rel, "rel", "blocked_by", "relationship type")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("dep")
	return cmd
}
