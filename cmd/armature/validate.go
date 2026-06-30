package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var (
		ci     bool
		strict bool
		scope  string
		quiet  bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the issue graph for consistency",
		Long: `Check the issue graph for structural consistency and traceability coverage.

This command validates parent-child relationships, dependency links, field requirements,
and coverage metrics (% of issues cited in documentation). Errors prevent merges in CI mode.
Warnings highlight potential issues. Use --ci to exit non-zero on errors, or --strict to
treat warnings as errors. Use --scope to validate only a subtree. Use --quiet to suppress
INFO lines while still printing COVERAGE and OK lines.`,
		Example: `  # Validate the full issue graph
  $ arm validate

  # Validate with strict mode (warnings become errors)
  $ arm validate --strict

  # Exit non-zero in CI if any errors found
  $ arm validate --ci

  # Validate only a specific subtree
  $ arm validate --scope parent-issue-id

  # Suppress INFO lines (e.g. phantom-scope notices)
  $ arm validate --quiet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _, err := materialize.MaterializeAndReturn(appCtx.IssuesDir, appCtx.StateDir, true)
			if err != nil {
				return err
			}

			opts := validate.Options{
				ScopeID:   scope,
				Strict:    strict,
				IssuesDir: appCtx.IssuesDir,
				StateDir:  appCtx.StateDir,
				RepoPath:  appCtx.RepoPath,
			}
			result := validate.Validate(state, opts)

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
				for _, e := range result.Errors {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ERROR: %s\n", e)
				}
				for _, w := range result.Warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "WARNING: %s\n", w)
				}
				if !quiet {
					for _, i := range result.Infos {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "INFO: %s\n", i)
					}
				}
				if result.Coverage != nil {
					cov := result.Coverage
					totalCited := cov.CitedNodes + cov.AcceptedRiskNodes
					if cov.AcceptedRiskNodes > 0 {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "COVERAGE: %d/%d cited (%d source-linked, %d accepted-risk)\n",
							totalCited, cov.TotalNodes, cov.CitedNodes, cov.AcceptedRiskNodes)
					} else {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "COVERAGE: %d/%d cited\n",
							totalCited, cov.TotalNodes)
					}
				}
				if result.OK {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "OK: no issues found")
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
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress INFO lines; still prints COVERAGE and OK lines")
	return cmd
}
