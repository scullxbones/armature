package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
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

func newListCmd() *cobra.Command {
	var filterParent string
	var filterType string
	var filterStatus string
	var group bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues with optional --type, --parent, and --status filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"
			if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, singleBranch); err != nil {
				return err
			}

			index, err := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
			if err != nil {
				return err
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
					issue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", id+".json"))
					if err == nil {
						le.ClaimedBy = issue.ClaimedBy
					}
					entries = append(entries, le)
				}
				data, _ := json.MarshalIndent(entries, "", "  ")
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
					if status == ops.StatusDone && appCtx.Mode != "single-branch" {
						label = "done (awaiting merge)"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", label)
					sort.Strings(groups[status])
					for _, id := range groups[status] {
						e := index[id]
						line := fmt.Sprintf("  %-12s  %s", id, e.Title)
						if status == ops.StatusDone && appCtx.Mode != "single-branch" && e.Branch != "" {
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
				// Story Board view (table format)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-12s %-38s %-30s %s\n", "ID", "STATUS", "CLAIMED", "OUTCOME", "TITLE")
				for _, id := range ids {
					e := index[id]
					claimed := ""
					issue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", id+".json"))
					if err == nil {
						claimed = issue.ClaimedBy
					}
					outcome := e.Outcome
					if len(outcome) > 30 {
						outcome = outcome[:27] + "..."
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-12s %-38s %-30s %s\n", id, e.Status, claimed, outcome, e.Title)
				}
				return nil
			}

			for _, id := range ids {
				entry := index[id]
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-12s  %-14s  %s\n", id, entry.Status, entry.Title)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&filterParent, "parent", "", "filter by parent issue ID")
	cmd.Flags().StringVar(&filterType, "type", "", "filter by issue type (task, story, feature, bug)")
	cmd.Flags().StringVar(&filterStatus, "status", "", "filter by status (open, in-progress, done, merged, cancelled, blocked)")
	cmd.Flags().BoolVar(&group, "group", false, "group issues by status with section headers (human format only)")

	return cmd
}
