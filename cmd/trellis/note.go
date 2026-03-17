package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newNoteCmd() *cobra.Command {
	var issueID, msg string

	cmd := &cobra.Command{
		Use:   "note",
		Short: "Add a note to an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Msg: msg}}
			if err := appendLowStakesOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "note": "added"}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&msg, "msg", "", "note message")
	cmd.MarkFlagRequired("issue")
	cmd.MarkFlagRequired("msg")
	return cmd
}
