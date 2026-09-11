package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/stats"
	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	var cost bool
	var ratesPath string

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Derived metrics from the ops log",
		Long: `Derived metrics from the ops log.

The --cost view (G1.2) sums recorded token counts into per-story and per-wave
dollar estimates. Counts come from optional input_tokens/output_tokens on
transition outcome payloads and assessment attestations. Zero or omitted is
unset and is not billed.

Dollars use USD per million tokens. Built-in rates (override with --rates or
.armature/cost-rates.json):

  default / claude-sonnet-4-5  $3.00 in / $15.00 out
  claude-haiku-4-5              $1.00 in / $5.00 out
  claude-opus-4-5               $15.00 in / $75.00 out
  gpt-4.1                       $2.00 in / $8.00 out
  gpt-4.1-mini                  $0.40 in / $1.60 out

A --rates file is JSON: {"models":{"name":{"input_usd_per_mtok":3,"output_usd_per_mtok":15}}}.
Unlisted models keep the built-in table. Outcome records resolve model as
payload preferred_model, then the issue preferred_model, then default.
Assessment records use model_identity or default; they do not inherit the
issue preferred_model.

Waves are greedy first-fit groups of issues that recorded usage and do not
share overlapping scope (same idea as arm ready --waves). Stories roll up
descendant usage by walking parent links to the nearest story. Spend uses
the same captured validated op set as the snapshot. Idempotent assessment
attestations (same issue and result_fingerprint) are counted once.

Latency and rework analytics from the long-horizon stats proposal are not
implemented here. Pass --cost for the spend view.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cost {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(),
					"arm stats: pass --cost for per-story and per-wave dollar estimates from recorded tokens.")
				return nil
			}
			return runStatsCost(cmd, ratesPath)
		},
	}

	cmd.Flags().BoolVar(&cost, "cost", false, "aggregate recorded token counts into dollar estimates")
	cmd.Flags().StringVar(&ratesPath, "rates", "", "JSON rate table path (default: .armature/cost-rates.json or built-in rates)")

	return cmd
}

func runStatsCost(cmd *cobra.Command, ratesPath string) error {
	ctx := currentCtx(cmd)
	store := newSnapshotStore(ctx)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	for _, w := range snap.Warnings {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}

	rates, err := stats.ResolveRates(ratesPath, ctx.IssuesDir)
	if err != nil {
		return err
	}

	issues := snapshotIssueInfo(snap)
	report := stats.Estimate(stats.CollectUsage(snap.Ops), issues, rates)

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" || format == "agent" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal cost report: %w", err)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	printCostHuman(cmd, report)
	return nil
}

func printCostHuman(cmd *cobra.Command, report stats.Report) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "Cost estimates (USD, from recorded tokens)")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Stories:")
	if len(report.Stories) == 0 {
		_, _ = fmt.Fprintln(out, "  (none)")
	}
	for _, s := range report.Stories {
		_, _ = fmt.Fprintf(out, "  %-24s  %s  (%d in / %d out)\n",
			s.ID, stats.FormatUSD(s.USD), s.InputTokens, s.OutputTokens)
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Waves:")
	if len(report.Waves) == 0 {
		_, _ = fmt.Fprintln(out, "  (none)")
	}
	for _, w := range report.Waves {
		_, _ = fmt.Fprintf(out, "  %-10s  %s  %s  (%d in / %d out)\n",
			w.ID, strings.Join(w.Issues, ", "), stats.FormatUSD(w.USD), w.InputTokens, w.OutputTokens)
	}
}

func snapshotIssueInfo(snap *snapshot.Snapshot) map[string]stats.IssueInfo {
	out := make(map[string]stats.IssueInfo, len(snap.Issues))
	if snap == nil {
		return out
	}
	for id, issue := range snap.Issues {
		if issue == nil {
			continue
		}
		out[id] = issueInfoFrom(issue)
	}
	return out
}

func issueInfoFrom(issue *materialize.Issue) stats.IssueInfo {
	return stats.IssueInfo{
		ID:             issue.ID,
		Type:           issue.Type,
		Parent:         issue.Parent,
		PreferredModel: issue.PreferredModel,
		Scope:          issue.Scope,
	}
}
