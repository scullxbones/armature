// Package main implements the arm CLI.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/exitcodes"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func applyTTYDetectionPolicy(cmd *cobra.Command) (format string, nonInteractive bool) {
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
			format, nonInteractive := applyTTYDetectionPolicy(cmd)
			tui.SetFormat(format)
			tui.SetNonInteractive(nonInteractive)

			if versionRequested(cmd) {
				return nil
			}

			if cmd.Parent() == nil && shouldPrintRootHelp(format, nonInteractive, tui.IsTerminal()) {
				return nil
			}

			repoPath, _ := cmd.Flags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}
			ctx, err := config.ResolveContext(repoPath)
			if err != nil {
				if cmd.Parent() == nil && isAbsentArmatureLayout(repoPath, err) {
					baseCtx := cmd.Context()
					if baseCtx == nil {
						baseCtx = context.Background()
					}
					cmd.SetContext(context.WithValue(baseCtx, homeEmptyReasonKey{}, err.Error()))
					return nil
				}
				return platformProtocolError(cmd, err)
			}

			if config.DetectUnmigratedLayout(ctx.WorktreePath, ctx.IssuesDir) {
				return platformProtocolError(cmd, fmt.Errorf(
					"repo uses the pre-collapse .arm/.armature/ worktree layout; run `arm bootstrap` to migrate to the current layout",
				))
			}

			attachExecutionState(cmd, ctx)
			return nil
		},
		Args: rejectUnknownRootArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if versionRequested(cmd) {
				return writeVersionOutput(cmd)
			}
			format, _ := cmd.Flags().GetString("format")
			if shouldPrintRootHelp(format, tui.IsNonInteractive(), tui.IsTerminal()) {
				return cmd.Help()
			}
			return writeReadyHome(cmd, homeEmptyReason(cmd))
		},
	}

	root.PersistentFlags().Bool("debug", false, "dump debug diagnostics on error")
	root.PersistentFlags().String("format", "human", "output format: human, json, agent")
	root.PersistentFlags().String("repo", "", "repository path (default: current directory)")
	root.PersistentFlags().Bool("non-interactive", false, "skip TUI and emit structured output (auto-set when --format=agent or non-TTY)")
	root.Flags().BoolP("version", "v", false, "print version and exit")
	root.Flags().BoolP("Version", "V", false, "print version and exit")
	root.SetFlagErrorFunc(failLoudFlagError)

	root.AddGroup(&cobra.Group{ID: "workflow", Title: "Workflow Commands:"})
	root.AddGroup(&cobra.Group{ID: "dag", Title: "DAG Commands:"})
	root.AddGroup(&cobra.Group{ID: "sync", Title: "Sync Commands:"})
	root.AddGroup(&cobra.Group{ID: "admin", Title: "Admin Commands:"})

	for _, item := range []struct {
		group string
		cmd   *cobra.Command
	}{
		{"admin", newVersionCmd()},
		{"admin", newWorkerInitCmd()},
		{"admin", newBootstrapCmd()},
		{"workflow", newReadyCmd()},
		{"workflow", newClaimCmd()},
		{"workflow", newTransitionCmd()},
		{"workflow", newUnassignCmd()},
		{"workflow", newReopenCmd()},
		{"workflow", newHeartbeatCmd()},
		{"workflow", newNoteCmd()},
		{"workflow", newDecisionCmd()},
		{"workflow", newAmendCmd()},
		{"workflow", newConfirmCmd()},
		{"workflow", newAssignCmd()},
		{"dag", newDAGCmd()},
		{"dag", newLinkCmd()},
		{"dag", newUnlinkCmd()},
		{"sync", newSyncCmd()},
		{"sync", newPushOpsCmd()},
		{"sync", newMergedCmd()},
		{"sync", newMaterializeCmd()},
		{"sync", newImportCmd()},
		{"admin", newCreateCmd()},
		{"admin", newReparentCmd()},
		{"admin", newValidateCmd()},
		{"admin", newRenderContextCmd()},
		{"admin", newLogCmd()},
		{"admin", newStatsCmd()},
		{"admin", newWorkersCmd()},
		{"admin", newSourcesCmd()},
		{"admin", newShowCmd()},
		{"admin", newListCmd()},
		{"admin", newScopeRenameCmd()},
		{"admin", newScopeDeleteCmd()},
		{"admin", newDoctorCmd()},
		{"admin", newCompletionCmd()},
		{"admin", newHookCmd()},
		{"admin", newGateCmd()},
		{"admin", newTUICmd()},
		{"admin", newContextHistoryCmd()},
		{"admin", newContextReportCmd()},
		{"admin", newHarnessHookCmd()},
		{"admin", newReviewCmd()},
		{"admin", newWorktreeCmd()},
	} {
		item.cmd.GroupID = item.group
		root.AddCommand(item.cmd)
	}

	root.SetHelpCommandGroupID("admin")
	root.InitDefaultHelpCmd()
	applyPavedRoadMetadata(root)
	installCommandFailureMapping(root)
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
	if target == root && argvNamesPlatformProtocol(argv) {
		err = skipCommandFailure(err)
	}
	err = mapAgentFacingError(target, err)
	format, _ := applyTTYDetectionPolicy(root)
	debug, _ := root.PersistentFlags().GetBool("debug")
	return handleRootError(stdout, stderr, format, debug, err)
}

