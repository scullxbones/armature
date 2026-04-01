package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/trellis/internal/config"
	"github.com/scullxbones/trellis/internal/exitcodes"
	"github.com/scullxbones/trellis/internal/tui"
	"github.com/scullxbones/trellis/internal/worker"
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
			if !cmd.Flags().Changed("format") && format == "human" && (os.Getenv("GEMINI_CLI") != "" || os.Getenv("TERM") == "dumb" || !tui.IsTerminal()) {
				format = "agent"
				_ = cmd.Flags().Set("format", "agent")
			}
			tui.SetFormat(format)

			// Auto-set --non-interactive when --format=agent or non-TTY.
			nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
			if !nonInteractive && (format == "agent" || !tui.IsTerminal()) {
				nonInteractive = true
				_ = cmd.Flags().Set("non-interactive", "true")
			}
			tui.SetNonInteractive(nonInteractive)

			repoPath, _ := cmd.Flags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}
			ctx, err := config.ResolveContext(repoPath)
			if err != nil {
				return err
			}
			workerID, _ := worker.GetWorkerID(repoPath)
			if workerID == "" {
				workerID = "default"
			}
			ctx.StateDir = filepath.Join(ctx.IssuesDir, "state", workerID)
			appCtx = ctx
			initPushDeps()
			return nil
		},
	}

	root.PersistentFlags().Bool("debug", false, "dump debug diagnostics on error")
	root.PersistentFlags().String("format", "human", "output format: human, json, agent")
	root.PersistentFlags().String("repo", "", "repository path (default: current directory)")
	root.PersistentFlags().Bool("non-interactive", false, "skip TUI and emit structured output (auto-set when --format=agent or non-TTY)")

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
	root.AddCommand(newTUICmd())
	root.AddCommand(newContextHistoryCmd())

	return root
}

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		code := classifyError(err)
		format, _ := root.PersistentFlags().GetString("format")
		if format == "json" || format == "agent" {
			writeJSONError(os.Stderr, err.Error(), code)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		if debug, _ := root.PersistentFlags().GetBool("debug"); debug {
			fmt.Fprintf(os.Stderr, "DEBUG: %+v\n", err)
		}
		os.Exit(code.Int())
	}
	os.Exit(exitcodes.ExitSuccess.Int())
}
