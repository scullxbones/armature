package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/tui"
	"github.com/scullxbones/trellis/internal/tui/dagsum"
	"github.com/scullxbones/trellis/internal/traceability"
	"github.com/spf13/cobra"
)

func newDAGSummaryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dag-summary",
		Short: "Interactive TUI for reviewing and signing off DAG items",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			state, _, err := materialize.MaterializeAndReturn(issuesDir, true)
			if err != nil {
				return err
			}

			var unconfirmed []*materialize.Issue
			for _, issue := range state.Issues {
				if !issue.Provenance.DAGConfirmed {
					unconfirmed = append(unconfirmed, issue)
				}
			}
			sort.Slice(unconfirmed, func(i, j int) bool {
				return unconfirmed[i].ID < unconfirmed[j].ID
			})

			if len(unconfirmed) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "All items already reviewed.")
				return nil
			}

			tracePath := filepath.Join(issuesDir, "state", "traceability.json")
			cov, _ := traceability.Read(tracePath)

			if !tui.IsInteractive() {
				type pendingItem struct {
					IssueID string `json:"issue_id"`
					Title   string `json:"title"`
					Status  string `json:"status"`
				}
				var pending []pendingItem
				for _, issue := range unconfirmed {
					pending = append(pending, pendingItem{
						IssueID: issue.ID,
						Title:   issue.Title,
						Status:  issue.Status,
					})
				}
				out, _ := json.Marshal(map[string]interface{}{
					"pending_dag_confirmation": pending,
					"count":                   len(pending),
				})
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Traceability: %.1f%% (%d/%d nodes cited)\n\n",
				cov.CoveragePct, cov.CitedNodes, cov.TotalNodes)

			m := dagsum.New(unconfirmed, workerID, "")
			p := tea.NewProgram(m)
			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("dag-summary TUI: %w", err)
			}
			final := finalModel.(dagsum.Model)

			confirmedIDs := final.ConfirmedIDs()
			for _, id := range confirmedIDs {
				o := ops.Op{
					Type:      ops.OpDAGTransition,
					TargetID:  id,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
				}
				if err := appendLowStakesOp(logPath, o); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: emit dag-transition for %s: %v\n", id, err)
				}
			}

			if err := writeDAGSummaryArtifact(issuesDir, unconfirmed, confirmedIDs, cov); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: write dag-summary.md: %v\n", err)
			}

			return nil
		},
	}
}

func writeDAGSummaryArtifact(issuesDir string, reviewed []*materialize.Issue,
	confirmedIDs []string, cov traceability.Coverage) error {

	confirmedSet := make(map[string]struct{}, len(confirmedIDs))
	for _, id := range confirmedIDs {
		confirmedSet[id] = struct{}{}
	}

	var sb strings.Builder
	sb.WriteString("# DAG Summary Review\n\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s\n\n", time.Now().UTC().Format("2006-01-02T15:04:05Z")))
	sb.WriteString(fmt.Sprintf("**Traceability:** %.1f%% (%d/%d cited)\n\n",
		cov.CoveragePct, cov.CitedNodes, cov.TotalNodes))
	sb.WriteString("## Review Results\n\n")
	sb.WriteString("| ID | Title | Status |\n|---|---|---|\n")
	for _, issue := range reviewed {
		status := "skipped"
		if _, ok := confirmedSet[issue.ID]; ok {
			status = "✓ confirmed"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", issue.ID, issue.Title, status))
	}

	path := filepath.Join(issuesDir, "state", "dag-summary.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
