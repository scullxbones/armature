package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the issue graph and documentation",
		Long: `Check the issue graph for structural consistency and traceability coverage.

This command group provides tools for validating the issue DAG and documentation.
Use 'validate graph' to check structural consistency, or 'validate doc-examples' to
validate JSON examples in documentation.`,
		Example: `  # Validate the full issue graph
  $ arm validate graph

  # Validate with strict mode (warnings become errors)
  $ arm validate graph --strict

  # Exit non-zero in CI if any errors found
  $ arm validate graph --ci`,
	}

	cmd.AddCommand(newValidateGraphCmd())
	cmd.AddCommand(newValidateDocExamplesCmd())

	return cmd
}

func newValidateGraphCmd() *cobra.Command {
	var (
		ci     bool
		strict bool
		scope  string
		parent string
		quiet  bool
	)

	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Validate the issue graph for consistency",
		Long: `Check the issue graph for structural consistency and traceability coverage.

This command validates parent-child relationships, dependency links, field requirements,
and coverage metrics (% of issues cited in documentation). Errors prevent merges in CI mode.
Warnings highlight potential issues. Use --ci to exit non-zero on errors, or --strict to
treat warnings as errors. Use --scope to validate only a subtree. Use --parent to validate
only direct children of a parent issue. Use --quiet to suppress INFO lines while still printing
COVERAGE and OK lines.`,
		Example: `  # Validate the full issue graph
  $ arm validate

  # Validate with strict mode (warnings become errors)
  $ arm validate --strict

  # Exit non-zero in CI if any errors found
  $ arm validate --ci

  # Validate only a specific subtree
  $ arm validate --scope parent-issue-id

  # Validate only direct children of a parent issue
  $ arm validate --parent story-id

  # Suppress INFO lines (e.g. phantom-scope notices)
  $ arm validate --quiet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			store := newSnapshotStore(appCtx)
			snap, err := store.Load(cmd.Context())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}
			state := snap.State

			// Read manifest data
			manifestData, err := adapters.ReadManifestFile(filepath.Join(appCtx.IssuesDir, "sources"))
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

			// Read coverage data
			coverageData, err := adapters.ReadCoverageFile(store.StatePath("traceability.json"))
			if err != nil {
				return fmt.Errorf("read coverage: %w", err)
			}
			var cov *traceability.Coverage
			if coverageData != nil {
				cov = &traceability.Coverage{}
				if err := json.Unmarshal(coverageData, cov); err != nil {
					return fmt.Errorf("parse coverage: %w", err)
				}
			}

			// Build a Graph projection from state
			graph := materialize.GraphFromState(state)

			// Expand globs for scope validation
			scopeGlobs := make(map[string][]string)
			for _, issue := range state.Issues {
				scopeGlobs[issue.ID] = issue.Scope
			}
			preExpandedScopes := adapters.ExpandGlobs(scopeGlobs)

			opts := validate.Options{
				ScopeID:           scope,
				ParentID:          parent,
				Strict:            strict,
				ManifestData:      manifestData,
				Coverage:          cov,
				PreExpandedScopes: preExpandedScopes,
			}
			result := validate.Validate(state, graph, opts)

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" {
				payload := map[string]any{
					"errors":   result.Errors,
					"warnings": result.Warnings,
					"infos":    result.Infos,
				}
				if result.Coverage != nil {
					payload["coverage"] = result.Coverage
				}
				out, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			} else {
				if err := output.RenderValidation(cmd.OutOrStdout(), result, quiet); err != nil {
					return fmt.Errorf("render validation: %w", err)
				}
			}

			if (ci || strict) && len(result.Errors) > 0 {
				return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ci, "ci", false, "Exit non-zero if errors found")
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings as errors")
	cmd.Flags().StringVar(&scope, "scope", "", "Validate only the subtree rooted at this node ID")
	cmd.Flags().StringVar(&parent, "parent", "", "Validate only direct children of this parent node ID")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress INFO lines; still prints COVERAGE and OK lines")
	return cmd
}
