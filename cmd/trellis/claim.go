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

	cmd := &cobra.Command{
		Use:   "claim",
		Short: "Claim a ready task",
		RunE: func(cmd *cobra.Command, args []string) error {
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
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: scope overlap with %s (%s)\n", id, entry.Title)
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

			result := map[string]interface{}{"issue": issueID, "claimed_by": workerID, "ttl": ttl}
			data, _ := json.Marshal(result)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to claim")
	cmd.Flags().IntVar(&ttl, "ttl", 60, "claim TTL in minutes")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