func main() {
	os.Exit(executeRoot(newRootCmd(), os.Args[1:], os.Stdout, os.Stderr))
}

const (
	pavedRoadAnnotationKey = "armature.paved-road"
	pavedRoadKindPaved     = "paved"
	pavedRoadKindEscape    = "escape-hatch"
	escapeHatchHelpMarker  = "[escape hatch]"
)

type pavedRoadClass struct {
	Kind string
	Note string
}

type pavedRoadStep struct {
	ID          string
	Title       string
	Description string
	Commands    []string
}

type pavedRoadDefault struct {
	Flag     string
	Commands []string
	Reason   string
}

var pavedRoadCommands = map[string]pavedRoadClass{
	"":     {Kind: pavedRoadKindPaved},
	"help": {Kind: pavedRoadKindPaved},

	"version":     {Kind: pavedRoadKindPaved},
	"worker-init": {Kind: pavedRoadKindPaved},
	"bootstrap":   {Kind: pavedRoadKindPaved},

	"ready":      {Kind: pavedRoadKindPaved},
	"claim":      {Kind: pavedRoadKindPaved},
	"transition": {Kind: pavedRoadKindPaved},
	"note":       {Kind: pavedRoadKindPaved},
	"decision":   {Kind: pavedRoadKindPaved},
	"unassign":   {Kind: pavedRoadKindEscape, Note: "Claim is the dispatch reservation. Unassign is recovery."},
	"reopen":     {Kind: pavedRoadKindEscape, Note: "Rework after done. The road completes once, then syncs."},
	"heartbeat":  {Kind: pavedRoadKindEscape, Note: "The harness hook heartbeats on tool use. Manual heartbeat is for long stretches with no tools."},
	"amend":      {Kind: pavedRoadKindEscape, Note: "Correct fields after create or apply. Prefer getting the plan right."},
	"confirm":    {Kind: pavedRoadKindEscape, Note: "Interactive promotion. The road uses dag transition after validate."},
	"assign":     {Kind: pavedRoadKindEscape, Note: "Soft assignment without a claim. Dispatch uses claim --worktree."},

	"dag":                  {Kind: pavedRoadKindPaved},
	"dag apply":            {Kind: pavedRoadKindPaved},
	"dag context":          {Kind: pavedRoadKindPaved},
	"dag transition":       {Kind: pavedRoadKindPaved},
	"dag revert":           {Kind: pavedRoadKindEscape, Note: "Undo a plan apply. Not part of the forward pipeline."},
	"dag summary":          {Kind: pavedRoadKindEscape, Note: "Interactive draft survey. Prefer list and dag transition."},
	"dag override-release": {Kind: pavedRoadKindEscape, Note: "Human Plan Release that skipped validate. Never a green release."},
	"link":                 {Kind: pavedRoadKindPaved},
	"unlink":               {Kind: pavedRoadKindEscape, Note: "Remove a dependency. The road adds blocked_by edges; it does not routinely delete them."},

	"sync":        {Kind: pavedRoadKindPaved},
	"push-ops":    {Kind: pavedRoadKindPaved},
	"merged":      {Kind: pavedRoadKindEscape, Note: "Manual merged promotion. Prefer sync after the PR lands."},
	"materialize": {Kind: pavedRoadKindEscape, Note: "Replay ops to rebuild state. Recovery, not the daily loop."},
	"import":      {Kind: pavedRoadKindEscape, Note: "Bulk create from CSV/JSON. Prefer dag apply from a plan."},

	"create":                  {Kind: pavedRoadKindEscape, Note: "Mint one issue by flags. Prefer dag apply."},
	"reparent":                {Kind: pavedRoadKindEscape, Note: "Move an issue in the hierarchy after the fact."},
	"validate":                {Kind: pavedRoadKindPaved},
	"validate doc-examples":   {Kind: pavedRoadKindEscape, Note: "Hidden make-check helper. Not an agent verb."},
	"render-context":          {Kind: pavedRoadKindPaved},
	"log":                     {Kind: pavedRoadKindEscape, Note: "Ops audit log. Use when diagnosing, not when dispatching."},
	"stats":                   {Kind: pavedRoadKindEscape, Note: "Derived ops-log cost view. Use when diagnosing spend, not when dispatching."},
	"workers":                 {Kind: pavedRoadKindEscape, Note: "Worker activity dump. Ready and list cover the dispatch loop."},
	"sources":                 {Kind: pavedRoadKindPaved},
	"sources add":             {Kind: pavedRoadKindPaved},
	"sources sync":            {Kind: pavedRoadKindPaved},
	"sources verify":          {Kind: pavedRoadKindEscape, Note: "Re-check cached sources. Add and sync are the road."},
	"sources link":            {Kind: pavedRoadKindEscape, Note: "Attach a source after the fact. Prefer --source at create/apply."},
	"sources accept-citation": {Kind: pavedRoadKindEscape, Note: "Record citation acceptance. Grounding at plan time is the road."},
	"sources stale-review":    {Kind: pavedRoadKindEscape, Note: "Review sources whose cache drifted. Not the daily loop."},
	"show":                    {Kind: pavedRoadKindPaved},
	"list":                    {Kind: pavedRoadKindPaved},
	"scope-rename":            {Kind: pavedRoadKindEscape, Note: "Rewrite a scope glob across issues. Planning should not need this."},
	"scope-delete":            {Kind: pavedRoadKindEscape, Note: "Drop a scope glob. Prefer amending the plan."},
	"doctor":                  {Kind: pavedRoadKindPaved},
	"completion":              {Kind: pavedRoadKindEscape, Note: "Shell completion script. Not an orchestration verb."},
	"hook":                    {Kind: pavedRoadKindEscape, Note: "Git hook management. Bootstrap --with-hooks installs them."},
	"hook run":                {Kind: pavedRoadKindEscape, Note: "Invoke a named git hook. Git and the harness call this, not agents."},
	"gate":                    {Kind: pavedRoadKindPaved},
	"gate run":                {Kind: pavedRoadKindPaved},
	"tui":                     {Kind: pavedRoadKindEscape, Note: "Interactive kanban. Agents use list, ready, and show."},
	"context-history":         {Kind: pavedRoadKindEscape, Note: "Scan git history for context changes. Diagnostic only."},
	"context-report":          {Kind: pavedRoadKindEscape, Note: "Price fixture-measured CLI payloads. Diagnostic metering, not the daily loop."},
	"harness-hook":            {Kind: pavedRoadKindEscape, Note: "Internal harness entrypoint. Hidden from --help groups."},
	"review":                  {Kind: pavedRoadKindPaved},
	"review prepare":          {Kind: pavedRoadKindPaved},
	"review record":           {Kind: pavedRoadKindPaved},
	"review commits":          {Kind: pavedRoadKindPaved},
	"review validate":         {Kind: pavedRoadKindEscape, Note: "Advisory assessment check. Record remains the enforcement gate."},
	"worktree":                {Kind: pavedRoadKindPaved},
	"worktree list":           {Kind: pavedRoadKindPaved},
	"worktree gc":             {Kind: pavedRoadKindEscape, Note: "Remove worktrees after merged/cancelled. Sync/merged teardown is the road."},
}

