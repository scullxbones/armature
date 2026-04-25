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
	var issueID, rationale string
	var ci, force, nonInteractive bool

	cmd := &cobra.Command{
		Use:   "accept-citation [issue-id]",
		Short: "Accept a citation for an issue with a recorded rationale",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			words := strings.Fields(rationale)
			if len(words) < 3 {
				return fmt.Errorf("rationale must be at least 3 words (got %d)", len(words))
			}

			// Determine if we should skip the interactive prompt
			skipPrompt := ci || force || nonInteractive

			if !skipPrompt {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"Accept citation for issue %q with rationale: %q\nConfirm? [y/N]: ", issueID, rationale)
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

			op := ops.Op{
				Type:      ops.OpCitationAccepted,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					Rationale:                 rationale,
					ConfirmedNoninteractively: skipPrompt,
				},
			}
			if err := appendLowStakesOp(logPath, op); err != nil {
				return err
			}

			result := map[string]interface{}{
				"issue":                      issueID,
				"rationale":                  rationale,
				"confirmed_noninteractively": skipPrompt,
			}
			data, _ := json.Marshal(result)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to accept citation for")
	cmd.Flags().StringVar(&rationale, "rationale", "", "rationale for accepting the citation (>=3 words)")
	cmd.Flags().BoolVar(&ci, "ci", false, "bypass interactive prompt (non-interactive/CI mode)")
	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt and proceed (alias for --ci)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "skip confirmation prompt and proceed (alias for --ci)")
	_ = cmd.MarkFlagRequired("rationale")
	return cmd
}
