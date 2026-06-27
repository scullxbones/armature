package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
)

// statusOrder defines display priority for --group output — lower number appears first.
var statusOrder = map[string]int{
	ops.StatusInProgress: 0,
	ops.StatusClaimed:    1,
	ops.StatusDone:       2,
	ops.StatusOpen:       3,
	ops.StatusBlocked:    4,
	ops.StatusMerged:     5,
	ops.StatusCancelled:  6,
}

type listEntry struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Parent    string `json:"parent,omitempty"`
	Title     string `json:"title"`
	Outcome   string `json:"outcome,omitempty"`
	ClaimedBy string `json:"claimed_by,omitempty"`
}

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

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				entries := make([]listEntry, 0, len(ids))
				for _, id := range ids {
					e := index[id]
					le := listEntry{
						ID:      id,
						Type:    e.Type,
						Status:  e.Status,
						Parent:  e.Parent,
						Title:   e.Title,
						Outcome: e.Outcome,
					}
					issue := snap.State.Issues[id]
					if issue != nil {
						le.ClaimedBy = issue.ClaimedBy
					}
					entries = append(entries, le)
				}
				data, _ := json.MarshalIndent(entries, "", "  ") //nolint:errcheck // slice of serializable structs
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
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
					oi, ok1 := statusOrder[statuses[i]]
					oj, ok2 := statusOrder[statuses[j]]
					if !ok1 {
						oi = 99
					}
					if !ok2 {
						oj = 99
					}
					return oi < oj
				})
				for _, status := range statuses {
					label := status
					if status == ops.StatusDone && ctx.Mode != "single-branch" {
						label = "done (awaiting merge)"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", label)
					sort.Strings(groups[status])
					for _, id := range groups[status] {
						e := index[id]
						line := fmt.Sprintf("  %-12s  %s", id, e.Title)
						if status == ops.StatusDone && ctx.Mode != "single-branch" && e.Branch != "" {
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
	cmd.Flags().BoolVar(&group, "group", false, "group issues by status with section headers (human format only)")
	cmd.Flags().BoolVar(&terminal, "terminal", false, "filter to all terminal statuses (done, merged, cancelled)")

	return cmd
}
