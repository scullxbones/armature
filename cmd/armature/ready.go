package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/tui"
	readytui "github.com/scullxbones/armature/internal/tui/ready"
	"github.com/spf13/cobra"
)

func newReadyCmd() *cobra.Command {
	var workerID string
	var filterParent string
	var assignedTo string
	var explain bool
	var waves bool

	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Show tasks ready to be claimed",
		Long: `Display all issues that are ready to be claimed by a worker.

An issue is ready when it has no unmet blocking dependencies and its status is "open".
This command shows a prioritized list of tasks available for work, optionally filtered
to a specific worker or a subtree of issues. Use --format json for automation.`,
		Example: `  # Show all ready tasks in interactive mode
  $ arm ready

  # Show ready tasks filtered for a specific worker
  $ arm ready --worker alice-worker

  # Show ready tasks scoped to a specific story subtree
  $ arm ready --parent STORY-ID

  # Show ready tasks in JSON format (suitable for agents)
  $ arm ready --format json

  # Diagnose why open tasks are not in the ready queue
  $ arm ready --explain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)

			store := newSnapshotStore(ctx)
			snap, err := store.Load(cmd.Context())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}
			index := snap.Index
			issues := snap.Issues

			// --explain: print why each open unclaimed task is not ready, then return.
			if explain {
				notReady := ready.ExplainNotReady(index, issues, nowEpoch())
				format, _ := cmd.Flags().GetString("format")
				if format == "json" || format == "agent" || tui.IsNonInteractive() {
					data, _ := json.MarshalIndent(notReady, "", "  ") //nolint:errcheck // slice of serializable structs
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				} else {
					ids := make([]string, 0, len(notReady))
					for id := range notReady {
						ids = append(ids, id)
					}
					sort.Strings(ids)
					for _, id := range ids {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", id, notReady[id])
					}
				}
				return nil
			}

			entries := ready.ComputeReady(index, issues, workerID, nowEpoch())
			expiredClaims := ready.ExpiredClaims(issues, time.Now())

			// Apply --assigned-to filter: keep only tasks assigned to the given worker.
			entries = ready.FilterByAssignedTo(entries, assignedTo)

			// Apply --parent filter: keep only descendants of the given issue.
			if filterParent != "" {
				descendants := ready.CollectDescendants(filterParent, index)
				filtered := entries[:0]
				for _, e := range entries {
					if descendants[e.Issue] {
						filtered = append(filtered, e)
					}
				}
				entries = filtered
			}

			format, _ := cmd.Flags().GetString("format")
			switch {
			case format == "json" || format == "agent" || tui.IsNonInteractive():
				if waves {
					// Partition ready entries into scope-disjoint waves
					nodeIndex := make(map[string]*dag.Node)
					for id, entry := range index {
						node := &dag.Node{
							ID:        id,
							Title:     entry.Title,
							Type:      entry.Type,
							Parent:    entry.Parent,
							Children:  make([]string, len(entry.Children)),
							BlockedBy: make([]string, len(entry.BlockedBy)),
							Blocks:    make([]string, len(entry.Blocks)),
						}
						copy(node.Children, entry.Children)
						copy(node.BlockedBy, entry.BlockedBy)
						copy(node.Blocks, entry.Blocks)
						nodeIndex[id] = node
					}
					graph := dag.FromIndex(nodeIndex)
					wavesData := ready.PartitionWaves(entries, index, graph)
					if err := output.RenderReadyWaves(cmd.OutOrStdout(), wavesData); err != nil {
						return err
					}
				} else {
					if err := output.RenderReady(cmd.OutOrStdout(), entries, true); err != nil {
						return err
					}
				}
				// Distinct expired-claims section: kept off stdout (which JSON/agent
				// consumers parse as the ready queue) and printed to stderr, same as
				// the snapshot warnings above — visible, but not part of the parsed
				// payload shape.
				if len(expiredClaims) > 0 {
					if err := output.RenderExpiredClaims(cmd.ErrOrStderr(), expiredClaims, true); err != nil {
						return err
					}
				}
			case tui.IsInteractive():
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
					state := mustState(cmd)
					ctx := state.ctx
					workerID, logPath, err := resolveWorkerAndLog(ctx)
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
					if err := appendHighStakesOp(state, logPath, op); err != nil {
						return err
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s\n", final.Selected())
				}
				return nil
			default:
				if len(entries) == 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tasks ready.")
				} else if err := output.RenderReady(cmd.OutOrStdout(), entries, false); err != nil {
					return err
				}
				// Distinct expired-claims section, always shown (not just when the
				// ready queue is empty) so expired claims are never silently omitted
				// nor silently folded into the ready list.
				if err := output.RenderExpiredClaims(cmd.OutOrStdout(), expiredClaims, false); err != nil {
					return err
				}
				return nil
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workerID, "worker", "", "worker ID for assignment-aware sorting")
	cmd.Flags().StringVar(&filterParent, "parent", "", "filter to descendants of this issue ID")
	cmd.Flags().StringVar(&assignedTo, "assigned-to", "", "filter to tasks assigned to this worker ID")
	cmd.Flags().BoolVar(&explain, "explain", false, "diagnose why open tasks are not in the ready queue")
	cmd.Flags().BoolVar(&waves, "waves", false, "partition ready entries into scope-disjoint waves (JSON/agent output only)")
	return cmd
}
