package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/spf13/cobra"
)

func newClaimCmd() *cobra.Command {
	var issueID string
	var ttl int
	var force bool

	cmd := &cobra.Command{
		Use:   "claim [issue-id]",
		Short: "Claim a ready task",
		Long: `Claim an issue to assign it to the current worker.

Claiming an issue marks it as assigned to your worker ID and sets a TTL (time-to-live).
If the TTL expires without progress, the claim becomes stale and may be reassigned.
This command also detects and warns about scope overlaps with concurrently claimed issues.
When you claim a task, its parent story (if open) is automatically advanced to in-progress.`,
		Example: `  # Claim an issue by ID
  $ arm claim E6-S4-T2

  # Claim with a custom TTL of 120 minutes
  $ arm claim --issue E6-S4-T2 --ttl 120

  # Claim despite scope overlap warning
  $ arm claim E6-S4-T2 --force

  # Claim using flag style
  $ arm claim --issue another-task-id`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			issuesDir := appCtx.IssuesDir

			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir, appCtx.Mode == "single-branch")
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			issue, ok := snap.Issues[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			if issue.Provenance.Confidence == "inferred" {
				return fmt.Errorf("cannot claim %s: node has confidence=inferred — wait for a human to confirm it", issueID)
			}

			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return err
			}

			// Read all ops again for overlap dismissal check
			allOps, _, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}

			// Save parent status from initial snapshot before claim op is emitted
			initialParentStatus := ""
			if parentID := issue.Parent; parentID != "" {
				if parentEntry, ok := snap.Index[parentID]; ok {
					initialParentStatus = parentEntry.Status
				}
			}

			for id, entry := range snap.Index {
				if id == issueID || (entry.Status != "claimed" && entry.Status != "in-progress") {
					continue
				}
				if claimPkg.ScopesOverlap(issue.Scope, entry.Scope) {
					msg := fmt.Sprintf("scope overlap with %s (%s)", id, entry.Title)
					// Same worker claiming serially: auto-dismiss — log a note, no error or warning.
					if entry.Assignee == workerID {
						// Only write the dismissal note if it hasn't been written before for this pair.
						if !claimPkg.HasOverlapDismissalNote(allOps, issueID, id) {
							noteOp := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
								WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Serial claim: scope overlap with %s (same worker, dismissed)", id)}}
							appendOp(appCtx, logPath, noteOp) //nolint:errcheck,gosec,gosec
						}
						continue
					}
					if !force {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", msg)
						return fmt.Errorf("cannot claim %s: %s — use --force to override", issueID, msg)
					}
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", msg)
					noteOp := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
						WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", id)}}
					appendOp(appCtx, logPath, noteOp) //nolint:errcheck,gosec
					noteOp2 := ops.Op{Type: ops.OpNote, TargetID: id, Timestamp: nowEpoch(),
						WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", issueID)}}
					appendOp(appCtx, logPath, noteOp2) //nolint:errcheck,gosec
				}
			}

			op := ops.Op{
				Type: ops.OpClaim, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{TTL: ttl},
			}
			if err := appendHighStakesOp(mustState(cmd), logPath, op); err != nil {
				return err
			}

			snap, err = snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir, appCtx.Mode == "single-branch")
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}
			issueAfter, ok := snap.Issues[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found after claim", issueID)
			}
			won := issueAfter.ClaimedBy == workerID
			if !won {
				format, _ := cmd.Root().PersistentFlags().GetString("format")
				if format == "json" || format == "agent" {
					result := map[string]any{
						"issue":      issueID,
						"claimed":    false,
						"claimed_by": issueAfter.ClaimedBy,
						"reason":     "lost_claim_race",
					}
					data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claim lost for %s (claimed by %s)\n", issueID, issueAfter.ClaimedBy)
				}
				return nil
			}

			// Auto-advance any open ancestor story/epic to in-progress.
			// Note: We check the parent status from the INITIAL snapshot (before claim op)
			// to match the original behavior: if the parent was "open" when we started,
			// we emit an explicit transition op to advance it to in-progress.
			if parentID := issue.Parent; parentID != "" && initialParentStatus == ops.StatusOpen {
				advanceOp := ops.Op{
					Type:      ops.OpTransition,
					TargetID:  parentID,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload:   ops.Payload{To: ops.StatusInProgress},
				}
				appendOp(appCtx, logPath, advanceOp) //nolint:errcheck,gosec
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]any{"issue": issueID, "claimed_by": workerID, "ttl": ttl}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed %s\n", issueID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to claim")
	cmd.Flags().IntVar(&ttl, "ttl", 60, "claim TTL in minutes")
	cmd.Flags().BoolVar(&force, "force", false, "override scope overlap warning and proceed with claim")
	return cmd
}
