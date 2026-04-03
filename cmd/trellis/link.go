package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var sourceID, dep, rel string

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Add a dependency link between issues",
		Long: `Create a dependency relationship between two issues.

Link one issue (source) to another (dependency) with a specified relationship type.
Valid relationship types include: depends-on (source depends on dependency), blocks
(source blocks dependency), and relates-to (informational connection). Links establish
the DAG structure and help identify blocking dependencies.`,
		Example: `  # Source depends on another issue
  $ trls link --source E6-S4-T2 --dep E6-S4-T1 --rel depends-on

  # Source blocks another issue
  $ trls link --source E6-S4-T1 --dep E6-S4-T3 --rel blocks

  # Informational relationship
  $ trls link --source E6-S4-T2 --dep E5-S2-T1 --rel relates-to`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpLink, TargetID: sourceID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Dep: dep, Rel: rel}}
			if err := appendOp(logPath, op); err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"source": sourceID, "dep": dep, "rel": rel}
				data, _ := json.Marshal(result)
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
