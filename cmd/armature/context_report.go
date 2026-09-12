package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/contextreport"
	"github.com/spf13/cobra"
)

func newContextReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context-report",
		Short: "Price fixture-measured main-path CLI payloads by bytes and estimated tokens",
		Long: `Measure json/agent stdout for main-path commands list, ready, show,
render-context, and review against the embedded fixture graph, plus the
fixture render-context bundle.

Tokens are bytes/4 (integer division), matching the token_budget
convention (character budget = tokens * 4). Fixtures are embedded in the
binary; --repo is not required.`,
		Args: cobra.NoArgs,
		// context-report bypasses the root PersistentPreRunE (it does not
		// need config.ResolveContext; fixtures are embedded), so it applies
		// the same --non-interactive/--format auto-detection via the shared
		// autoDetectTTYPolicy helper in main.go. That keeps direct
		// terminal-detection calls confined to main.go per the CLI Grammar
		// Contract (docs/design/cli-grammar-contract.md).
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			autoDetectTTYPolicy(cmd.Root())
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := contextreport.Collect()
			if err != nil {
				return err
			}
			format := "human"
			if cmd.Root() != nil {
				if f, err := cmd.Root().PersistentFlags().GetString("format"); err == nil && f != "" {
					format = f
				}
			}
			if format == "json" || format == "agent" {
				out, err := contextreport.RenderJSON(report)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), contextreport.RenderHuman(report))
			return nil
		},
	}
	return cmd
}
