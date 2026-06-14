package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/spf13/cobra"
)

// validNodeTypes is the complete set of accepted node types for arm create.
var validNodeTypes = map[string]bool{
	"epic":    true,
	"story":   true,
	"feature": true,
	"task":    true,
	"bug":     true,
}

// validNodeTypesList is the sorted list of valid types for error messages.
var validNodeTypesList = []string{"epic", "story", "feature", "task", "bug"}

// validParentChildTypes defines which parent types may contain which child types.
var validParentChildTypes = map[string]map[string]bool{
	"epic":    {"story": true, "feature": true, "task": true, "bug": true},
	"story":   {"task": true, "bug": true},
	"feature": {"task": true, "bug": true},
	"task":    {},
	"bug":     {},
}

func newCreateCmd() *cobra.Command {
	var title, nodeType, parent, id, priority, dod, confidence, acceptanceJSON, sourceRef string
	var scope []string
	var contextFiles []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new work item",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate node type before doing anything else.
			if !validNodeTypes[nodeType] {
				return fmt.Errorf("invalid type %q: valid types are %s",
					nodeType, strings.Join(validNodeTypesList, ", "))
			}

			// Validate parent/type combination when a parent is specified.
			if parent != "" {
				appCtx := currentCtx(cmd)
				allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(appCtx.IssuesDir, "ops"))
				if err != nil {
					return fmt.Errorf("read ops: %w", err)
				}
				if _, err := materialize.Materialize(appCtx.StateDir, allOps, appCtx.Mode == "single-branch", offsets); err != nil {
					return err
				}
				parentIssue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", parent+".json"))
				if err != nil {
					return fmt.Errorf("parent %s not found: %w", parent, err)
				}
				allowed, ok := validParentChildTypes[parentIssue.Type]
				if !ok || !allowed[nodeType] {
					return fmt.Errorf("invalid parent: %s (%s) cannot contain %s", parent, parentIssue.Type, nodeType)
				}
			}

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
				ContextFiles:     contextFiles,
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

			format, _ := cmd.Root().PersistentFlags().GetString("format") //nolint:errcheck // fails only if flag absent (programming error)
			if format == "json" || format == "agent" {
				result := map[string]string{"id": id, "status": "created"}
				data, _ := json.Marshal(result)                      //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data)) //nolint:errcheck // stdout write not actionable in CLI
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", id) //nolint:errcheck // stdout write not actionable in CLI
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "item title")
	cmd.Flags().StringVar(&nodeType, "type", "task", "item type: epic, story, task, bug")
	cmd.Flags().StringVar(&parent, "parent", "", "parent node ID")
	cmd.Flags().StringVar(&id, "id", "", "explicit ID (auto-generated if empty)")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: critical, high, medium, low")
	cmd.Flags().StringVar(&dod, "dod", "", "definition of done")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "file scope globs")
	cmd.Flags().StringSliceVar(&contextFiles, "context-file", nil, "stable reference file to render before work; may be repeated")
	cmd.Flags().StringVar(&confidence, "confidence", "", "confidence level: draft or verified (default verified)")
	cmd.Flags().StringVar(&acceptanceJSON, "acceptance", "", "acceptance criteria as JSON array")
	cmd.Flags().StringVar(&sourceRef, "source", "", "source ID (UUID) or URL/path to source-link at creation time")
	_ = cmd.MarkFlagRequired("title") //nolint:errcheck // fails only if flag absent (programming error)

	return cmd
}
