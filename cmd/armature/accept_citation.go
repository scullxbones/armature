package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newAcceptCitationCmd() *cobra.Command {
	var issueIDs []string
	var rationale string
	var ci, force, nonInteractive bool

	cmd := &cobra.Command{
		Use:   "accept-citation [issue-id]",
		Short: "Accept a citation for an issue with a recorded rationale",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(issueIDs) == 0 && len(args) > 0 {
				issueIDs = []string{args[0]}
			}
			if len(issueIDs) == 0 {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			words := strings.Fields(rationale)
			if len(words) < 3 {
				return fmt.Errorf("rationale must be at least 3 words (got %d)", len(words))
			}

			skipPrompt := ci || force || nonInteractive

			if !skipPrompt {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"Accept citation for %d issue(s) with rationale: %q\nConfirm? [y/N]: ", len(issueIDs), rationale)
				scanner := bufio.NewScanner(cmd.InOrStdin())
				scanner.Scan()
				answer := strings.TrimSpace(scanner.Text())
				if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
					return fmt.Errorf("aborted by user")
				}
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			ts := nowEpoch()
			for _, id := range issueIDs {
				op := ops.Op{
					Type:      ops.OpCitationAccepted,
					TargetID:  id,
					Timestamp: ts,
					WorkerID:  workerID,
					Payload: ops.Payload{
						Rationale:                 rationale,
						ConfirmedNoninteractively: skipPrompt,
					},
				}
				if err := appendLowStakesOp(logPath, op); err != nil {
					return err
				}

				result := map[string]any{
					"issue":                      id,
					"rationale":                  rationale,
					"confirmed_noninteractively": skipPrompt,
				}
				data, _ := json.Marshal(result)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&issueIDs, "issue", nil, "issue ID to accept citation for (repeatable)")
	cmd.Flags().StringVar(&rationale, "rationale", "", "rationale for accepting the citation (>=3 words)")
	cmd.Flags().BoolVar(&ci, "ci", false, "bypass interactive prompt (non-interactive/CI mode)")
	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt and proceed (alias for --ci)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "skip confirmation prompt and proceed (alias for --ci)")
	_ = cmd.MarkFlagRequired("rationale")
	return cmd
}
