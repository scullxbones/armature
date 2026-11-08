package main

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

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

			if group {
				for _, g := range output.ListGroupsByStatus(listFacts, ids) {
					label := g.Status
					if g.Status == ops.StatusDone {
						label = "done (awaiting merge)"
					}
					if _, err := fmt.Fprintf(out, "\n=== %s ===\n", label); err != nil {
						return err
					}
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
						if _, err := fmt.Fprintln(out, line); err != nil {
							return err
						}
						if err := writeHostRefLines(out, snap.State.Issues[id]); err != nil {
							return err
						}
					}
				}
				return nil
			}

			if filterParent != "" {
				if len(ids) == 0 {
					return nil
				}
				if _, err := fmt.Fprintf(out, "%-12s %-12s %-38s %-30s %s\n", "ID", "STATUS", "CLAIMED", "OUTCOME", "TITLE"); err != nil {
					return err
				}
				for _, id := range ids {
					e := index[id]
					claimed := ""
					issue := snap.State.Issues[id]
					if issue != nil {
						claimed = issue.ClaimedBy
					}
					outcome := e.Outcome
					const maxOutcome = 30
					if len(outcome) > maxOutcome {
						outcome = outcome[:27] + "..."
					}
					if _, err := fmt.Fprintf(out, "%-12s %-12s %-38s %-30s %s\n", id, e.Status, claimed, outcome, e.Title); err != nil {
						return err
					}
					if err := writeHostRefLines(out, issue); err != nil {
						return err
					}
				}
				return nil
			}

			for _, id := range ids {
				e := index[id]
				status := e.Status
				if e.AssignedWorker != "" {
					status = status + " (assigned to " + e.AssignedWorker + ")"
				}
				if _, err := fmt.Fprintf(out, "  %-12s  %-14s  %s\n", id, status, e.Title); err != nil {
					return err
				}
				if err := writeHostRefLines(out, snap.State.Issues[id]); err != nil {
					return err
				}
			}
			return nil
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

func writeHostRefLines(w io.Writer, issue *materialize.Issue) error {
	if issue == nil || len(issue.HostRefs) == 0 {
		return nil
	}
	for _, line := range formatHostRefLines(issue.HostRefs) {
		if _, err := fmt.Fprintf(w, "    %s\n", line); err != nil {
			return err
		}
	}
	return nil
}

func formatHostRefLines(refs []materialize.HostRef) []string {
	current, hasCurrent := materialize.CurrentHostRef(refs)
	live := map[string]materialize.HostRef{}
	order := []string{}
	for _, ref := range refs {
		if ref.Withdrawn {
			delete(live, ref.URL)
			order = dropHostURL(order, ref.URL)
			continue
		}
		order = dropHostURL(order, ref.URL)
		order = append(order, ref.URL)
		live[ref.URL] = ref
	}
	lines := make([]string, 0, len(order))
	for _, url := range order {
		ref := live[url]
		parts := []string{url}
		if ref.Role != "" {
			parts = append(parts, "role="+ref.Role)
		}
		if hasCurrent && url == current.URL {
			parts = append(parts, "current")
		}
		lines = append(lines, strings.Join(parts, "  "))
	}
	return lines
}
