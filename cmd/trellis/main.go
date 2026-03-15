package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "trls",
		Short: "Trellis — git-native work orchestration",
		SilenceUsage: true,
	}

	root.PersistentFlags().Bool("debug", false, "dump debug diagnostics on error")
	root.PersistentFlags().String("format", "human", "output format: human, json, agent")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newWorkerInitCmd())

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
