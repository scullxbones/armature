package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/sources"
	"github.com/spf13/cobra"
)

func newSourceLinkCmd() *cobra.Command {
	var issueIDs []string
	var sourceID string

	cmd := &cobra.Command{
		Use:   "source-link [issue-id]",
		Short: "Link one or more issues to a source entry in the manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Positional arg is a backward-compat single-issue path.
			if len(issueIDs) == 0 && len(args) > 0 {
				issueIDs = []string{args[0]}
			}
			if len(issueIDs) == 0 {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			dir := sourcesDir()
			manifest, err := sources.ReadManifest(dir)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

			entry, ok := manifest.Get(sourceID)
			if !ok {
				return fmt.Errorf("source-id %q not found in manifest", sourceID)
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			for _, issueID := range issueIDs {
				op := ops.Op{
					Type:      ops.OpSourceLink,
					TargetID:  issueID,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload: ops.Payload{
						SourceID:  sourceID,
						SourceURL: entry.URL,
					},
				}
				if err := appendLowStakesOp(logPath, op); err != nil {
					return err
				}

				result := map[string]string{"issue": issueID, "source_id": sourceID, "source_url": entry.URL}
				data, _ := json.Marshal(result)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&issueIDs, "issue", nil, "issue ID to link (repeatable)")
	cmd.Flags().StringVar(&sourceID, "source-id", "", "UUID of the source entry in the manifest")
	_ = cmd.MarkFlagRequired("source-id")
	return cmd
}
