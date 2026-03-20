package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var (
		ci     bool
		strict bool
		scope  string
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the issue graph for consistency",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _, err := materialize.MaterializeAndReturn(appCtx.IssuesDir, true)
			if err != nil {
				return err
			}

			opts := validate.Options{
				ScopeID:   scope,
				Strict:    strict,
				IssuesDir: appCtx.IssuesDir,
				RepoPath:  appCtx.RepoPath,
			}
			result := validate.Validate(state, opts)

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" {
				out, err := json.MarshalIndent(map[string]interface{}{
					"errors":   result.Errors,
					"warnings": result.Warnings,
				}, "", "  ")
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
				if result.Coverage != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "COVERAGE: %.1f%% (%d/%d nodes cited)\n",
						result.Coverage.CoveragePct, result.Coverage.CitedNodes, result.Coverage.TotalNodes)
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
	return cmd
}
