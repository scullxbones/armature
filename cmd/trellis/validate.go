package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var ci bool

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the issue graph for consistency",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _, err := materialize.MaterializeAndReturn(appCtx.IssuesDir, true)
			if err != nil {
				return err
			}

			result := validate.Validate(state)

			for _, e := range result.Errors {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ERROR: %s\n", e)
			}
			for _, w := range result.Warnings {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "WARNING: %s\n", w)
			}

			if result.OK {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "OK: no issues found")
			}

			if ci && len(result.Errors) > 0 {
				return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ci, "ci", false, "exit with non-zero status if errors found")
	return cmd
}
