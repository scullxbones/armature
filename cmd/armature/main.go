// Package main implements the arm CLI.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

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

			state := &executionState{ctx: ctx, tracker: initPushDeps(ctx)}
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

	statsCmd := newStatsCmd()
	statsCmd.GroupID = "admin"
	root.AddCommand(statsCmd)

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

	contextReportCmd := newContextReportCmd()
	contextReportCmd.GroupID = "admin"
	root.AddCommand(contextReportCmd)

	harnessHookCmd := newHarnessHookCmd()
	harnessHookCmd.GroupID = "admin"
	root.AddCommand(harnessHookCmd)

	reviewCmd := newReviewCmd()
	reviewCmd.GroupID = "admin"
	root.AddCommand(reviewCmd)

	worktreeCmd := newWorktreeCmd()
	worktreeCmd.GroupID = "admin"
	root.AddCommand(worktreeCmd)

	// Cobra lazily adds `help` in ExecuteC. Install it first so the paved-road
	// walk classifies every registered command, including help.
	root.SetHelpCommandGroupID("admin")
	root.InitDefaultHelpCmd()
	applyPavedRoadMetadata(root)
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

// Paved-road classification (NXTTN-S4-T1). Every cobra command is paved or
// escape-hatch. Metadata on the tree is the source of truth for --help
// markers and docs/paved-road.md.

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

// pavedRoadCommands classifies every registered command path (space-separated
// names, no "arm" prefix). Root is "". Cobra's generated help command is
// installed in newRootCmd before applyPavedRoadMetadata so it is classified
// here like every other registered command.
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

// pavedRoadPipeline maps Next-Ten pipeline steps onto concrete commands.
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

// pavedRoadDefaultsAudit lists flags that exist to leave the paved road.
// Skip and force flags stay labeled here until they are removed.
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

func isPavedRoadLeaf(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	for _, sub := range cmd.Commands() {
		if sub.IsAvailableCommand() {
			return false
		}
	}
	return cmd.Parent() != nil || cmd.Name() == "arm"
}

func pavedRoadKindOf(cmd *cobra.Command) string {
	if cmd == nil || cmd.Annotations == nil {
		return ""
	}
	return cmd.Annotations[pavedRoadAnnotationKey]
}

func generatePavedRoadDoc(root *cobra.Command) string {
	var b bytes.Buffer
	b.WriteString("# The Paved Road\n\n")
	b.WriteString("Generated from cobra command metadata in `cmd/armature/main.go`. Do not edit by hand.\n")
	b.WriteString("Regenerate: `UPDATE_PAVED_ROAD=1 go test ./cmd/armature -run TestPavedRoadHelpClassification_REQ_NXTTN_S4_T1`.\n\n")
	b.WriteString("## What this is\n\n")
	b.WriteString("The paved road is the one blessed end-to-end pipeline: bootstrap, plan/decompose, wave dispatch, work, review, sync.\n")
	b.WriteString("Agents should follow that pipeline. Everything else is an escape hatch.\n")
	b.WriteString("Escape-hatch commands stay in their existing `--help` groups. They are marked `[escape hatch]` inline.\n")
	b.WriteString("There is no separate escape-hatch group.\n\n")

	b.WriteString("## Pipeline\n\n")
	for i, step := range pavedRoadPipeline {
		fmt.Fprintf(&b, "### %d. %s\n\n%s\n\n", i+1, step.Title, step.Description)
		for _, path := range step.Commands {
			fmt.Fprintf(&b, "- `arm %s`\n", path)
		}
		b.WriteString("\n")
	}

	var paved []string
	var escape []string
	var unclassified []string
	walkPavedRoadCommands(root, func(cmd *cobra.Command) {
		path := pavedRoadPath(cmd)
		if path == "" {
			return
		}
		kind := pavedRoadKindOf(cmd)
		switch kind {
		case pavedRoadKindPaved:
			paved = append(paved, path)
		case pavedRoadKindEscape:
			escape = append(escape, path)
		default:
			unclassified = append(unclassified, path)
		}
	})
	sort.Strings(paved)
	sort.Strings(escape)
	sort.Strings(unclassified)

	b.WriteString("## Command classification\n\n")
	b.WriteString("Every cobra command, including leaf subcommands, is paved or escape-hatch.\n")
	b.WriteString("Zero unclassified leaf commands is a gate.\n\n")
	b.WriteString("### Paved\n\n")
	for _, path := range paved {
		step := pavedRoadStepFor(path)
		if step == "" {
			fmt.Fprintf(&b, "- `arm %s`\n", path)
			continue
		}
		fmt.Fprintf(&b, "- `arm %s` — %s\n", path, step)
	}
	b.WriteString("\n### Escape hatch\n\n")
	for _, path := range escape {
		note := pavedRoadCommands[path].Note
		if note == "" {
			fmt.Fprintf(&b, "- `arm %s`\n", path)
			continue
		}
		fmt.Fprintf(&b, "- `arm %s` — %s\n", path, note)
	}
	b.WriteString("\n### Unclassified\n\n")
	if len(unclassified) == 0 {
		b.WriteString("None. Every registered command is classified.\n\n")
	} else {
		for _, path := range unclassified {
			fmt.Fprintf(&b, "- `arm %s`\n", path)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Defaults audit\n\n")
	b.WriteString("These flags exist because the road was not always the default.\n")
	b.WriteString("Skip and force flags stay listed here. They are not the paved-road invocation.\n\n")
	b.WriteString("| Flag | Command | Why it exists |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, d := range pavedRoadDefaultsAudit {
		cmds := make([]string, 0, len(d.Commands))
		for _, c := range d.Commands {
			cmds = append(cmds, "`arm "+c+"`")
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", d.Flag, strings.Join(cmds, ", "), d.Reason)
	}
	b.WriteString("\n")
	return b.String()
}

func pavedRoadStepFor(path string) string {
	for _, step := range pavedRoadPipeline {
		for _, c := range step.Commands {
			if c == path {
				return step.Title
			}
		}
	}
	return ""
}
