package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var sourceID, dep, rel, reason string
	var overrideValidate bool

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Add a dependency link between issues",
		Long: `Create a dependency relationship between two issues.

Link one issue (source) to another (dependency) with a specified relationship type.
The only supported --rel value is blocked_by (source is blocked by dependency); the
inverse "blocks" relationship is derived automatically on the dependency and cannot
be set directly. Links establish the DAG structure and drive ready-queue eligibility.

The default path previews the post-edit Graph Finding set and refuses when the
edit would introduce findings. A recorded human override is --override-validate
--reason, which requires a controlling terminal and is never green.`,
		Example: `  # Source is blocked by another issue
  $ arm link --source E6-S4-T2 --dep E6-S4-T1 --rel blocked_by`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rel != "blocked_by" {
				return fmt.Errorf("invalid --rel value %q: only blocked_by is supported (blocks is derived automatically)", rel)
			}
			return runGraphEdit(cmd, ops.OpLink, sourceID, dep, rel, overrideValidate, reason)
		},
	}

	cmd.Flags().StringVar(&sourceID, "source", "", "source issue ID")
	cmd.Flags().StringVar(&dep, "dep", "", "dependency issue ID")
	cmd.Flags().StringVar(&rel, "rel", "blocked_by", "relationship type")
	cmd.Flags().BoolVar(&overrideValidate, "override-validate", false,
		"Human escape hatch: write the graph edit despite introduced findings. Requires --reason and a TTY. Never green.")
	cmd.Flags().StringVar(&reason, "reason", "", "Recorded reason for --override-validate")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("dep")
	return cmd
}
