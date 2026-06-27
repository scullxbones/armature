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

func newScopeRenameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scope-rename <old-path> <new-path>",
		Short: "Rename a scope path across all issues (substring match)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldPath := args[0]
			newPath := args[1]

			if oldPath == "" || newPath == "" {
				return fmt.Errorf("old-path and new-path must not be empty")
			}
			if oldPath == newPath {
				return fmt.Errorf("old-path and new-path are identical: %q", oldPath)
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

			// Find issues with scope entries that contain oldPath as a substring.
			var affected []string
			for id, issue := range issues {
				for _, entry := range issue.Scope {
					if strings.Contains(entry, oldPath) {
						affected = append(affected, id)
						break
					}
				}
			}

			if len(affected) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: no issues have scope entries matching %q\n", oldPath)
				return nil
			}

			// Sort for deterministic output and op order.
			sort.Strings(affected)

			// Use the same timestamp for all ops.
			ts := nowEpoch()

			for _, id := range affected {
				op := ops.Op{
					Type:      ops.OpScopeRename,
					TargetID:  id,
					Timestamp: ts,
					WorkerID:  workerID,
					Payload: ops.Payload{
						OldPath: oldPath,
						NewPath: newPath,
					},
				}
				if err := appendLowStakesOp(state, logPath, op); err != nil {
					return fmt.Errorf("append op for %s: %w", id, err)
				}
			}

			// Refresh snapshot to apply the ops to state.
			if _, err := store.Refresh(context.Background()); err != nil {
				return fmt.Errorf("refresh snapshot: %w", err)
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]any{
					"old_path":       oldPath,
					"new_path":       newPath,
					"affected_count": len(affected),
					"affected":       affected,
				}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Renamed scope %q -> %q in %d issue(s): %s\n",
					oldPath, newPath, len(affected), strings.Join(affected, ", "))
			}
			return nil
		},
	}

	return cmd
}