var pavedRoadPipeline = []pavedRoadStep{
	{
		ID:          "bootstrap",
		Title:       "Bootstrap",
		Description: "Initialize the repo, register this clone as a worker, and confirm the binary.",
		Commands:    []string{"bootstrap", "worker-init", "version"},
	},
	{
		ID:          "plan",
		Title:       "Plan / decompose",
		Description: "Register sources, render plan context, apply the plan, promote drafts, and add dependency edges.",
		Commands:    []string{"sources add", "sources sync", "dag context", "dag apply", "dag transition", "link"},
	},
	{
		ID:          "dispatch",
		Title:       "Wave dispatch",
		Description: "Survey the graph, take the ready queue, claim with a worktree, and render the worker spec.",
		Commands:    []string{"list", "doctor", "ready", "claim", "render-context", "worktree list", "show"},
	},
	{
		ID:          "work",
		Title:       "Work",
		Description: "Record progress, keep the graph green, run the configured gate, and mark the issue done.",
		Commands:    []string{"note", "decision", "validate", "gate run", "transition"},
	},
	{
		ID:          "review",
		Title:       "Review",
		Description: "Prepare the bundle, record the assessment, and list delivery commits.",
		Commands:    []string{"review prepare", "review record", "review commits"},
	},
	{
		ID:          "sync",
		Title:       "Sync",
		Description: "Publish ops, then let merged PRs promote issues.",
		Commands:    []string{"push-ops", "sync"},
	},
}

