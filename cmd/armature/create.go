package main

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var title, nodeType, parent, id, priority, dod, confidence, acceptanceJSON, sourceRef string
	var scope []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new work item",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			if id == "" {
				id = fmt.Sprintf("%s-%d", nodeType, nowEpoch())
			}

			payload := ops.Payload{
				Title:            title,
				NodeType:         nodeType,
				Parent:           parent,
				Scope:            scope,
				Priority:         priority,
				DefinitionOfDone: dod,
				Confidence:       confidence,
			}

			if acceptanceJSON != "" {
				var raw json.RawMessage
				if err := json.Unmarshal([]byte(acceptanceJSON), &raw); err != nil {
					return fmt.Errorf("invalid --acceptance JSON: %w", err)
				}
				payload.Acceptance = raw
			}

			op := ops.Op{
				Type:      ops.OpCreate,
				TargetID:  id,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   payload,
			}

			if err := appendOp(logPath, op); err != nil {
				return err
			}

			// If --source was provided, resolve it from the manifest and emit a
			// source-link op so the issue is fully cited in a single invocation.
			if sourceRef != "" {
				dir := sourcesDir()
				manifest, err := sources.ReadManifest(dir)
				if err != nil {
					return fmt.Errorf("read manifest: %w", err)
				}

				var entry *sources.SourceEntry
				var resolvedID string

				// Treat the ref as a UUID first; fall back to URL/path lookup.
				if _, parseErr := uuid.Parse(sourceRef); parseErr == nil {
					e, ok := manifest.Get(sourceRef)
					if !ok {
						return fmt.Errorf("source %q not found in manifest", sourceRef)
					}
					entry = e
					resolvedID = sourceRef
				} else {
					e, ok := manifest.GetByURL(sourceRef)
					if !ok {
						return fmt.Errorf("source %q not found in manifest", sourceRef)
					}
					entry = e
					resolvedID = entry.ID
				}

				slOp := ops.Op{
					Type:      ops.OpSourceLink,
					TargetID:  id,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload: ops.Payload{
						SourceID:  resolvedID,
						SourceURL: entry.URL,
					},
				}
				if err := appendLowStakesOp(logPath, slOp); err != nil {
					return err
				}
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"id": id, "status": "created"}
				data, _ := json.Marshal(result)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", id)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "item title")
	cmd.Flags().StringVar(&nodeType, "type", "task", "item type: epic, story, task")
	cmd.Flags().StringVar(&parent, "parent", "", "parent node ID")
	cmd.Flags().StringVar(&id, "id", "", "explicit ID (auto-generated if empty)")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: critical, high, medium, low")
	cmd.Flags().StringVar(&dod, "dod", "", "definition of done")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "file scope globs")
	cmd.Flags().StringVar(&confidence, "confidence", "", "confidence level: draft or verified (default verified)")
	cmd.Flags().StringVar(&acceptanceJSON, "acceptance", "", "acceptance criteria as JSON array")
	cmd.Flags().StringVar(&sourceRef, "source", "", "source ID (UUID) or URL/path to source-link at creation time")
	_ = cmd.MarkFlagRequired("title")

	return cmd
}
