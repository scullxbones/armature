package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/output"
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

			store := newSnapshotStore(ctx)
			snap, err := store.Load(cmd.Context())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			// Multi-issue JSON: emit a JSON array using the canonical output.IssueJSON schema
			if format == "json" && len(ids) > 1 {
				results := make([]output.IssueJSON, 0, len(ids))
				for _, id := range ids {
					issuePtr, ok := snap.Issues[id]
					if !ok || issuePtr == nil {
						return fmt.Errorf("issue %q not found", id)
					}
					results = append(results, output.MarshalIssue(issuePtr))
				}
				data, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal issues JSON: %w", err)
				}
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
					// Route single-issue JSON through the canonical output package schema
					if err := output.RenderIssue(cmd.OutOrStdout(), &issue, true); err != nil {
						return err
					}
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
