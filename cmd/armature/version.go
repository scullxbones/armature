package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/tui"
	"github.com/spf13/cobra"
)

type versionRow struct {
	Version string `json:"version"`
}

const versionHelp = "arm version reports the build identity of this binary"

func versionRequested(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Name() == "version" {
		return true
	}
	if cmd.Parent() != nil {
		return false
	}
	v, _ := cmd.Flags().GetBool("version")
	upper, _ := cmd.Flags().GetBool("Version")
	return v || upper
}

func writeVersionOutput(cmd *cobra.Command) error {
	flags := cmd.Root().PersistentFlags()
	format, _ := flags.GetString("format")
	explicitHuman := flags.Changed("format") && format == "human"
	if !explicitHuman && (format == "json" || format == "agent" || tui.IsNonInteractive()) {
		return writeNamedEnvelope(cmd.OutOrStdout(), "versions", []versionRow{{Version: Version}}, []string{versionHelp})
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "arm version %s\n", Version)
	return err
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print arm version",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			format, nonInteractive := autoDetectTTYPolicy(cmd)
			tui.SetFormat(format)
			tui.SetNonInteractive(nonInteractive)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeVersionOutput(cmd)
		},
	}
}