var pavedRoadDefaultsAudit = []pavedRoadDefault{
	{
		Flag:     "--skip-delivery-gate",
		Commands: []string{"transition"},
		Reason:   "Bypasses the delivery gate on --to done. The paved road runs the gate.",
	},
	{
		Flag:     "--force",
		Commands: []string{"transition"},
		Reason:   "Bypasses branch and PR discipline on --to done.",
	},
	{
		Flag:     "--force",
		Commands: []string{"claim"},
		Reason:   "Bypasses claim overlap planning.",
	},
	{
		Flag:     "--force",
		Commands: []string{"merged"},
		Reason:   "Bypasses hook-violation refusal at merge.",
	},
	{
		Flag:     "--force",
		Commands: []string{"sources accept-citation"},
		Reason:   "Skips the confirmation prompt. The paved road cites sources at plan time.",
	},
	{
		Flag:     "--strict=false",
		Commands: []string{"validate"},
		Reason:   "Keeps warnings as warnings. The paved road is fail-closed (strict by default).",
	},
	{
		Flag:     "--global",
		Commands: []string{"bootstrap"},
		Reason:   "Deploys skills outside the repo. The paved road deploys locally.",
	},
	{
		Flag:     "--raw",
		Commands: []string{"render-context"},
		Reason:   "Skips the token budget. The paved road truncates to budget.",
	},
	{
		Flag:     "--approve-all",
		Commands: []string{"dag summary"},
		Reason:   "Bulk-approves drafts. Plan release is dag transition after validate.",
	},
}

func applyPavedRoadMetadata(root *cobra.Command) {
	if root == nil {
		return
	}
	walkPavedRoadCommands(root, func(cmd *cobra.Command) {
		path := pavedRoadPath(cmd)
		class, ok := pavedRoadCommands[path]
		if !ok && cmd.Name() == "help" {
			class = pavedRoadClass{Kind: pavedRoadKindPaved}
			ok = true
		}
		if !ok {
			return
		}
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[pavedRoadAnnotationKey] = class.Kind
		if class.Kind != pavedRoadKindEscape {
			return
		}
		if strings.Contains(cmd.Short, escapeHatchHelpMarker) {
			return
		}
		if strings.TrimSpace(cmd.Short) == "" {
			cmd.Short = escapeHatchHelpMarker
			return
		}
		cmd.Short = cmd.Short + " " + escapeHatchHelpMarker
	})
}

func pavedRoadPath(cmd *cobra.Command) string {
	if cmd == nil || cmd.Parent() == nil {
		return ""
	}
	var parts []string
	for c := cmd; c.Parent() != nil; c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return strings.Join(parts, " ")
}

func walkPavedRoadCommands(root *cobra.Command, visit func(*cobra.Command)) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		visit(cmd)
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
