package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

var nonTerminalStatuses = map[string]bool{
	ops.StatusOpen:       true,
	ops.StatusClaimed:    true,
	ops.StatusInProgress: true,
	ops.StatusBlocked:    true,
}

func newScopeDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scope-delete <path>",
		Short: "Remove an exact scope entry from all issues that have it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deletedPath := args[0]

			if deletedPath == "" {
				return fmt.Errorf("path must not be empty")
			}

			state := mustState(cmd)
			appCtx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return err
			}

			store := newSnapshotStore(appCtx)
			index, err := store.ReadIndex()
			if err != nil {
				return fmt.Errorf("read index: %w", err)
			}

			affected := issuesMatchingScope(index, func(scopeEntry string) bool {
				return scopeEntry == deletedPath
			})

			if len(affected) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: no issues have scope entry %q\n", deletedPath)
				return nil
			}

			ts := nowEpoch()

			proposed := make([]ops.Op, 0, len(affected))
			for _, id := range affected {
				proposed = append(proposed, ops.Op{
					Type:      ops.OpScopeDelete,
					TargetID:  id,
					Timestamp: ts,
					WorkerID:  workerID,
					Payload: ops.Payload{
						DeletedPath: deletedPath,
					},
				})
			}
			if err := appendLowStakesOps(state, logPath, proposed); err != nil {
				return err
			}

			snap, err := store.Load(context.Background())
			if err != nil {
				return fmt.Errorf("refresh snapshot: %w", err)
			}

			updatedIssues := snap.State.Issues
			for _, id := range affected {
				issue, ok := updatedIssues[id]
				if !ok {
					continue
				}
				if nonTerminalStatuses[issue.Status] && len(issue.Scope) == 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"warning: issue %s now has an empty scope (status: %s)\n", id, issue.Status)
				}
			}

			writeCommandResult(cmd, map[string]any{
				"deleted_path":   deletedPath,
				"affected_count": len(affected),
				"affected":       affected,
			}, "Deleted scope %q from %d issue(s): %s\n",
				deletedPath, len(affected), strings.Join(affected, ", "))
			return nil
		},
	}

	return cmd
}
