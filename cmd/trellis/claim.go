package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	claimPkg "github.com/scullxbones/trellis/internal/claim"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
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
  $ trls claim E6-S4-T2

  # Claim with a custom TTL of 120 minutes
  $ trls claim --issue E6-S4-T2 --ttl 120

  # Claim despite scope overlap warning
  $ trls claim E6-S4-T2 --force

  # Claim using flag style
  $ trls claim --issue another-task-id`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			issuesDir := appCtx.IssuesDir

			if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, appCtx.Mode == "single-branch"); err != nil {
				return err
			}

			issue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", issueID+".json"))
			if err != nil {
				return fmt.Errorf("issue %s not found: %w", issueID, err)
			}

			if issue.Provenance.Confidence == "inferred" {
				return fmt.Errorf("cannot claim %s: node has confidence=inferred — wait for a human to confirm it", issueID)
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			index, _ := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
			for id, entry := range index {
				if id == issueID || (entry.Status != "claimed" && entry.Status != "in-progress") {
					continue
				}
				if claimPkg.ScopesOverlap(issue.Scope, entry.Scope) {
					msg := fmt.Sprintf("scope overlap with %s (%s)", id, entry.Title)
					if !force {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", msg)
						return fmt.Errorf("cannot claim %s: %s — use --force to override", issueID, msg)
					}
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", msg)
					noteOp := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
						WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", id)}}
					appendOp(logPath, noteOp) //nolint:errcheck
					noteOp2 := ops.Op{Type: ops.OpNote, TargetID: id, Timestamp: nowEpoch(),
						WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", issueID)}}
					appendOp(logPath, noteOp2) //nolint:errcheck
				}
			}

			op := ops.Op{
				Type: ops.OpClaim, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{TTL: ttl},
			}
			if err := appendHighStakesOp(logPath, op); err != nil {
				return err
			}

			// Auto-advance any open ancestor story/epic to in-progress.
			if parentID := issue.Parent; parentID != "" {
				if parentEntry, ok := index[parentID]; ok && parentEntry.Status == ops.StatusOpen {
					advanceOp := ops.Op{
						Type:      ops.OpTransition,
						TargetID:  parentID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{To: ops.StatusInProgress},
					}
					appendOp(logPath, advanceOp) //nolint:errcheck
				}
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]interface{}{"issue": issueID, "claimed_by": workerID, "ttl": ttl}
				data, _ := json.Marshal(result)
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
