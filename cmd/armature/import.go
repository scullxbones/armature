package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/importbf"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	var source string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import issues from a CSV or JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}

			ext := strings.ToLower(filepath.Ext(filePath))
			var items []importbf.ImportedIssue
			switch ext {
			case ".csv":
				items, err = importbf.ParseCSV(data)
			case ".json":
				items, err = importbf.ParseJSON(data)
			default:
				return fmt.Errorf("unsupported file extension %q: use .csv or .json", ext)
			}
			if err != nil {
				return fmt.Errorf("parse file: %w", err)
			}

			if dryRun {
				format, _ := cmd.Flags().GetString("format")
				if format == "json" {
					ids := make([]string, len(items))
					for i, item := range items {
						ids[i] = item.ID
					}
					out, _ := json.Marshal(map[string]interface{}{
						"created":   len(items),
						"issue_ids": ids,
						"dry_run":   true,
					})
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would import %d items\n", len(items))
				}
				return nil
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			var createdIDs []string
			for _, item := range items {
				createOp := ops.Op{
					Type:      ops.OpCreate,
					TargetID:  item.ID,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload: ops.Payload{
						Title:    item.Title,
						NodeType: item.Type,
						Parent:   item.Parent,
						Scope:    item.Scope,
					},
				}
				if err := appendLowStakesOp(logPath, createOp); err != nil {
					return fmt.Errorf("emit create op for %s: %w", item.ID, err)
				}
				createdIDs = append(createdIDs, item.ID)

				if source != "" {
					sourceLinkOp := ops.Op{
						Type:      ops.OpSourceLink,
						TargetID:  item.ID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload: ops.Payload{
							SourceID: source,
						},
					}
					if err := appendLowStakesOp(logPath, sourceLinkOp); err != nil {
						return fmt.Errorf("emit source-link op for %s: %w", item.ID, err)
					}
				}
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" {
				out, _ := json.Marshal(map[string]interface{}{
					"created":   len(createdIDs),
					"issue_ids": createdIDs,
				})
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "imported %d items\n", len(createdIDs))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&source, "source", "", "source ID to link imported items to")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be imported without writing ops")

	return cmd
}
