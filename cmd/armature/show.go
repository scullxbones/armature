package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	var issueID string
	var fieldFlag string

	cmd := &cobra.Command{
		Use:   "show [issue-id ...]",
		Short: "Show a human-readable summary of one or more issues",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Collect all IDs: --issue flag plus all positional args
			var ids []string
			if issueID != "" {
				ids = append(ids, issueID)
			}
			ids = append(ids, args...)
			if len(ids) == 0 {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			ctx := currentCtx(cmd)
			issuesDir := ctx.IssuesDir
			singleBranch := ctx.Mode == "single-branch"

			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir, singleBranch)
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			// Multi-issue JSON: emit a JSON array
			if format == "json" && len(ids) > 1 {
				type showJSON struct {
					ID         string          `json:"id"`
					Title      string          `json:"title"`
					Type       string          `json:"type"`
					Status     string          `json:"status"`
					Parent     string          `json:"parent,omitempty"`
					ClaimedBy  string          `json:"claimed_by,omitempty"`
					DoD        string          `json:"definition_of_done,omitempty"`
					Acceptance json.RawMessage `json:"acceptance,omitempty"`
					Scope      []string        `json:"scope,omitempty"`
					Notes      []string        `json:"notes,omitempty"`
					Outcome    string          `json:"outcome,omitempty"`
					AssignedTo string          `json:"assigned_worker,omitempty"`
					BlockedBy  []string        `json:"blocked_by,omitempty"`
					Blocks     []string        `json:"blocks,omitempty"`
				}
				results := make([]showJSON, 0, len(ids))
				for _, id := range ids {
					issuePtr, ok := snap.Issues[id]
					if !ok || issuePtr == nil {
						return fmt.Errorf("issue %q not found", id)
					}
					issue := *issuePtr
					noteTexts := make([]string, 0, len(issue.Notes))
					for _, n := range issue.Notes {
						if n.Deleted {
							continue
						}
						noteTexts = append(noteTexts, n.Msg)
					}
					results = append(results, showJSON{
						ID:         issue.ID,
						Title:      issue.Title,
						Type:       issue.Type,
						Status:     issue.Status,
						Parent:     issue.Parent,
						ClaimedBy:  issue.ClaimedBy,
						DoD:        issue.DefinitionOfDone,
						Acceptance: issue.Acceptance,
						Scope:      issue.Scope,
						Notes:      noteTexts,
						Outcome:    issue.Outcome,
						AssignedTo: issue.AssignedWorker,
						BlockedBy:  issue.BlockedBy,
						Blocks:     issue.Blocks,
					})
				}
				data, _ := json.MarshalIndent(results, "", "  ") //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			// Single or multi-issue non-JSON: iterate and print each, separated by "---"
			for i, id := range ids {
				issuePtr, ok := snap.Issues[id]
				if !ok || issuePtr == nil {
					return fmt.Errorf("issue %q not found", id)
				}
				issue := *issuePtr

				if i > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "---")
				}

				// If --field flag is set, extract and print only the requested fields
				if fieldFlag != "" {
					fields := extractFieldsFromIssue(&issue, fieldFlag)
					for _, field := range fields {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), field)
					}
					continue
				}

				if format == "json" {
					type showJSON struct {
						ID         string          `json:"id"`
						Title      string          `json:"title"`
						Type       string          `json:"type"`
						Status     string          `json:"status"`
						Parent     string          `json:"parent,omitempty"`
						ClaimedBy  string          `json:"claimed_by,omitempty"`
						DoD        string          `json:"definition_of_done,omitempty"`
						Acceptance json.RawMessage `json:"acceptance,omitempty"`
						Scope      []string        `json:"scope,omitempty"`
						Notes      []string        `json:"notes,omitempty"`
						Outcome    string          `json:"outcome,omitempty"`
						AssignedTo string          `json:"assigned_worker,omitempty"`
						BlockedBy  []string        `json:"blocked_by,omitempty"`
						Blocks     []string        `json:"blocks,omitempty"`
					}
					noteTexts := make([]string, 0, len(issue.Notes))
					for _, n := range issue.Notes {
						if n.Deleted {
							continue
						}
						noteTexts = append(noteTexts, n.Msg)
					}
					out := showJSON{
						ID:         issue.ID,
						Title:      issue.Title,
						Type:       issue.Type,
						Status:     issue.Status,
						Parent:     issue.Parent,
						ClaimedBy:  issue.ClaimedBy,
						DoD:        issue.DefinitionOfDone,
						Acceptance: issue.Acceptance,
						Scope:      issue.Scope,
						Notes:      noteTexts,
						Outcome:    issue.Outcome,
						AssignedTo: issue.AssignedWorker,
						BlockedBy:  issue.BlockedBy,
						Blocks:     issue.Blocks,
					}
					data, _ := json.MarshalIndent(out, "", "  ") //nolint:errcheck // result struct contains only serializable values
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
					continue
				}

				// Use output.RenderIssue for human-readable output
				if err := output.RenderIssue(cmd.OutOrStdout(), &issue, false); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to show")
	cmd.Flags().StringVar(&fieldFlag, "field", "", "comma-separated list of fields to extract (e.g., status or status,outcome,title)")

	return cmd
}
