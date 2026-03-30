package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/scullxbones/trellis/internal/hooks"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newTransitionCmd() *cobra.Command {
	var issueID, to, outcome, branch, pr string

	cmd := &cobra.Command{
		Use:   "transition [issue-id]",
		Short: "Transition an issue to a new status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			if !ops.ValidTransitionTargets[to] {
				valid := []string{}
				for s := range ops.ValidTransitionTargets {
					valid = append(valid, s)
				}
				sort.Strings(valid)
				return fmt.Errorf("invalid status %q: valid values are %v", to, valid)
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			issuesDir := appCtx.IssuesDir
			cfg := appCtx.Config

			{
				// Get current issue status from materialized index
				index, _ := materialize.LoadIndex(filepath.Join(issuesDir, "index.json"))
				currentStatus := ""
				if entry, ok := index[issueID]; ok {
					currentStatus = entry.Status
				}

				hookInput := hooks.HookInput{
					IssueID:    issueID,
					FromStatus: currentStatus,
					ToStatus:   to,
					WorkerID:   workerID,
				}
				if err := hooks.RunPreTransition(&cfg, hookInput); err != nil {
					return err
				}
			}

			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID,
				Payload:  ops.Payload{To: to, Outcome: outcome, Branch: branch, PR: pr},
			}
			if err := appendHighStakesOp(logPath, op); err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "status": to}
				data, _ := json.Marshal(result)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s → %s\n", issueID, to)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&to, "to", "", "target status")
	cmd.Flags().StringVar(&outcome, "outcome", "", "outcome description")
	cmd.Flags().StringVar(&branch, "branch", "", "feature branch name")
	cmd.Flags().StringVar(&pr, "pr", "", "PR number")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
