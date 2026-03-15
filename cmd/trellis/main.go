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
	root.AddCommand(newInitCmd())
	root.AddCommand(newMaterializeCmd())
	root.AddCommand(newReadyCmd())
	root.AddCommand(newCreateCmd())
	root.AddCommand(newNoteCmd())
	root.AddCommand(newLinkCmd())
	root.AddCommand(newDecisionCmd())
	root.AddCommand(newHeartbeatCmd())
	root.AddCommand(newTransitionCmd())
	root.AddCommand(newReopenCmd())
	root.AddCommand(newMergedCmd())
	root.AddCommand(newClaimCmd())
	root.AddCommand(newRenderContextCmd())
	root.AddCommand(newValidateCmd())
	root.AddCommand(newDecomposeApplyCmd())
	root.AddCommand(newDecomposeRevertCmd())
	root.AddCommand(newDecomposeContextCmd())

	return root
}

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if debug, _ := root.PersistentFlags().GetBool("debug"); debug {
			fmt.Fprintf(os.Stderr, "DEBUG: %+v\n", err)
		}
		os.Exit(1)
	}
}
