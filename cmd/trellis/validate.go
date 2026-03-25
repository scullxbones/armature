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
				payload := map[string]interface{}{
					"errors":   result.Errors,
					"warnings": result.Warnings,
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
	return cmd
}
