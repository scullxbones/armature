package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/docvalidate"
	"github.com/spf13/cobra"
)

func newValidateDocExamplesCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:               "validate-doc-examples",
		Short:             "Validate typed JSON examples in canonical documentation",
		Hidden:            true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := docvalidate.Validate(repo); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Documentation JSON examples are valid")
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", ".", "repository root")
	return cmd
}
