package main

import (
	"fmt"
	"os"

	"github.com/scullxbones/trellis/internal/config"
	"github.com/scullxbones/trellis/internal/tui"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var appCtx *config.Context

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "trls",
		Short:        "Trellis — git-native work orchestration",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			tui.SetFormat(format)

			repoPath, _ := cmd.Flags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}
			ctx, err := config.ResolveContext(repoPath)
			if err != nil {
				return err
			}
			appCtx = ctx
			initPushDeps()
			return nil
		},
	}

	root.PersistentFlags().Bool("debug", false, "dump debug diagnostics on error")
	root.PersistentFlags().String("format", "human", "output format: human, json, agent")
	root.PersistentFlags().String("repo", "", "repository path (default: current directory)")

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
	root.AddCommand(newStatusCmd())
	root.AddCommand(newMergedCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newClaimCmd())
	root.AddCommand(newRenderContextCmd())
	root.AddCommand(newValidateCmd())
	root.AddCommand(newDecomposeApplyCmd())
	root.AddCommand(newDecomposeRevertCmd())
	root.AddCommand(newDecomposeContextCmd())
	root.AddCommand(newLogCmd())
	root.AddCommand(newAssignCmd())
	root.AddCommand(newUnassignCmd())
	root.AddCommand(newWorkersCmd())
	root.AddCommand(newSourcesCmd())
	root.AddCommand(newDAGSummaryCmd())
	root.AddCommand(newDAGTransitionCmd())
	root.AddCommand(newImportCmd())
	root.AddCommand(newConfirmCmd())
	root.AddCommand(newStaleReviewCmd())
	root.AddCommand(newAmendCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newSourceLinkCmd())
	root.AddCommand(newAcceptCitationCmd())
	root.AddCommand(newShowCmd())
	root.AddCommand(newDoctorCmd())

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
