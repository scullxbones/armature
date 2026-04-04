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
		Use:   "note [issue-id] [message]",
		Short: "Add a note to an issue",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle positional arguments: args[0] = issue-id, args[1] = message
			if len(args) >= 2 {
				issueID = args[0]
				msg = args[1]
			} else if len(args) == 1 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}
			if msg == "" {
				return fmt.Errorf("message is required (via --msg flag or positional argument)")
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Msg: msg}}
			if err := appendLowStakesOp(logPath, op); err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "note": "added"}
				data, _ := json.Marshal(result)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note added to %s\n", issueID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&msg, "msg", "", "note message")
	return cmd
}
