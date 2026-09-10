package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/contextreport"
	"github.com/spf13/cobra"
)

func newContextReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context-report",
		Short: "Price static agent-facing artifacts by bytes and estimated tokens",
		Long: `Enumerate embedded skills, CONTEXT.md, docs/commands.md, docs/concepts.md,
and docs/use-cases.md. Report byte size and estimated tokens using bytes/4
(integer division), matching the token_budget convention.

Dynamic structured command payloads are out of scope (AOC-S3-T2).`,
		Args: cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if cmd.Root() != nil {
				if r, err := cmd.Root().PersistentFlags().GetString("repo"); err == nil && r != "" {
					repo = r
				}
			}
			report, err := contextreport.Collect(repo)
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
