package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
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

			// Load snapshot to ensure state is current before scanning scope entries.
			store := newSnapshotStore(appCtx)
			snap, err := store.Load(context.Background())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}

			// Get issues from snapshot
			issues := snap.State.Issues
			if issues == nil {
				issues = make(map[string]*materialize.Issue)
			}

			// Find issues with an exact scope entry matching deletedPath.
			var affected []string
			for id, issue := range issues {
				for _, entry := range issue.Scope {
					if entry == deletedPath {
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

			for _, id := range affected {
				op := ops.Op{
					Type:      ops.OpScopeDelete,
					TargetID:  id,
					Timestamp: ts,
					WorkerID:  workerID,
					Payload: ops.Payload{
						DeletedPath: deletedPath,
					},
				}
				if err := appendLowStakesOp(state, logPath, op); err != nil {
					return fmt.Errorf("append op for %s: %w", id, err)
				}
			}

			// Refresh snapshot to apply the ops to state.
			snap, err = store.Refresh(context.Background())
			if err != nil {
				return fmt.Errorf("refresh snapshot: %w", err)
			}

			// Warn about issues that now have an empty scope and are non-terminal.
			updatedIssues := snap.State.Issues
			if updatedIssues == nil {
				updatedIssues = make(map[string]*materialize.Issue)
			}
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

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]any{
					"deleted_path":   deletedPath,
					"affected_count": len(affected),
					"affected":       affected,
				}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deleted scope %q from %d issue(s): %s\n",
					deletedPath, len(affected), strings.Join(affected, ", "))
			}
			return nil
		},
	}

	return cmd
}
