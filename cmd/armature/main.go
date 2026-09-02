// Package main implements the arm CLI.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/exitcodes"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// autoDetectTTYPolicy is the single TTY-detection mechanism for the CLI (per the
// CLI Grammar Contract, docs/design/cli-grammar-contract.md § TTY detection). It reads
// the --format and --non-interactive flags off cmd's flag set (root persistent flags,
// or the flag set of a command whose PersistentPreRunE bypasses root's, such as
// bootstrap.go) and auto-upgrades them to agent/non-interactive when running non-TTY.
// No other file in cmd/armature may call tui.IsTerminal() directly; callers that need
// TTY-aware behavior go through this function instead.
func autoDetectTTYPolicy(cmd *cobra.Command) (format string, nonInteractive bool) {
	flags := cmd.Flags()

	format, _ = flags.GetString("format")
	if !flags.Changed("format") && format == "human" &&
		(os.Getenv("GEMINI_CLI") != "" || os.Getenv("TERM") == "dumb" || !tui.IsTerminal()) {
		format = "agent"
		_ = flags.Set("format", "agent")
	}

	nonInteractive, _ = flags.GetBool("non-interactive")
	if !nonInteractive && (format == "agent" || !tui.IsTerminal()) {
		nonInteractive = true
		_ = flags.Set("non-interactive", "true")
	}

	return format, nonInteractive
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "arm",
		Short:         "Armature — git-native work orchestration",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			format, nonInteractive := autoDetectTTYPolicy(cmd)
			tui.SetFormat(format)
			tui.SetNonInteractive(nonInteractive)

			repoPath, _ := cmd.Flags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}
			ctx, err := config.ResolveContext(repoPath)
			if err != nil {
				return platformProtocolError(cmd, err)
			}

			// Detect old unmigrated dual-branch layout (.arm/.armature/) and refuse
			// with clear guidance to run bootstrap. This check applies to all non-bootstrap
			// commands; bootstrap has its own PersistentPreRunE override that bypasses this.
			if config.DetectUnmigratedLayout(ctx.WorktreePath, ctx.IssuesDir) {
				return platformProtocolError(cmd, fmt.Errorf(
					"repo uses the pre-collapse .arm/.armature/ worktree layout; run `arm bootstrap` to migrate to the current layout",
				))
			}

			workerID, _ := worker.GetWorkerID(repoPath) //nolint:errcheck // best-effort; missing worker ID falls back to empty
			if workerID == "" {
				workerID = "default"
			}
			workerID = workerIdentityWithSlot(workerID)
			ctx.StateDir = stateDirFor(ctx, workerID)

			state := &executionState{ctx: ctx}
			state.pusher, state.tracker = initPushDeps(ctx)
			baseCtx := cmd.Context()
			if baseCtx == nil {
				baseCtx = context.Background()
			}
			cmd.SetContext(context.WithValue(baseCtx, executionStateKey{}, state))
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

	bootstrapCmd := newBootstrapCmd()
	bootstrapCmd.GroupID = "admin"
	root.AddCommand(bootstrapCmd)

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

	// DAG commands (group with subcommands)
	dagCmd := newDAGCmd()
	dagCmd.GroupID = "dag"
	root.AddCommand(dagCmd)

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

	pushOpsCmd := newPushOpsCmd()
	pushOpsCmd.GroupID = "sync"
	root.AddCommand(pushOpsCmd)

	mergedCmd := newMergedCmd()
	mergedCmd.GroupID = "sync"
	root.AddCommand(mergedCmd)

	materializeCmd := newMaterializeCmd()
	materializeCmd.GroupID = "sync"
	root.AddCommand(materializeCmd)

	importCmd := newImportCmd()
	importCmd.GroupID = "sync"
	root.AddCommand(importCmd)

	// Admin commands
	createCmd := newCreateCmd()
	createCmd.GroupID = "admin"
	root.AddCommand(createCmd)

	reparentCmd := newReparentCmd()
	reparentCmd.GroupID = "admin"
	root.AddCommand(reparentCmd)

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

	completionCmd := newCompletionCmd()
	completionCmd.GroupID = "admin"
	root.AddCommand(completionCmd)

	hookCmd := newHookCmd()
	hookCmd.GroupID = "admin"
	root.AddCommand(hookCmd)

	gateCmd := newGateCmd()
	gateCmd.GroupID = "admin"
	root.AddCommand(gateCmd)

	tuiCmd := newTUICmd()
	tuiCmd.GroupID = "admin"
	root.AddCommand(tuiCmd)

	contextHistoryCmd := newContextHistoryCmd()
	contextHistoryCmd.GroupID = "admin"
	root.AddCommand(contextHistoryCmd)

	harnessHookCmd := newHarnessHookCmd()
	harnessHookCmd.GroupID = "admin"
	root.AddCommand(harnessHookCmd)

	reviewCmd := newReviewCmd()
	reviewCmd.GroupID = "admin"
	root.AddCommand(reviewCmd)

	worktreeCmd := newWorktreeCmd()
	worktreeCmd.GroupID = "admin"
	root.AddCommand(worktreeCmd)

	return root
}

// executeRoot runs root and returns the process exit code. It is the single
// Execute seam shared by main and the CLI tests.
//
// Cobra reports flag-parsing and Args errors from ExecuteC *before* the root
// PersistentPreRunE runs, so at that point nothing has classified the error and
// no implicit --format has been resolved. Both decisions are therefore repeated
// here against the command cobra actually resolved:
//
//   - a `hook` or `harness-hook` failure stays on its platform protocol rather
//     than putting a Command Failure object on a stdout the platform owns;
//   - the implicit agent format is applied so a non-TTY parse failure renders
//     the promised JSON object instead of a human error line.
//
// Both are idempotent on the path where PersistentPreRunE already ran.
func executeRoot(root *cobra.Command, argv []string, stdout, stderr io.Writer) int {
	root.SetArgs(argv)
	executed, err := root.ExecuteC()
	if err == nil {
		return exitcodes.ExitSuccess.Int()
	}
	target := executed
	if target == nil {
		target = root
	}
	err = platformProtocolError(target, err)
	// When cobra could not resolve a subcommand it hands back the root command,
	// so the parent-chain walk above sees nothing to classify. Fall back to argv
	// in exactly that case; see argvNamesPlatformProtocol for why it is blunt.
	if target == root && argvNamesPlatformProtocol(argv) {
		err = skipCommandFailure(err)
	}
	format, _ := autoDetectTTYPolicy(root)
	debug, _ := root.PersistentFlags().GetBool("debug")
	return handleRootError(stdout, stderr, format, debug, err)
}

func main() {
	os.Exit(executeRoot(newRootCmd(), os.Args[1:], os.Stdout, os.Stderr))
}
