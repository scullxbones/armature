package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	var issueID string
	var fieldFlag string

	cmd := &cobra.Command{
		Use:   "show [issue-id]",
		Short: "Show a human-readable summary of a single issue",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, singleBranch); err != nil {
				return err
			}

			issuePath := filepath.Join(appCtx.StateDir, "issues", issueID+".json")
			issue, err := materialize.LoadIssue(issuePath)
			if err != nil {
				return fmt.Errorf("issue %q not found", issueID)
			}

			// If --field flag is set, extract and print only the requested fields
			if fieldFlag != "" {
				fields := extractFieldsFromIssue(&issue, fieldFlag)
				for _, field := range fields {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), field)
				}
				return nil
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
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
				}
				noteTexts := make([]string, 0, len(issue.Notes))
				for _, n := range issue.Notes {
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
				}
				data, _ := json.MarshalIndent(out, "", "  ")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			// Human-readable output
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "ID:        %s\n", issue.ID)
			_, _ = fmt.Fprintf(w, "Title:     %s\n", issue.Title)
			_, _ = fmt.Fprintf(w, "Type:      %s\n", issue.Type)
			_, _ = fmt.Fprintf(w, "Status:    %s\n", issue.Status)
			if issue.Parent != "" {
				_, _ = fmt.Fprintf(w, "Parent:    %s\n", issue.Parent)
			}
			if issue.ClaimedBy != "" {
				_, _ = fmt.Fprintf(w, "ClaimedBy: %s\n", issue.ClaimedBy)
			}
			if issue.AssignedWorker != "" {
				_, _ = fmt.Fprintf(w, "Assigned:  %s\n", issue.AssignedWorker)
			}
			if issue.DefinitionOfDone != "" {
				_, _ = fmt.Fprintf(w, "DoD:       %s\n", issue.DefinitionOfDone)
			}
			if len(issue.Acceptance) > 0 && string(issue.Acceptance) != "null" {
				compact, err := json.Marshal(issue.Acceptance)
				if err == nil {
					_, _ = fmt.Fprintf(w, "Acceptance: %s\n", string(compact))
				}
			}
			if len(issue.Scope) > 0 {
				_, _ = fmt.Fprintf(w, "Scope:     %s\n", strings.Join(issue.Scope, ", "))
			}
			if issue.Outcome != "" {
				_, _ = fmt.Fprintf(w, "Outcome:   %s\n", issue.Outcome)
			}
			if len(issue.Notes) > 0 {
				_, _ = fmt.Fprintf(w, "Notes:\n")
				for _, n := range issue.Notes {
					_, _ = fmt.Fprintf(w, "  - %s\n", n.Msg)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to show")
	cmd.Flags().StringVar(&fieldFlag, "field", "", "comma-separated list of fields to extract (e.g., status or status,outcome,title)")

	return cmd
}
