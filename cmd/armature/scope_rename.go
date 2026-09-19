package main

import (
	"context"
	"fmt"
	"strings"

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

			store := newSnapshotStore(appCtx)
			index, err := store.ReadIndex()
			if err != nil {
				return fmt.Errorf("read index: %w", err)
			}

			affected := issuesMatchingScope(index, func(scopeEntry string) bool {
				return strings.Contains(scopeEntry, oldPath)
			})

			if len(affected) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: no issues have scope entries matching %q\n", oldPath)
				return nil
			}

			ts := nowEpoch()

			proposed := make([]ops.Op, 0, len(affected))
			for _, id := range affected {
				proposed = append(proposed, ops.Op{
					Type:      ops.OpScopeRename,
					TargetID:  id,
					Timestamp: ts,
					WorkerID:  workerID,
					Payload: ops.Payload{
						OldPath: oldPath,
						NewPath: newPath,
					},
				})
			}
			if err := appendLowStakesOps(state, logPath, proposed); err != nil {
				return err
			}

			if _, err := store.Load(context.Background()); err != nil {
				return fmt.Errorf("refresh snapshot: %w", err)
			}

			writeCommandResult(cmd, map[string]any{
				"old_path":       oldPath,
				"new_path":       newPath,
				"affected_count": len(affected),
				"affected":       affected,
			}, "Renamed scope %q -> %q in %d issue(s): %s\n",
				oldPath, newPath, len(affected), strings.Join(affected, ", "))
			return nil
		},
	}

	return cmd
}
