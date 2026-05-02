package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
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

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			// Materialize to ensure state is current before scanning scope entries.
			singleBranch := appCtx.Mode == "single-branch"
			if _, err := materialize.Materialize(appCtx.IssuesDir, appCtx.StateDir, singleBranch); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			// Load materialized issues to find which ones have matching scope entries.
			issuesStateDir := filepath.Join(appCtx.StateDir, "issues")
			issues, err := materialize.LoadAllIssues(issuesStateDir)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
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
				if err := appendLowStakesOp(logPath, op); err != nil {
					return fmt.Errorf("append op for %s: %w", id, err)
				}
			}

			// Rematerialize to apply the ops to state.
			if _, err := materialize.Materialize(appCtx.IssuesDir, appCtx.StateDir, singleBranch); err != nil {
				return fmt.Errorf("rematerialize: %w", err)
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]any{
					"old_path":       oldPath,
					"new_path":       newPath,
					"affected_count": len(affected),
					"affected":       affected,
				}
				data, _ := json.Marshal(result)
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
