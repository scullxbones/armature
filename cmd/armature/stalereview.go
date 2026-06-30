package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/tui/stalereview"
	"github.com/spf13/cobra"
)

func newStaleReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stale-review",
		Short: "Review sources whose cached content has changed since last sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			manifest, err := sources.ReadManifest(sourcesDir())
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

			state, _, err := materialize.MaterializeAndReturn(issuesDir, appCtx.StateDir, true)
			if err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			// Detect stale entries.
			var reviewItems []stalereview.ReviewItem
			for _, entry := range manifest.Entries {
				data, err := sources.ReadCache(sourcesDir(), entry.ID)
				if err != nil {
					return fmt.Errorf("read cache for %s: %w", entry.ID, err)
				}
				currentFP := sources.Fingerprint(data)
				if data == nil || currentFP != entry.Fingerprint {
					// Find cited issues.
					var cited []*materialize.Issue
					for _, issue := range state.Issues {
						if len(issue.SourceLinks) == 0 {
							continue
						}
						for _, link := range issue.SourceLinks {
							if link.SourceEntryID == entry.ID {
								cited = append(cited, issue)
								break
							}
						}
					}
					sort.Slice(cited, func(i, j int) bool {
						return cited[i].ID < cited[j].ID
					})

					summary := fmt.Sprintf("fingerprint changed (stored: %s, current: %s)",
						entry.Fingerprint, currentFP)
					if data == nil {
						summary = "no cache found"
					}
					reviewItems = append(reviewItems, stalereview.ReviewItem{
						SourceID:      entry.ID,
						ChangeSummary: summary,
						CitedIssues:   cited,
					})
				}
			}

			sort.Slice(reviewItems, func(i, j int) bool {
				return reviewItems[i].SourceID < reviewItems[j].SourceID
			})

			if len(reviewItems) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No stale sources detected.")
				return nil
			}

			format, _ := cmd.Flags().GetString("format")
			if format == "json" || format == "agent" || tui.IsNonInteractive() {
				type staleSource struct {
					SourceID      string   `json:"source_id"`
					ChangeSummary string   `json:"change_summary"`
					CitedIssues   []string `json:"cited_issues"`
				}
				var staleSources []staleSource
				for _, item := range reviewItems {
					var ids []string
					for _, issue := range item.CitedIssues {
						ids = append(ids, issue.ID)
					}
					staleSources = append(staleSources, staleSource{
						SourceID:      item.SourceID,
						ChangeSummary: item.ChangeSummary,
						CitedIssues:   ids,
					})
				}
				data, _ := json.MarshalIndent(map[string]interface{}{
					"stale_sources": staleSources,
					"count":         len(staleSources),
				}, "", "  ")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			if !tui.IsTerminal() {
				// Human-readable summary for non-TTY (format == "human")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Stale Sources:")
				for _, item := range reviewItems {
					var ids []string
					for _, issue := range item.CitedIssues {
						ids = append(ids, issue.ID)
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s) cites: %s\n", item.SourceID, item.ChangeSummary, strings.Join(ids, ", "))
				}
				return nil
			}

			m := stalereview.New(reviewItems, workerID)
			p := tea.NewProgram(m)
			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("stale-review TUI: %w", err)
			}
			final := finalModel.(stalereview.Model)

			decisions := final.Decisions()
			items := final.Items()
			for i, item := range items {
				var decision string
				switch decisions[i] {
				case 1: // decisionConfirmed
					decision = "confirmed"
				case 2: // decisionFlagged
					decision = "flagged"
				default:
					continue
				}
				for _, issue := range item.CitedIssues {
					noteMsg := fmt.Sprintf("stale-review: source %s %s — %s", item.SourceID, decision, item.ChangeSummary)
					o := ops.Op{
						Type:      ops.OpNote,
						TargetID:  issue.ID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{Msg: noteMsg},
					}
					if err := appendLowStakesOp(logPath, o); err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
							"warning: emit note for %s: %v\n", issue.ID, err)
					}
				}
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Stale review complete. Confirmed: %d/%d\n",
				final.ConfirmedCount(), final.Total())
			return nil
		},
	}
}
