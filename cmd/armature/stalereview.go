package main

import (
	"context"
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
			execState := mustState(cmd)
			appCtx := execState.ctx

			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			lc := sources.NewLifecycle(sourcesDir())

			// Load snapshot to get materialized state
			store := newSnapshotStore(appCtx)
			snap, err := store.Load(context.Background())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			state := snap.State
			if state == nil {
				state = &materialize.State{Issues: make(map[string]*materialize.Issue)}
			}

			// Detect stale entries.
			verifyResults, _ := lc.VerifyAll() //nolint:errcheck // all results included even if some entries are not OK

			var reviewItems []stalereview.ReviewItem
			for _, result := range verifyResults {
				// Only include sources that are not OK.
				if result.Status == sources.VerifyOK {
					continue
				}

				// Find cited issues.
				var cited []*materialize.Issue
				for _, issue := range state.Issues {
					if len(issue.SourceLinks) == 0 {
						continue
					}
					for _, link := range issue.SourceLinks {
						if link.SourceEntryID == result.ID {
							cited = append(cited, issue)
							break
						}
					}
				}
				sort.Slice(cited, func(i, j int) bool {
					return cited[i].ID < cited[j].ID
				})

				summary := ""
				switch result.Status {
				case sources.VerifyChanged:
					summary = fmt.Sprintf("fingerprint changed (stored: %s, current: %s)",
						result.Stored[:8], result.Current[:8])
				case sources.VerifyMissing:
					summary = "no cache found"
				case sources.VerifyStale:
					summary = "last sync failed"
				case sources.VerifyError:
					summary = fmt.Sprintf("error: %v", result.Error)
				}

				reviewItems = append(reviewItems, stalereview.ReviewItem{
					SourceID:      result.ID,
					ChangeSummary: summary,
					CitedIssues:   cited,
				})
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
				data, _ := json.MarshalIndent(map[string]interface{}{ //nolint:errcheck // map of serializable values
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
			final := finalModel.(stalereview.Model) //nolint:errcheck // map of serializable values

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
					if err := appendLowStakesOp(execState, logPath, o); err != nil {
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
