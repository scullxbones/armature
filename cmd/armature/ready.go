package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/tui"
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
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() { err = mapReadyError(err) }()
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
			if assignedTo != "" {
				expiredClaims = filterExpiredClaimsByAssignedWorker(expiredClaims, issues, assignedTo)
			}

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

				filteredExpired := expiredClaims[:0]
				for _, e := range expiredClaims {
					if descendants[e.Issue] {
						filteredExpired = append(filteredExpired, e)
					}
				}
				expiredClaims = filteredExpired
			}

			format, _ := cmd.Flags().GetString("format")
			switch {
			case format == "json" || format == "agent" || tui.IsNonInteractive():
				if waves {
					wavesData := ready.PartitionWaves(entries, index)
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
				selected, err := runReadyTUI(entries)
				if err != nil {
					return err
				}
				if selected != "" {
					state := mustState(cmd)
					ctx := state.ctx
					workerID, logPath, err := resolveWorkerAndLog(ctx)
					if err != nil {
						return err
					}
					op := ops.Op{
						Type:      ops.OpClaim,
						TargetID:  selected,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{TTL: 60},
					}
					if err := appendHighStakesOp(state, logPath, op); err != nil {
						return err
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s\n", selected)
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

// filterExpiredClaimsByAssignedWorker keeps only the expired-claim entries
// whose issue is assigned to assignedTo, per the issue's AssignedWorker field
// (the same field ready.FilterByAssignedTo uses for the main ready list).
// This is deliberately NOT a filter on ClaimedBy (who currently holds the
// claim) — assignment and claim ownership can diverge (e.g. issue assigned
// to worker-a but claimed by worker-b), and this view is about what's
// assigned to assignedTo, not who claimed it.
func filterExpiredClaimsByAssignedWorker(
	expiredClaims []ready.ExpiredClaimEntry,
	issues map[string]*materialize.Issue,
	assignedTo string,
) []ready.ExpiredClaimEntry {
	filtered := expiredClaims[:0]
	for _, e := range expiredClaims {
		issue := issues[e.Issue]
		if issue != nil && issue.AssignedWorker == assignedTo {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

const codeReady1 = "READY-1"

func init() {
	armerrors.Register(codeReady1)
}

func mapReadyError(err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	return armerrors.Wrap(codeReady1, err.Error(), []string{"arm doctor"}, 1, err)
}
