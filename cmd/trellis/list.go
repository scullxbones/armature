package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/spf13/cobra"
)

type listEntry struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Parent string `json:"parent,omitempty"`
	Title  string `json:"title"`
}

func newListCmd() *cobra.Command {
	var filterParent string
	var filterType string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues with optional --type and --parent filters",
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
				ids = append(ids, id)
			}
			sort.Strings(ids)

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" {
				entries := make([]listEntry, 0, len(ids))
				for _, id := range ids {
					e := index[id]
					entries = append(entries, listEntry{
						ID:     id,
						Type:   e.Type,
						Status: e.Status,
						Parent: e.Parent,
						Title:  e.Title,
					})
				}
				data, _ := json.MarshalIndent(entries, "", "  ")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			for _, id := range ids {
				entry := index[id]
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-12s  %s\n", id, entry.Title)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&filterParent, "parent", "", "filter by parent issue ID")
	cmd.Flags().StringVar(&filterType, "type", "", "filter by issue type (task, story, feature, bug)")

	return cmd
}
