package main

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/scullxbones/armature/internal/externalref"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var filterParent string
	var filterType string
	var filterStatus string
	var filterExternalRef string
	var group bool
	var terminal bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues with optional --type, --parent, --status, and --external-ref filters",
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

			var wantHostURL string
			if filterExternalRef != "" {
				gh, parseErr := externalref.ParseGitHubIssueURL(filterExternalRef)
				if parseErr != nil {
					return fmt.Errorf("parse --external-ref: %w", parseErr)
				}
				wantHostURL = gh.URL()
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
				if terminal && !ops.IsTerminalStatus(entry.Status) {
					continue
				}
				if wantHostURL != "" && !slices.Contains(entry.LiveHostURLs, wantHostURL) {
					continue
				}
				ids = append(ids, id)
			}
			sort.Strings(ids)

			filtered := filterParent != "" || filterType != "" || filterStatus != "" || terminal || wantHostURL != ""
			listFacts := outputListFacts(index)
			out := cmd.OutOrStdout()
			if structuredFormat(cmd) {
				rows := output.ListRows(listFacts, ids)
				var groups []output.ListGroup
				if group {
					groups = output.ListGroupsByStatus(listFacts, ids)
				}
				return output.WriteListEnvelope(out, rows, groups, group, filtered)
			}

			return output.RenderList(out, listHumanRows(index, snap.State.Issues, ids), group, filterParent != "")
		},
	}

	cmd.Flags().StringVar(&filterParent, "parent", "", "filter by parent issue ID")
	cmd.Flags().StringVar(&filterType, "type", "", "filter by issue type (task, story, feature, bug)")
	cmd.Flags().StringVar(&filterStatus, "status", "", "filter by status (open, in-progress, done, merged, cancelled, blocked)")
	cmd.Flags().StringVar(&filterExternalRef, "external-ref", "", "filter by live GitHub issue URL (parsed canonical form)")
	cmd.Flags().BoolVar(&group, "group", false, "group issues by status (human headers; structured groups adjunct)")
	cmd.Flags().BoolVar(&terminal, "terminal", false, "filter to all terminal statuses (done, merged, cancelled)")

	return cmd
}

func listHumanRows(index materialize.Index, issues map[string]*materialize.Issue, ids []string) []output.IssueFacts {
	rows := make([]output.IssueFacts, 0, len(ids))
	for _, id := range ids {
		e := index[id]
		row := output.IssueFacts{
			ID:             id,
			Status:         e.Status,
			Title:          e.Title,
			AssignedWorker: e.AssignedWorker,
			Outcome:        e.Outcome,
			Branch:         e.Branch,
			PR:             e.PR,
		}
		if issue := issues[id]; issue != nil {
			row.ClaimedBy = issue.ClaimedBy
			row.HostRefs = hostRefFacts(issue.HostRefs)
		}
		rows = append(rows, row)
	}
	return rows
}
