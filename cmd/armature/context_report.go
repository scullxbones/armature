package main

import (
	"fmt"
	"io"

	"github.com/scullxbones/armature/internal/contextreport"
	"github.com/scullxbones/armature/internal/output"
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
		// context-report bypasses the root PersistentPreRunE (config.ResolveContext
		// would fail on a source tree that is not an Armature ops worktree), so it
		// applies the same --non-interactive/--format auto-detection via the shared
		// autoDetectTTYPolicy helper in main.go instead of a no-op. That keeps
		// direct terminal-detection calls confined to main.go per the CLI Grammar
		// Contract (docs/design/cli-grammar-contract.md).
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			autoDetectTTYPolicy(cmd.Root())
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
				return writeContextReportEnvelope(cmd.OutOrStdout(), report)
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), contextreport.RenderHuman(report))
			return nil
		},
	}
	return cmd
}

func contextReportHelp() []string {
	return []string{
		"estimated tokens = bytes/4 (integer division), matching token_budget",
		"dynamic structured command payloads are out of scope (AOC-S3-T2)",
	}
}

func writeContextReportEnvelope(w io.Writer, report contextreport.Report) error {
	artifacts := report.Artifacts
	if artifacts == nil {
		artifacts = []contextreport.Artifact{}
	}
	env, err := output.NewEnvelope("artifacts", artifacts, contextReportHelp())
	if err != nil {
		return err
	}
	if err := env.AddAdjunct("estimation_method", report.EstimationMethod); err != nil {
		return err
	}
	if err := env.AddAdjunct("total_bytes", report.TotalBytes); err != nil {
		return err
	}
	if err := env.AddAdjunct("total_estimated_tokens", report.TotalEstimatedTokens); err != nil {
		return err
	}
	return output.WriteEnvelope(w, env)
}
