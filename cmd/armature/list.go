package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
)

const listShowHelp = output.ListShowHelp

type listEntry = output.ListIssue
type listGroup = output.ListGroup

// terminalStatuses is the set of statuses that represent terminal (completed) states.
var terminalStatuses = map[string]bool{
	ops.StatusDone:      true,
	ops.StatusMerged:    true,
	ops.StatusCancelled: true,
}

func newListCmd() *cobra.Command {
	var filterParent string
	var filterType string
	var filterStatus string
	var group bool
	var terminal bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues with optional --type, --parent, and --status filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)
			store := newSnapshotStore(ctx)
			snap, err := store.Load(context.Background())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}

			index := snap.Index
			if index == nil {
				index = make(materialize.Index)
			}

			var ids []string
			for id, entry := range index {
				if filterParent != "" && entry.Parent != filterParent {
					continue
				}
				if filterType != "" && entry.Type != filterType {
					continue
				}
				if filterStatus != "" && entry.Status != filterStatus {
					continue
				}
				if terminal && !terminalStatuses[entry.Status] {
					continue
				}
				ids = append(ids, id)
			}
			sort.Strings(ids)

			filtered := filterParent != "" || filterType != "" || filterStatus != "" || terminal
			if structuredFormat(cmd) {
				rows := output.ListRows(index, ids)
				var groups []listGroup
				if group {
					groups = output.ListGroupsByStatus(index, ids)
				}
				return output.WriteListEnvelope(cmd.OutOrStdout(), rows, groups, group, filtered)
			}

			if group {
				groups := make(map[string][]string)
				for _, id := range ids {
					s := index[id].Status
					groups[s] = append(groups[s], id)
				}
				statuses := make([]string, 0, len(groups))
				for s := range groups {
					statuses = append(statuses, s)
				}
				sort.Slice(statuses, func(i, j int) bool {
					return output.ListStatusRank(statuses[i]) < output.ListStatusRank(statuses[j])
				})
				for _, status := range statuses {
					label := status
					if status == ops.StatusDone {
						label = "done (awaiting merge)"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", label)
					sort.Strings(groups[status])
					for _, id := range groups[status] {
						e := index[id]
						line := fmt.Sprintf("  %-12s  %s", id, e.Title)
						if status == ops.StatusDone && e.Branch != "" {
							line += fmt.Sprintf("  [branch: %s", e.Branch)
							if e.PR != "" {
								line += fmt.Sprintf(", PR: #%s", e.PR)
							}
							line += "]"
						}
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
					}
				}
				return nil
			}

			if filterParent != "" {
				if len(ids) == 0 {
					return nil
				}
				// Story Board view: migrate to output.RenderBoard for a single table renderer
				boardEntries := make([]output.BoardEntry, 0, len(ids))
				for _, id := range ids {
					e := index[id]
					claimed := ""
					issue := snap.State.Issues[id]
					if issue != nil {
						claimed = issue.ClaimedBy
					}
					boardEntries = append(boardEntries, output.BoardEntry{
						Issue:   id,
						Status:  e.Status,
						Claimed: claimed,
						Outcome: e.Outcome,
						Title:   e.Title,
					})
				}
				return output.RenderBoard(cmd.OutOrStdout(), boardEntries)
			}

			// Use output.RenderList for simple list view
			entries := make([]output.ListEntry, 0, len(ids))
			for _, id := range ids {
				e := index[id]
				le := output.ListEntry{
					Issue:      id,
					Status:     e.Status,
					Title:      e.Title,
					AssignedTo: e.AssignedWorker,
				}
				entries = append(entries, le)
			}
			return output.RenderList(cmd.OutOrStdout(), entries)
		},
	}

	cmd.Flags().StringVar(&filterParent, "parent", "", "filter by parent issue ID")
	cmd.Flags().StringVar(&filterType, "type", "", "filter by issue type (task, story, feature, bug)")
	cmd.Flags().StringVar(&filterStatus, "status", "", "filter by status (open, in-progress, done, merged, cancelled, blocked)")
	cmd.Flags().BoolVar(&group, "group", false, "group issues by status (human headers; structured groups adjunct)")
	cmd.Flags().BoolVar(&terminal, "terminal", false, "filter to all terminal statuses (done, merged, cancelled)")

	return cmd
}
