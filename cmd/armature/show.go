package main

import (
	"fmt"
	"io"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/stats"
	"github.com/spf13/cobra"
)

const showLargeFieldLimit = output.ShowLargeFieldLimit

type showTruncation = output.ShowTruncation

func truncateShowIssue(row *output.IssueJSON) []showTruncation {
	return output.TruncateShowIssue(row)
}

func writeShowEnvelope(w io.Writer, ids []string, rows []output.IssueJSON, trunc []showTruncation) error {
	return output.WriteShowEnvelope(w, ids, rows, trunc)
}

func newShowCmd() *cobra.Command {
	var issueID string
	var fieldFlag string
	var full bool

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
			structured := format == "json" || format == "agent"

			for _, id := range ids {
				issuePtr, ok := snap.Issues[id]
				if !ok || issuePtr == nil {
					return fmt.Errorf("issue %q not found", id)
				}
			}

			if fieldFlag != "" {
				for _, id := range ids {
					issue := *snap.Issues[id]
					fields := extractFieldsFromIssue(&issue, fieldFlag)
					for _, field := range fields {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), field)
					}
				}
				return nil
			}

			if structured {
				rows := make([]output.IssueJSON, 0, len(ids))
				var trunc []showTruncation
				for _, id := range ids {
					row := output.MarshalIssue(snap.Issues[id])
					if !full {
						trunc = append(trunc, truncateShowIssue(&row)...)
					}
					rows = append(rows, row)
				}
				return writeShowEnvelope(cmd.OutOrStdout(), ids, rows, trunc)
			}

			needSpend := true
			var (
				costReport *stats.Report
				issueInfo  map[string]stats.IssueInfo
			)
			if needSpend {
				var costErr error
				costReport, issueInfo, costErr = loadSpendReport(ctx, snap)
				if costErr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: spend-to-date unavailable: %s\n", costErr)
				}
			}

			for i, id := range ids {
				issue := *snap.Issues[id]

				if i > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "---")
				}

				if err := output.RenderIssue(cmd.OutOrStdout(), &issue, false); err != nil {
					return err
				}
				if costReport != nil {
					spend := stats.Rollup(*costReport, id, issueInfo)
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), stats.FormatSpend(spend))
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to show")
	cmd.Flags().StringVar(&fieldFlag, "field", "", "comma-separated list of fields to extract (e.g., status or status,outcome,title)")
	cmd.Flags().BoolVar(&full, "full", false, "return complete large text fields in structured output (default truncates with total size)")

	return cmd
}

func loadSpendReport(ctx *config.Context, snap *snapshot.Snapshot) (*stats.Report, map[string]stats.IssueInfo, error) {
	rates, err := stats.ResolveRates("", ctx.IssuesDir)
	if err != nil {
		return nil, nil, err
	}
	issues := snapshotIssueInfo(snap)
	report := stats.Estimate(stats.CollectUsage(snap.Ops), issues, rates)
	return &report, issues, nil
}
