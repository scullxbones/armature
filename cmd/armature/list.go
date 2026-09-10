package main

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
)

const listShowHelp = "arm show <id> for outcome, scope, and acceptance"

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

// listEntry is the N4 default list row: id, type, status, title only.
type listEntry struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

// listGroup is the structured --group adjunct: status buckets in workflow order.
// Issue rows stay in the issues payload (N2); groups only name ids.
type listGroup struct {
	Status string   `json:"status"`
	IDs    []string `json:"ids"`
}

// terminalStatuses is the set of statuses that represent terminal (completed) states.
var terminalStatuses = map[string]bool{
	ops.StatusDone:      true,
	ops.StatusMerged:    true,
	ops.StatusCancelled: true,
}

func statusRank(status string) int {
	if n, ok := statusOrder[status]; ok {
		return n
	}
	return 99
}

func listRows(index materialize.Index, ids []string) []listEntry {
	rows := make([]listEntry, 0, len(ids))
	for _, id := range ids {
		e := index[id]
		rows = append(rows, listEntry{
			ID:     id,
			Type:   e.Type,
			Status: e.Status,
			Title:  e.Title,
		})
	}
	return rows
}

func listGroupsByStatus(index materialize.Index, ids []string) []listGroup {
	buckets := make(map[string][]string)
	for _, id := range ids {
		s := index[id].Status
		buckets[s] = append(buckets[s], id)
	}
	statuses := make([]string, 0, len(buckets))
	for s := range buckets {
		statuses = append(statuses, s)
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statusRank(statuses[i]) < statusRank(statuses[j])
	})
	groups := make([]listGroup, 0, len(statuses))
	for _, status := range statuses {
		members := buckets[status]
		sort.Strings(members)
		groups = append(groups, listGroup{Status: status, IDs: members})
	}
	return groups
}

func listHelp(filtered bool, n int) []string {
	if n == 0 {
		reason := "no issues in the repository"
		if filtered {
			reason = "no issues match the filter"
		}
		return []string{reason, listShowHelp}
	}
	return []string{listShowHelp}
}

func writeListEnvelope(w io.Writer, rows []listEntry, groups []listGroup, grouped, filtered bool) error {
	env, err := output.NewEnvelope("issues", rows, listHelp(filtered, len(rows)))
	if err != nil {
		return err
	}
	if grouped {
		if err := env.AddAdjunct("groups", groups); err != nil {
			return err
		}
	}
	return output.WriteEnvelope(w, env)
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
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				rows := listRows(index, ids)
				var groups []listGroup
				if group {
					groups = listGroupsByStatus(index, ids)
				}
				return writeListEnvelope(cmd.OutOrStdout(), rows, groups, group, filtered)
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
					return statusRank(statuses[i]) < statusRank(statuses[j])
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
