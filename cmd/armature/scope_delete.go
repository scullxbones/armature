package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

// nonTerminalStatuses is the set of statuses for which an empty scope after
// deletion is noteworthy (i.e. the issue is still active in some sense).
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

			// Read index directly from disk to scan scope entries; no rematerialization needed.
			store := newSnapshotStore(appCtx)
			index, err := store.ReadIndex()
			if err != nil {
				return fmt.Errorf("read index: %w", err)
			}

			// Find issues with an exact scope entry matching deletedPath.
			var affected []string
			for id, idxEntry := range index {
				for _, scopeEntry := range idxEntry.Scope {
					if scopeEntry == deletedPath {
						affected = append(affected, id)
						break
					}
				}
			}

			if len(affected) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: no issues have scope entry %q\n", deletedPath)
				return nil
			}

			// Sort for deterministic output and op order.
			sort.Strings(affected)

			// Use the same timestamp for all ops.
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

			// Refresh snapshot to apply the ops to state.
			snap, err := store.Load(context.Background())
			if err != nil {
				return fmt.Errorf("refresh snapshot: %w", err)
			}

			// Warn about issues that now have an empty scope and are non-terminal.
			// snap.State.Issues is always initialized after Refresh(); nil check is unnecessary.
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
