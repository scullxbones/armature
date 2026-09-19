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
				if terminal && !isTerminalStatus(entry.Status) {
					continue
				}
				ids = append(ids, id)
			}
			sort.Strings(ids)

			filtered := filterParent != "" || filterType != "" || filterStatus != "" || terminal
			if structuredFormat(cmd) {
				rows := output.ListRows(index, ids)
				var groups []output.ListGroup
				if group {
					groups = output.ListGroupsByStatus(index, ids)
				}
				return output.WriteListEnvelope(cmd.OutOrStdout(), rows, groups, group, filtered)
			}

			if group {
				for _, g := range output.ListGroupsByStatus(index, ids) {
					label := g.Status
					if g.Status == ops.StatusDone {
						label = "done (awaiting merge)"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", label)
					for _, id := range g.IDs {
						e := index[id]
						line := fmt.Sprintf("  %-12s  %s", id, e.Title)
						if g.Status == ops.StatusDone && e.Branch != "" {
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
