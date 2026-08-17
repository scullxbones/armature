package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/issueid"
	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var title, nodeType, parent, id, priority, dod, confidence, acceptanceJSON, sourceRef string
	var scope []string
	var contextFiles []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new work item",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate node type before doing anything else.
			if !issuetype.IsValid(nodeType) {
				return fmt.Errorf("invalid type %q: valid types are %s",
					nodeType, strings.Join(issuetype.All(), ", "))
			}

			// Validate parent/type combination when a parent is specified.
			if parent != "" {
				ctx := currentCtx(cmd)
				store := newSnapshotStore(ctx)
				parentIssue, err := store.ReadIssue(parent)
				if err != nil {
					return fmt.Errorf("parent %s not found", parent)
				}
				if !issuetype.IsLegalHierarchy(parentIssue.Type, nodeType) {
					return fmt.Errorf("invalid parent: %s (%s) cannot contain %s", parent, parentIssue.Type, nodeType)
				}
			}

			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}

			if id == "" {
				id = fmt.Sprintf("%s-%d", nodeType, nowEpoch())
			}
			if err := issueid.Validate(id); err != nil {
				return err
			}

			if strings.TrimSpace(confidence) != "" {
				return fmt.Errorf("--confidence is not set at birth (always draft); promote later with arm dag transition --to verified")
			}
			payload := ops.Payload{
				Title:            title,
				NodeType:         nodeType,
				Parent:           parent,
				Scope:            scope,
				ContextFiles:     contextFiles,
				Priority:         priority,
				DefinitionOfDone: dod,
				Confidence:       "draft",
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

			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}

			// If --source was provided, resolve it from the manifest and emit a
			// source-link op so the issue is fully cited in a single invocation.
			if sourceRef != "" {
				dir := sourcesDir(ctx)
				lc := sources.NewLifecycle(dir)

				var entry *sources.SourceEntry
				var resolvedID string
				var resolveErr error

				// Treat the ref as a UUID first; fall back to URL/path lookup.
				if _, parseErr := uuid.Parse(sourceRef); parseErr == nil {
					entry, resolveErr = lc.Get(sourceRef)
					if resolveErr != nil {
						return fmt.Errorf("source %q not found in manifest: %w", sourceRef, resolveErr)
					}
					resolvedID = sourceRef
				} else {
					// Fall back to URL lookup in the manifest.
					entry, resolveErr = lc.GetByURL(sourceRef)
					if resolveErr != nil {
						return fmt.Errorf("source %q not found in manifest", sourceRef)
					}
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
				if err := appendLowStakesOp(state, logPath, slOp); err != nil {
					return err
				}
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"id": id, "status": "created"}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", id)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "item title")
	cmd.Flags().StringVar(&nodeType, "type", "task", "item type: "+strings.Join(issuetype.All(), ", "))
	cmd.Flags().StringVar(&parent, "parent", "", "parent node ID")
	cmd.Flags().StringVar(&id, "id", "", "explicit ID (auto-generated if empty)")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: critical, high, medium, low")
	cmd.Flags().StringVar(&dod, "dod", "", "definition of done")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "file scope globs")
	cmd.Flags().StringSliceVar(&contextFiles, "context-file", nil, "stable reference file to render before work; may be repeated")
	cmd.Flags().StringVar(&confidence, "confidence", "", "rejected at birth; confidence level: draft or verified")
	cmd.Flags().StringVar(&acceptanceJSON, "acceptance", "", "acceptance criteria as JSON array")
	cmd.Flags().StringVar(&sourceRef, "source", "", "source ID (UUID) or URL/path to source-link at creation time")
	_ = cmd.MarkFlagRequired("title")

	return cmd
}
