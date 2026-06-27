package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newAmendCmd() *cobra.Command {
	var issueID, nodeType, dod, acceptanceJSON string
	var scope []string
	var contextFiles []string
	var clearContextFiles bool

	cmd := &cobra.Command{
		Use:   "amend [issue-id]",
		Short: "Amend fields on an existing issue (type, scope, acceptance, definition_of_done)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}

			payload := ops.Payload{
				NodeType:         nodeType,
				DefinitionOfDone: dod,
			}
			scopeChanged := cmd.Flags().Changed("scope")
			contextFilesChanged := cmd.Flags().Changed("context-file")
			if clearContextFiles && contextFilesChanged {
				return fmt.Errorf("cannot use --clear-context-files and --context-file together")
			}
			if scopeChanged {
				payload.Scope = scope
			}
			if clearContextFiles {
				payload.ClearContextFiles = true
			} else if contextFilesChanged {
				payload.ContextFiles = contextFiles
			}

			if acceptanceJSON != "" {
				var raw json.RawMessage
				if err := json.Unmarshal([]byte(acceptanceJSON), &raw); err != nil {
					return fmt.Errorf("invalid --acceptance JSON: %w", err)
				}
				payload.Acceptance = raw
			}

			if payload.NodeType == "" && !scopeChanged &&
				!contextFilesChanged && !clearContextFiles &&
				len(payload.Acceptance) == 0 && payload.DefinitionOfDone == "" {
				return fmt.Errorf("at least one of --type, --scope, --context-file, --clear-context-files, --acceptance, --dod must be provided")
			}

			op := ops.Op{
				Type:      ops.OpAmend,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   payload,
			}
			if err := appendLowStakesOp(state, logPath, op); err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "status": "amended"}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Amended %s\n", issueID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to amend")
	cmd.Flags().StringVar(&nodeType, "type", "", "new type (epic, story, task)")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "file scope globs")
	cmd.Flags().StringSliceVar(&contextFiles, "context-file", nil, "stable reference file to render before work; replaces the full list")
	cmd.Flags().BoolVar(&clearContextFiles, "clear-context-files", false, "remove all context_files entries from the issue")
	cmd.Flags().StringVar(&dod, "dod", "", "definition of done")
	cmd.Flags().StringVar(&acceptanceJSON, "acceptance", "", "acceptance criteria as JSON array")
	return cmd
}
