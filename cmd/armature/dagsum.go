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
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/tui/dagsummary"
	"github.com/spf13/cobra"
)

func newDAGSummaryCmd() *cobra.Command {
	var issueID string
	var approveAll bool

	cmd := &cobra.Command{
		Use:   "dag-summary",
		Short: "Interactive TUI for reviewing and signing off DAG items",
		Long: `Review and approve draft nodes in the issue DAG (Directed Acyclic Graph).

This command presents an interactive TUI for signing off on draft nodes that have been
inferred or awaiting confirmation. You can review traceability coverage, accept/reject
individual items, and generate a sign-off artifact. Use --approve-all in non-interactive
mode (agents) to auto-approve all pending draft items.`,
		Example: `  # Open interactive TUI to review and approve draft nodes
  $ arm dag-summary

  # Review only draft nodes in a subtree
  $ arm dag-summary --issue parent-task-id

  # Auto-approve all draft items in agent mode
  $ arm dag-summary --approve-all --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			state, _, err := materialize.MaterializeAndReturn(issuesDir, appCtx.StateDir, true)
			if err != nil {
				return err
			}

			tracePath := filepath.Join(appCtx.StateDir, "traceability.json")
			cov, _ := traceability.Read(tracePath)

			// Build a set of uncited IDs for fast lookup.
			uncitedSet := make(map[string]struct{}, len(cov.Uncited))
			for _, id := range cov.Uncited {
				uncitedSet[id] = struct{}{}
			}

			// Collect draft nodes from the subtree (or globally if no --issue given).
			var draftIssues []*materialize.Issue
			if issueID != "" {
				draftIssues = collectDraftSubtree(state, issueID)
			} else {
				for _, issue := range state.Issues {
					if issue.Provenance.Confidence == "draft" {
						draftIssues = append(draftIssues, issue)
					}
				}
			}
			sort.Slice(draftIssues, func(i, j int) bool {
				return draftIssues[i].ID < draftIssues[j].ID
			})

			if len(draftIssues) == 0 {
				format, _ := cmd.Flags().GetString("format")
				if format == "json" || format == "agent" || tui.IsNonInteractive() {
					data, _ := json.MarshalIndent(map[string]interface{}{
						"pending_dag_confirmation": []interface{}{},
						"count":                    0,
						"approve_all":              approveAll,
					}, "", "  ")
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				} else {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No draft nodes found.")
				}
				return nil
			}

			format, _ := cmd.Flags().GetString("format")
			if format == "json" || format == "agent" || tui.IsNonInteractive() {
				// In non-interactive mode, --approve-all emits ops for all draft items.
				if approveAll && len(draftIssues) > 0 {
					approvedIDs := make([]string, 0, len(draftIssues))
					for _, issue := range draftIssues {
						approvedIDs = append(approvedIDs, issue.ID)
					}
					for _, id := range approvedIDs {
						o := ops.Op{
							Type:      ops.OpDAGTransition,
							TargetID:  id,
							Timestamp: nowEpoch(),
							WorkerID:  workerID,
							Payload: ops.Payload{
								IssueID: id,
								To:      "verified",
							},
						}
						if err := appendLowStakesOp(logPath, o); err != nil {
							_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: emit dag-transition for %s: %v\n", id, err)
						}
					}
					if err := writeDAGSummaryArtifact(appCtx.StateDir, draftIssues, approvedIDs, cov); err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: write dag-summary.md: %v\n", err)
					}
				}

				type pendingItem struct {
					IssueID string `json:"issue_id"`
					Title   string `json:"title"`
					Status  string `json:"status"`
				}
				var pending []pendingItem
				for _, issue := range draftIssues {
					pending = append(pending, pendingItem{
						IssueID: issue.ID,
						Title:   issue.Title,
						Status:  issue.Status,
					})
				}
				data, _ := json.MarshalIndent(map[string]interface{}{
					"pending_dag_confirmation": pending,
					"count":                    len(pending),
					"approve_all":              approveAll,
				}, "", "  ")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			if !tui.IsTerminal() {
				// Human-readable summary for non-TTY (format == "human")
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"Traceability: %.1f%% (%d/%d nodes cited)\n\n",
					cov.CoveragePct, cov.CitedNodes, cov.TotalNodes)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pending DAG Confirmation:")
				for _, issue := range draftIssues {
					_, isUncited := uncitedSet[issue.ID]
					cited := "cited"
					if isUncited {
						cited = "uncited"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s (%s)\n", issue.ID, issue.Title, cited)
				}
				return nil
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),

				"Traceability: %.1f%% (%d/%d nodes cited)\n\n",
				cov.CoveragePct, cov.CitedNodes, cov.TotalNodes)

			// Build dagsummary items with IsCited populated.
			items := make([]dagsummary.Item, len(draftIssues))
			for i, issue := range draftIssues {
				_, isUncited := uncitedSet[issue.ID]
				items[i] = dagsummary.Item{
					ID:      issue.ID,
					Title:   issue.Title,
					IsCited: !isUncited,
				}
			}

			rootID := issueID
			m := dagsummary.New(items, rootID)
			p := tea.NewProgram(m)
			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("dag-summary TUI: %w", err)
			}
			final := finalModel.(dagsummary.Model)

			// Only emit ops if sign-off was confirmed.
			if !final.Done() {
				return nil
			}

			approvedIDs := final.ApprovedIDs()
			for _, id := range approvedIDs {
				o := ops.Op{
					Type:      ops.OpDAGTransition,
					TargetID:  id,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload: ops.Payload{
						IssueID: id,
						To:      "verified",
					},
				}
				if err := appendLowStakesOp(logPath, o); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: emit dag-transition for %s: %v\n", id, err)
				}
			}

			if err := writeDAGSummaryArtifact(appCtx.StateDir, draftIssues, approvedIDs, cov); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: write dag-summary.md: %v\n", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "root issue ID of the subtree to review (default: all draft nodes)")
	cmd.Flags().BoolVar(&approveAll, "approve-all", false, "approve all pending draft items (non-interactive only)")
	return cmd
}

// collectDraftSubtree walks the issue subtree rooted at rootID and returns
// all issues with confidence == "draft".
func collectDraftSubtree(state *materialize.State, rootID string) []*materialize.Issue {
	root, ok := state.Issues[rootID]
	if !ok {
		return nil
	}
	var result []*materialize.Issue
	var walk func(id string)
	walk = func(id string) {
		issue, ok := state.Issues[id]
		if !ok {
			return
		}
		if issue.Provenance.Confidence == "draft" {
			result = append(result, issue)
		}
		for _, childID := range issue.Children {
			walk(childID)
		}
	}
	walk(root.ID)
	return result
}

func writeDAGSummaryArtifact(stateDir string, reviewed []*materialize.Issue,
	approvedIDs []string, cov traceability.Coverage) error {

	approvedSet := make(map[string]struct{}, len(approvedIDs))
	for _, id := range approvedIDs {
		approvedSet[id] = struct{}{}
	}

	var sb strings.Builder
	sb.WriteString("# DAG Summary Review\n\n")
	fmt.Fprintf(&sb, "**Date:** %s\n\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(&sb, "**Traceability:** %.1f%% (%d/%d cited)\n\n",
		cov.CoveragePct, cov.CitedNodes, cov.TotalNodes)
	sb.WriteString("## Review Results\n\n")
	sb.WriteString("| ID | Title | Status |\n|---|---|---|\n")
	for _, issue := range reviewed {
		status := "skipped/rejected"
		if _, ok := approvedSet[issue.ID]; ok {
			status = "approved"
		}
		fmt.Fprintf(&sb, "| %s | %s | %s |\n", issue.ID, issue.Title, status)
	}

	path := filepath.Join(stateDir, "dag-summary.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
