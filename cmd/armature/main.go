package main

import (
	"fmt"
	"os"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/exitcodes"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var appCtx *config.Context

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "arm",
		Short:        "Armature — git-native work orchestration",
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
			ctx.StateDir = stateDirFor(ctx, workerID)
			appCtx = ctx
			initPushDeps()
			return nil
		},
	}

	root.PersistentFlags().Bool("debug", false, "dump debug diagnostics on error")
	root.PersistentFlags().String("format", "human", "output format: human, json, agent")
	root.PersistentFlags().String("repo", "", "repository path (default: current directory)")
	root.PersistentFlags().Bool("non-interactive", false, "skip TUI and emit structured output (auto-set when --format=agent or non-TTY)")

	// Add command groups
	root.AddGroup(&cobra.Group{ID: "workflow", Title: "Workflow Commands:"})
	root.AddGroup(&cobra.Group{ID: "dag", Title: "DAG Commands:"})
	root.AddGroup(&cobra.Group{ID: "sync", Title: "Sync Commands:"})
	root.AddGroup(&cobra.Group{ID: "admin", Title: "Admin Commands:"})

	// Workflow commands
	versionCmd := newVersionCmd()
	versionCmd.GroupID = "admin"
	root.AddCommand(versionCmd)

	workerInitCmd := newWorkerInitCmd()
	workerInitCmd.GroupID = "admin"
	root.AddCommand(workerInitCmd)

	initCmd := newInitCmd()
	initCmd.GroupID = "admin"
	root.AddCommand(initCmd)

	readyCmd := newReadyCmd()
	readyCmd.GroupID = "workflow"
	root.AddCommand(readyCmd)

	claimCmd := newClaimCmd()
	claimCmd.GroupID = "workflow"
	root.AddCommand(claimCmd)

	transitionCmd := newTransitionCmd()
	transitionCmd.GroupID = "workflow"
	root.AddCommand(transitionCmd)

	unassignCmd := newUnassignCmd()
	unassignCmd.GroupID = "workflow"
	root.AddCommand(unassignCmd)

	reopenCmd := newReopenCmd()
	reopenCmd.GroupID = "workflow"
	root.AddCommand(reopenCmd)

	heartbeatCmd := newHeartbeatCmd()
	heartbeatCmd.GroupID = "workflow"
	root.AddCommand(heartbeatCmd)

	noteCmd := newNoteCmd()
	noteCmd.GroupID = "workflow"
	root.AddCommand(noteCmd)

	decisionCmd := newDecisionCmd()
	decisionCmd.GroupID = "workflow"
	root.AddCommand(decisionCmd)

	amendCmd := newAmendCmd()
	amendCmd.GroupID = "workflow"
	root.AddCommand(amendCmd)

	confirmCmd := newConfirmCmd()
	confirmCmd.GroupID = "workflow"
	root.AddCommand(confirmCmd)

	assignCmd := newAssignCmd()
	assignCmd.GroupID = "workflow"
	root.AddCommand(assignCmd)

	// DAG commands
	dagSummaryCmd := newDAGSummaryCmd()
	dagSummaryCmd.GroupID = "dag"
	root.AddCommand(dagSummaryCmd)

	dagTransitionCmd := newDAGTransitionCmd()
	dagTransitionCmd.GroupID = "dag"
	root.AddCommand(dagTransitionCmd)

	decomposeApplyCmd := newDecomposeApplyCmd()
	decomposeApplyCmd.GroupID = "dag"
	root.AddCommand(decomposeApplyCmd)

	decomposeRevertCmd := newDecomposeRevertCmd()
	decomposeRevertCmd.GroupID = "dag"
	root.AddCommand(decomposeRevertCmd)

	decomposeContextCmd := newDecomposeContextCmd()
	decomposeContextCmd.GroupID = "dag"
	root.AddCommand(decomposeContextCmd)

	linkCmd := newLinkCmd()
	linkCmd.GroupID = "dag"
	root.AddCommand(linkCmd)

	unlinkCmd := newUnlinkCmd()
	unlinkCmd.GroupID = "dag"
	root.AddCommand(unlinkCmd)

	// Sync commands
	syncCmd := newSyncCmd()
	syncCmd.GroupID = "sync"
	root.AddCommand(syncCmd)

	mergedCmd := newMergedCmd()
	mergedCmd.GroupID = "sync"
	root.AddCommand(mergedCmd)

	materializeCmd := newMaterializeCmd()
	materializeCmd.GroupID = "sync"
	root.AddCommand(materializeCmd)

	importCmd := newImportCmd()
	importCmd.GroupID = "sync"
	root.AddCommand(importCmd)

	staleReviewCmd := newStaleReviewCmd()
	staleReviewCmd.GroupID = "sync"
	root.AddCommand(staleReviewCmd)

	// Admin commands
	createCmd := newCreateCmd()
	createCmd.GroupID = "admin"
	root.AddCommand(createCmd)

	validateCmd := newValidateCmd()
	validateCmd.GroupID = "admin"
	root.AddCommand(validateCmd)

	renderContextCmd := newRenderContextCmd()
	renderContextCmd.GroupID = "admin"
	root.AddCommand(renderContextCmd)

	logCmd := newLogCmd()
	logCmd.GroupID = "admin"
	root.AddCommand(logCmd)

	workersCmd := newWorkersCmd()
	workersCmd.GroupID = "admin"
	root.AddCommand(workersCmd)

	sourcesCmd := newSourcesCmd()
	sourcesCmd.GroupID = "admin"
	root.AddCommand(sourcesCmd)

	sourceLinkCmd := newSourceLinkCmd()
	sourceLinkCmd.GroupID = "admin"
	root.AddCommand(sourceLinkCmd)

	acceptCitationCmd := newAcceptCitationCmd()
	acceptCitationCmd.GroupID = "admin"
	root.AddCommand(acceptCitationCmd)

	showCmd := newShowCmd()
	showCmd.GroupID = "admin"
	root.AddCommand(showCmd)

	listCmd := newListCmd()
	listCmd.GroupID = "admin"
	root.AddCommand(listCmd)

	scopeRenameCmd := newScopeRenameCmd()
	scopeRenameCmd.GroupID = "admin"
	root.AddCommand(scopeRenameCmd)

	scopeDeleteCmd := newScopeDeleteCmd()
	scopeDeleteCmd.GroupID = "admin"
	root.AddCommand(scopeDeleteCmd)

	doctorCmd := newDoctorCmd()
	doctorCmd.GroupID = "admin"
	root.AddCommand(doctorCmd)

	installSkillsCmd := newInstallSkillsCmd()
	installSkillsCmd.GroupID = "admin"
	root.AddCommand(installSkillsCmd)

	completionCmd := newCompletionCmd()
	completionCmd.GroupID = "admin"
	root.AddCommand(completionCmd)

	hookCmd := newHookCmd()
	hookCmd.GroupID = "admin"
	root.AddCommand(hookCmd)

	tuiCmd := newTUICmd()
	tuiCmd.GroupID = "admin"
	root.AddCommand(tuiCmd)

	contextHistoryCmd := newContextHistoryCmd()
	contextHistoryCmd.GroupID = "admin"
	root.AddCommand(contextHistoryCmd)

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
