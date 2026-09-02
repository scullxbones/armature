package main

import (
	"fmt"
	"time"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newNoteCmd() *cobra.Command {
	var issueID, msg, noteID string

	cmd := &cobra.Command{
		Use:   "note [issue-id] [message]",
		Short: "Add or delete a note on an issue",
		Args:  cobra.MaximumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			isDelete := len(args) > 0 && args[0] == "delete"
			if isDelete {
				if len(args) >= 3 {
					issueID = args[1]
					noteID = args[2]
				} else if len(args) == 2 {
					issueID = args[1]
				}
				return runNoteDelete(cmd, issueID, noteID)
			}

			// Handle positional arguments: args[0] = issue-id, args[1] = message
			if len(args) >= 2 {
				issueID = args[0]
				msg = args[1]
			} else if len(args) == 1 {
				issueID = args[0]
			}
			return runNoteAdd(cmd, issueID, msg)
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&msg, "msg", "", "note message")
	cmd.Flags().StringVar(&noteID, "note-id", "", "note ID for deletion")
	return cmd
}

func runNoteAdd(cmd *cobra.Command, issueID, msg string) error {
	if issueID == "" {
		return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
	}
	if msg == "" {
		return fmt.Errorf("message is required (via --msg flag or positional argument)")
	}

	state := mustState(cmd)
	ctx := state.ctx
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("note-%d", time.Now().UnixNano())
	op := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
		WorkerID: workerID, Payload: ops.Payload{Msg: msg, NoteID: id}}
	if err := appendLowStakesOp(state, logPath, op); err != nil {
		return err
	}
	writeCommandResult(cmd, map[string]string{"issue": issueID, "note": "added", "note_id": id},
		"Note %s added to %s\n", id, issueID)
	return nil
}

func runNoteDelete(cmd *cobra.Command, issueID, noteID string) error {
	if issueID == "" {
		return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
	}
	if noteID == "" {
		return fmt.Errorf("note ID is required (via --note-id flag or positional argument)")
	}

	state := mustState(cmd)
	ctx := state.ctx
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	if err != nil {
		return err
	}
	op := ops.Op{Type: ops.OpNoteDelete, TargetID: issueID, Timestamp: nowEpoch(),
		WorkerID: workerID, Payload: ops.Payload{NoteID: noteID}}
	if err := appendLowStakesOp(state, logPath, op); err != nil {
		return err
	}
	writeCommandResult(cmd, map[string]string{"issue": issueID, "note": "deleted", "note_id": noteID},
		"Note %s deleted from %s\n", noteID, issueID)
	return nil
}
