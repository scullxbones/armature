package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/tui"
	readytui "github.com/scullxbones/armature/internal/tui/ready"
	"github.com/spf13/cobra"
)

func newReadyCmd() *cobra.Command {
	var workerID string
	var filterParent string
	var assignedTo string

	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Show tasks ready to be claimed",
		Long: `Display all issues that are ready to be claimed by a worker.

An issue is ready when it has no unmet blocking dependencies and its status is "open".
This command shows a prioritized list of tasks available for work, optionally filtered
to a specific worker or a subtree of issues. Use --format json for automation.`,
		Example: `  # Show all ready tasks in interactive mode
  $ trls ready

  # Show ready tasks filtered for a specific worker
  $ trls ready --worker alice-worker

  # Show ready tasks in JSON format (suitable for agents)
  $ trls ready --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, appCtx.Mode == "single-branch"); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			index, err := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
			if err != nil {
				return err
			}

			issues := make(map[string]*materialize.Issue)
			for id := range index {
				issue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", id+".json"))
				if err == nil {
					issues[id] = &issue
				}
			}

			entries := ready.ComputeReady(index, issues, workerID)

			// Apply --assigned-to filter: keep only tasks assigned to the given worker.
			entries = ready.FilterByAssignedTo(entries, assignedTo)

			// Apply --parent filter: keep only descendants of the given issue.
			if filterParent != "" {
				descendants := collectDescendants(filterParent, index)
				filtered := entries[:0]
				for _, e := range entries {
					if descendants[e.Issue] {
						filtered = append(filtered, e)
					}
				}
				entries = filtered
			}

			format, _ := cmd.Flags().GetString("format")
			if format == "json" || format == "agent" || tui.IsNonInteractive() {
				data, _ := json.MarshalIndent(entries, "", "  ")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else if tui.IsInteractive() {
				m := readytui.New(entries)
				p := tea.NewProgram(m)
				finalModel, err := p.Run()
				if err != nil {
					return err
				}
				final, ok := finalModel.(readytui.Model)
				if !ok {
					return fmt.Errorf("unexpected model type from TUI")
				}
				if final.Selected() != "" {
					workerID, logPath, err := resolveWorkerAndLog()
					if err != nil {
						return err
					}
					op := ops.Op{
						Type:      ops.OpClaim,
						TargetID:  final.Selected(),
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{TTL: 60},
					}
					if err := appendHighStakesOp(logPath, op); err != nil {
						return err
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s\n", final.Selected())
				}
				return nil
			} else {
				if len(entries) == 0 {
					stale := ready.StaleClaims(issues, time.Now())
					if len(stale) > 0 {
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "# stale claims (TTL expired):")
						for _, id := range stale {
							issue := issues[id]
							wid := ""
							if issue != nil {
								wid = issue.ClaimedBy
							}
							_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "#   %s (claimed by %s)\n", id, wid)
						}
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tasks ready.")
					return nil
				}
				for _, e := range entries {
					conf := ""
					if e.RequiresConfirmation {
						conf = " [requires confirmation]"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s  (%s)%s\n", e.Issue, e.Title, e.Priority, conf)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workerID, "worker", "", "worker ID for assignment-aware sorting")
	cmd.Flags().StringVar(&filterParent, "parent", "", "filter to descendants of this issue ID")
	cmd.Flags().StringVar(&assignedTo, "assigned-to", "", "filter to tasks assigned to this worker ID")
	return cmd
}

// collectDescendants returns the set of all descendant IDs of root (not including root itself).
func collectDescendants(root string, index materialize.Index) map[string]bool {
	result := make(map[string]bool)
	queue := []string{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		entry, ok := index[current]
		if !ok {
			continue
		}
		for _, child := range entry.Children {
			if !result[child] {
				result[child] = true
				queue = append(queue, child)
			}
		}
	}
	return result
}
