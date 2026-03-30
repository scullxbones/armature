package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/sources"
	"github.com/spf13/cobra"
)

func newSourceLinkCmd() *cobra.Command {
	var issueID, sourceID string

	cmd := &cobra.Command{
		Use:   "source-link [issue-id]",
		Short: "Link an issue to a source entry in the manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
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
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to link")
	cmd.Flags().StringVar(&sourceID, "source-id", "", "UUID of the source entry in the manifest")
	_ = cmd.MarkFlagRequired("source-id")
	return cmd
}
