package main

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

// statusOrder defines display priority — lower number appears first.
var statusOrder = map[string]int{
	ops.StatusInProgress: 0,
	ops.StatusClaimed:    1,
	ops.StatusDone:       2, // "awaiting merge" in dual-branch mode
	ops.StatusOpen:       3,
	ops.StatusBlocked:    4,
	ops.StatusMerged:     5,
	ops.StatusCancelled:  6,
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show issues grouped by status",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			if _, err := materialize.Materialize(issuesDir, singleBranch); err != nil {
				return err
			}

			index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
			if err != nil {
				return err
			}

			// Group by status
			groups := make(map[string][]string)
			for id, entry := range index {
				groups[entry.Status] = append(groups[entry.Status], id)
			}

			// Sort statuses by display priority
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
				ids := groups[status]
				sort.Strings(ids)

				label := status
				if status == ops.StatusDone && !singleBranch {
					label = "done (awaiting merge)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", label)

				for _, id := range ids {
					entry := index[id]
					line := fmt.Sprintf("  %-12s  %s", id, entry.Title)
					if status == ops.StatusDone && !singleBranch && entry.Branch != "" {
						line += fmt.Sprintf("  [branch: %s", entry.Branch)
						if entry.PR != "" {
							line += fmt.Sprintf(", PR: #%s", entry.PR)
						}
						line += "]"
					}
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
			}

			return nil
		},
	}
	return cmd
}
