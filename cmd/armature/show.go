package main

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/stats"
	"github.com/spf13/cobra"
)

const (
	showLargeFieldLimit = 512
	showFieldHelp       = "arm show <id> --field <name> extracts a scalar value, never an envelope"
)

// showTruncation is the truncated adjunct: large text fields stay present,
// with shown and total byte counts so agents can request --full.
type showTruncation struct {
	Field      string `json:"field"`
	ShownBytes int    `json:"shown_bytes"`
	TotalBytes int    `json:"total_bytes"`
}

func truncateShowText(s string, limit int) (string, int, bool) {
	total := len(s)
	if total <= limit {
		return s, total, false
	}
	shown := s
	for len(shown) > limit {
		_, size := utf8.DecodeLastRuneInString(shown)
		if size <= 0 {
			break
		}
		shown = shown[:len(shown)-size]
	}
	return shown, total, true
}

func truncateShowIssue(row *output.IssueJSON) []showTruncation {
	var hints []showTruncation
	if shown, total, truncated := truncateShowText(row.Outcome, showLargeFieldLimit); truncated {
		row.Outcome = shown
		hints = append(hints, showTruncation{Field: "outcome", ShownBytes: len(shown), TotalBytes: total})
	}
	if shown, total, truncated := truncateShowText(row.DefinitionOfDone, showLargeFieldLimit); truncated {
		row.DefinitionOfDone = shown
		hints = append(hints, showTruncation{Field: "definition_of_done", ShownBytes: len(shown), TotalBytes: total})
	}
	return hints
}

func showHelp(ids []string, trunc []showTruncation) []string {
	if len(trunc) == 0 {
		return []string{showFieldHelp}
	}
	id := "<id>"
	if len(ids) == 1 {
		id = ids[0]
	}
	t := trunc[0]
	line := fmt.Sprintf("%s truncated (%d of %d bytes); arm show %s --full for complete fields",
		t.Field, t.ShownBytes, t.TotalBytes, id)
	return []string{line, showFieldHelp}
}

func writeShowEnvelope(w io.Writer, ids []string, rows []output.IssueJSON, trunc []showTruncation) error {
	env, err := output.NewEnvelope("issues", rows, showHelp(ids, trunc))
	if err != nil {
		return err
	}
	if len(trunc) > 0 {
		if err := env.AddAdjunct("truncated", trunc); err != nil {
			return err
		}
	}
	return output.WriteEnvelope(w, env)
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
