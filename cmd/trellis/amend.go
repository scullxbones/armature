package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newAmendCmd() *cobra.Command {
	var issueID, nodeType, dod, acceptanceJSON string
	var scope []string

	cmd := &cobra.Command{
		Use:   "amend",
		Short: "Amend fields on an existing issue (type, scope, acceptance, definition_of_done)",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			payload := ops.Payload{
				NodeType:         nodeType,
				Scope:            scope,
				DefinitionOfDone: dod,
			}

			if acceptanceJSON != "" {
				var raw json.RawMessage
				if err := json.Unmarshal([]byte(acceptanceJSON), &raw); err != nil {
					return fmt.Errorf("invalid --acceptance JSON: %w", err)
				}
				payload.Acceptance = raw
			}

			if payload.NodeType == "" && len(payload.Scope) == 0 &&
				len(payload.Acceptance) == 0 && payload.DefinitionOfDone == "" {
				return fmt.Errorf("at least one of --type, --scope, --acceptance, --dod must be provided")
			}

			op := ops.Op{
				Type:      ops.OpAmend,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   payload,
			}
			if err := appendLowStakesOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "status": "amended"}
			data, _ := json.Marshal(result)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to amend")
	cmd.Flags().StringVar(&nodeType, "type", "", "new type (epic, story, task)")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "file scope globs")
	cmd.Flags().StringVar(&dod, "dod", "", "definition of done")
	cmd.Flags().StringVar(&acceptanceJSON, "acceptance", "", "acceptance criteria as JSON array")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
