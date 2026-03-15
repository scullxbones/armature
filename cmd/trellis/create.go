package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var repoPath, title, nodeType, parent, id, priority, dod string
	var scope []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new work item",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}

			if id == "" {
				id = fmt.Sprintf("%s-%d", nodeType, nowEpoch())
			}

			op := ops.Op{
				Type:      ops.OpCreate,
				TargetID:  id,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					Title:            title,
					NodeType:         nodeType,
					Parent:           parent,
					Scope:            scope,
					Priority:         priority,
					DefinitionOfDone: dod,
				},
			}

			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}

			result := map[string]string{"id": id, "status": "created"}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&title, "title", "", "item title")
	cmd.Flags().StringVar(&nodeType, "type", "task", "item type: epic, story, task")
	cmd.Flags().StringVar(&parent, "parent", "", "parent node ID")
	cmd.Flags().StringVar(&id, "id", "", "explicit ID (auto-generated if empty)")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: critical, high, medium, low")
	cmd.Flags().StringVar(&dod, "dod", "", "definition of done")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "file scope globs")
	cmd.MarkFlagRequired("title")

	return cmd
}
