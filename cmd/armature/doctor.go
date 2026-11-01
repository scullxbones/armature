package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/doctor"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var strict bool
	var verbose bool
	var fix bool
	var dryRun bool
	var explain bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run repo health checks (D1-D12, D14); --fix reconciles expired claims",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			rootErr := cmd.Root().PersistentPreRunE(cmd, args)
			if rootErr == nil {
				return nil
			}

			repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}

			absRepoPath, pathErr := filepath.Abs(repoPath)
			if pathErr != nil {
				return pathErr
			}

			if layout, layoutErr := config.ResolveLayout(absRepoPath); layoutErr == nil &&
				layout.WorktreePath != "" &&
				!config.DetectUnmigratedLayout(layout.WorktreePath, layout.IssuesDir) {
				attachExecutionState(cmd, layout)
				return nil
			}

			legacyArmaturePath := filepath.Join(absRepoPath, ".armature")
			legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")

			info, statErr := os.Stat(legacyOpsPath)
			if statErr == nil && info.IsDir() {
				legacyCtx := &config.Context{
					RepoPath:  absRepoPath,
					IssuesDir: legacyArmaturePath,
					StateDir:  filepath.Join(legacyArmaturePath, "state"),
				}
				state := &executionState{ctx: legacyCtx, tracker: initPushDeps(legacyCtx)}
				baseCtx := cmd.Context()
				if baseCtx == nil {
					baseCtx = context.Background()
				}
				cmd.SetContext(context.WithValue(baseCtx, executionStateKey{}, state))
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "legacy single-branch layout detected at %s; run `arm bootstrap` to migrate to dual-branch.\n", legacyOpsPath)
				return nil
			}

			return rootErr
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			issuesDir := appCtx.IssuesDir
			repoPath := appCtx.RepoPath

			if fix {
				return runDoctorFix(cmd, appCtx, dryRun)
			}

			report, err := doctor.Run(issuesDir, appCtx.StateDir, repoPath, appCtx.WorktreePath, verbose, time.Now())
			if err != nil {
				return mapDoctorLoadError(err)
			}

			// Product caller of TreeConsistencyViolations: fold every
			// open/claimed/in-progress under a merged ancestor into D14
			// (doctor package cannot import promotion — depguard).
			store := newSnapshotStore(appCtx)
			index, indexErr := store.ReadIndex()
			if indexErr != nil {
				return mapDoctorLoadError(indexErr)
			}
			graph := promotionGraphFromStore(store, index)
			enrichD14TreeConsistency(&report, graph)

			var explainGraph promotion.Graph
			var viewer string
			if explain {
				explainGraph = graph
				// Best-effort viewer so Option B matches transition owner gate.
				// Doctor must not fail when worker-init is missing.
				if wid, _, werr := resolveWorkerAndLogCmd(cmd); werr == nil {
					viewer = wid
				}
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if isStructuredFormat(format) {
				if err := writeDoctorEnvelope(cmd.OutOrStdout(), report, explain, explainGraph, viewer); err != nil {
					return err
				}
			} else {
				renderDoctorHuman(cmd, report, verbose, explain, explainGraph, viewer)
			}

			// Doctor report is not a Command Failure (ADR 0020 §7).
			if report.HasErrors() {
				return skipCommandFailure(fmt.Errorf("doctor: %d error(s) found", countBySeverity(report, doctor.SeverityError)))
			}
			if strict && report.HasWarnings() {
				return skipCommandFailure(fmt.Errorf("doctor --strict: %d warning(s) promoted to errors", countBySeverity(report, doctor.SeverityWarning)))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&strict, "strict", false, "promote warnings to errors")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "emit file path and line context for D3 violations; name uncited issue IDs for D6")
	cmd.Flags().BoolVar(&explain, "explain", false, "guided narrative and suggested remediation for non-OK checks")
	cmd.Flags().BoolVar(&fix, "fix", false, "reconcile expired claims (claimed->open, in-progress->blocked) by appending ops")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "with --fix, list planned fixes without writing any ops")
	cmd.AddCommand(newDoctorQuarantineCmd())
	return cmd
}

func newDoctorQuarantineCmd() *cobra.Command {
	var logName string
	var line int
	cmd := &cobra.Command{
		Use:    "quarantine",
		Short:  "Move a corrupt ops log line into a sidecar without deleting it",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			opsDir := filepath.Join(appCtx.IssuesDir, "ops")
			base := filepath.Base(logName)
			if base == "." || base == string(filepath.Separator) || !strings.HasSuffix(base, ".log") {
				return armerrors.Wrap("DOCTOR-1", "quarantine --log must be an ops *.log basename", []string{"arm doctor"}, nil)
			}
			logPath := filepath.Join(opsDir, base)
			if err := ops.QuarantineLogLine(logPath, line, time.Now().Unix()); err != nil {
				return mapDoctorLoadError(err)
			}
			if gc := worktreeGit(appCtx); gc != nil && appCtx.WorktreePath != "" {
				qRel, relErr := filepath.Rel(appCtx.WorktreePath, logPath+".quarantine")
				if relErr != nil {
					return relErr
				}
				logRel, relErr := filepath.Rel(appCtx.WorktreePath, logPath)
				if relErr != nil {
					return relErr
				}
				msg := "ops: log-quarantine " + base
				if err := gc.CommitWorktreeOp(qRel, msg); err != nil {
					return err
				}
				if err := gc.CommitWorktreeOp(logRel, msg); err != nil {
					return err
				}
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "quarantined %s:%d\n", base, line)
			return nil
		},
	}
	cmd.Flags().StringVar(&logName, "log", "", "ops log basename (worker-id.log)")
	cmd.Flags().IntVar(&line, "line", 0, "1-based physical line number to quarantine")
	_ = cmd.MarkFlagRequired("log")
	_ = cmd.MarkFlagRequired("line")
	return cmd
}

func mapDoctorLoadError(err error) error {
	if err == nil {
		return nil
	}
	var located *ops.ParseLoadError
	if errors.As(err, &located) {
		if repair := located.RepairCommand(); repair != "" {
			return armerrors.Map("DOCTOR-1", located.Error(), []string{repair}, err)
		}
		return armerrors.Map("DOCTOR-1", err.Error(), []string{"arm doctor quarantine --log <file> --line <n>"}, err)
	}
	var newer *ops.NewerSchemaVersionError
	if errors.As(err, &newer) {
		return armerrors.Map("DOCTOR-1", err.Error(), []string{"arm version"}, err)
	}
	return err
}

const doctorCodesHelp = "see docs/validation-codes.md for doctor check codes"

type doctorCheckRow struct {
	Check        string          `json:"check"`
	Severity     doctor.Severity `json:"severity"`
	Message      string          `json:"message"`
	Items        []string        `json:"items,omitempty"`
	VerboseItems []string        `json:"verbose_items,omitempty"`
	Explanation  string          `json:"explanation,omitempty"`
	Suggested    []string        `json:"suggested,omitempty"`
}

func writeDoctorEnvelope(w io.Writer, report doctor.Report, explain bool, graph promotion.Graph, viewer string) error {
	rows := make([]doctorCheckRow, 0, len(report.Checks))
	for _, f := range report.Checks {
		row := doctorCheckRow{
			Check:        f.Check,
			Severity:     f.Severity,
			Message:      f.Message,
			Items:        f.Items,
			VerboseItems: f.VerboseItems,
		}
		if explain && f.Severity != doctor.SeverityOK {
			row.Explanation, row.Suggested = doctorCheckGuidanceFor(f, graph, viewer)
		}
		rows = append(rows, row)
	}
	return writeCommandEnvelope(w, "checks", rows, doctorHelp(report), nil)
}

func doctorHelp(report doctor.Report) []string {
	if !report.HasErrors() && !report.HasWarnings() {
		return []string{"all doctor checks passed", doctorCodesHelp}
	}
	return []string{doctorCodesHelp}
}

func renderDoctorHuman(cmd *cobra.Command, report doctor.Report, verbose, explain bool, graph promotion.Graph, viewer string) {
	for _, f := range report.Checks {
		icon := "✓"
		switch f.Severity {
		case doctor.SeverityWarning:
			icon = "⚠"
		case doctor.SeverityError:
			icon = "✗"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", icon, f.Check, f.Message)
		if explain && f.Severity != doctor.SeverityOK {
			explanation, suggested := doctorCheckGuidanceFor(f, graph, viewer)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Explanation: %s\n", explanation)
			for _, s := range suggested {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Suggested: %s\n", s)
			}
		}
		items := f.Items
		if verbose && len(f.VerboseItems) > 0 {
			items = f.VerboseItems
		}
		for _, item := range items {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    - %s\n", item)
		}
	}
}

// doctorCheckGuidance is the shape-agnostic coverage entry point (empty finding).
func doctorCheckGuidance(check string) (explanation string, suggested []string) {
	return doctorCheckGuidanceFor(doctor.Finding{Check: check}, nil, "")
}

func doctorCheckGuidanceFor(f doctor.Finding, graph promotion.Graph, viewer string) (explanation string, suggested []string) {
	switch f.Check {
	case "D1":
		return "Git commits reference issue IDs that are not done or merged, so history and the issue graph have drifted.",
			[]string{"arm transition <issue-id> --to done"}
	case "D2":
		return "A worker's claim TTL expired without a heartbeat, which can block other workers from taking the issue.",
			[]string{"arm doctor --fix"}
	case "D3":
		return "Ops reference issue IDs that are not in the materialized graph, usually after a revert or a truncated log.",
			[]string{"arm log --json"}
	case "D4":
		return "A child issue names a missing parent, or an issue names itself as its parent (self-parent). D5 only walks blocked_by cycles.",
			[]string{"arm reparent --issue <child-id> --parent <parent-id>"}
	case "D5":
		return "A blocked_by cycle makes every issue in the loop permanently unready.",
			[]string{"arm unlink --source <issue-id> --dep <issue-id>"}
	case "D6":
		return "Uncited issues have no documented origin, so they cannot be promoted until they cite a source or accept citation risk.",
			[]string{"arm sources link <issue-id> --source-id <source-uuid>"}
	case "D7":
		return "Ops were excluded because the log file name does not match the worker ID recorded in the op.",
			[]string{"mv .armature/ops/<expected>.log .armature/ops/<got>.log"}
	case "D8":
		return "Dirty or untracked paths sit outside the active task's declared scope.",
			[]string{"arm scope-rename or remove the stray path"}
	case "D9":
		return "A checkout lives under .worktrees/ with no issue binding, so it is an unmanaged stray worktree.",
			[]string{"arm claim --issue <issue-id> --worktree <path>; or git worktree remove --force <path>"}
	case "D10":
		return "config.json failed a strict decode or a present field is out of range.",
			[]string{"edit .armature/config.json and re-run arm doctor"}
	case "D11":
		return "origin/_armature is missing, local-only ops are stale past local_only_ops_stale_hours, or the ops remote tip could not be verified.",
			[]string{"arm push-ops"}
	case "D12":
		return "The ops worktree is behind origin/_armature so the DAG cannot " +
				"be confirmed fresh, or origin/_armature could not be fetched so " +
				"the lag number may be stale. Warnings are ignored; behind is an error.",
			[]string{"git -C .armature fetch origin _armature && git -C .armature rebase origin/_armature"}
	case "D14":
		return d14Guidance(f, graph, viewer)
	default:
		return "Doctor reported a non-OK check; see the validation codes reference for remediation.",
			[]string{"see docs/validation-codes.md"}
	}
}

const d14Explanation = "An issue is still done while its parent is already merged — a stuck " +
	"done→merged promotion — or open/claimed work sits under a merged ancestor. " +
	"Merged-under-cancelled parents are not this check."

// enrichD14TreeConsistency merges promotion.TreeConsistencyViolations into the
// D14 finding so doctor CLI reports every open/claimed under a merged ancestor
// via the same invariant claim refuses.
func enrichD14TreeConsistency(report *doctor.Report, graph promotion.Graph) {
	if report == nil || graph == nil {
		return
	}
	viol := promotion.TreeConsistencyViolations(graph)
	if len(viol) == 0 {
		return
	}
	seen := map[string]bool{}
	var idx = -1
	for i := range report.Checks {
		if report.Checks[i].Check == "D14" {
			idx = i
			for _, item := range report.Checks[i].Items {
				seen[item] = true
			}
			break
		}
	}
	if idx < 0 {
		return
	}
	f := &report.Checks[idx]
	for _, v := range viol {
		if seen[v] {
			continue
		}
		f.Items = append(f.Items, v)
		seen[v] = true
	}
	if len(f.Items) == 0 {
		return
	}
	sort.Strings(f.Items)
	f.Severity = doctor.SeverityWarning
	f.Message = "Done or open/claimed work under a merged ancestor"
}

// d14Guidance keeps the broad D14 warning and uses the shared typed Plan
// (same truth as MERGED-1 / sync). Per finding: header → Option A/B → ordered
// steps — never cross-item dedup, never alpha-sort, never "; "-joined argv.
func d14Guidance(f doctor.Finding, graph promotion.Graph, viewer string) (explanation string, suggested []string) {
	explanation = d14Explanation
	if len(f.Items) == 0 || graph == nil {
		return explanation, []string{promotion.FallbackGuidance}
	}

	var cmds []string
	var needsTreeRepair bool
	for _, item := range f.Items {
		id := promotion.FindingID(item)
		plan := promotion.RemediationsPlanFor(id, graph, viewer)
		if len(plan.Guidance) > 0 {
			needsTreeRepair = true
		}
		// No seenCmd: each finding keeps its full typed plan (grouping intact).
		cmds = append(cmds, promotion.RenderGroupedNextActions(item, plan)...)
	}
	if needsTreeRepair {
		explanation = d14Explanation + " " + promotion.TreeRepairGuidance
	}
	if len(cmds) > 0 {
		return explanation, cmds
	}
	return explanation, []string{promotion.FallbackGuidance}
}

func runDoctorFix(cmd *cobra.Command, appCtx *config.Context, dryRun bool) error {
	if err := doctorFixConfigHealth(appCtx); err != nil {
		return err
	}

	_, allIssues, err := doctor.LoadState(appCtx.IssuesDir, appCtx.StateDir)
	if err != nil {
		return err
	}

	workerID, logPath, err := resolveWorkerAndLogCmd(cmd)
	if err != nil {
		return err
	}

	actions := doctor.PlanFixes(allIssues, workerID, time.Now(), appCtx.RepoPath)

	format, _ := cmd.Root().PersistentFlags().GetString("format")

	if dryRun {
		renderDoctorFixPlan(cmd, format, actions)
		return nil
	}

	state := mustState(cmd)
	if len(actions) == 0 {
		if err := publishLocalArmatureTipAfter(state, skipAfterIntegrate()); err != nil {
			return err
		}
		renderDoctorFixPlan(cmd, format, actions)
		return nil
	}
	for _, a := range actions {
		for _, op := range a.Ops {
			if err := appendHighStakesOp(state, logPath, op, skipAfterIntegrate()); err != nil {
				return err
			}
		}
	}

	renderDoctorFixPlan(cmd, format, actions)
	return nil
}

func renderDoctorFixPlan(cmd *cobra.Command, format string, actions []doctor.FixAction) {
	if isStructuredFormat(format) {
		data := mustMarshalIndent(actions)
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return
	}
	if len(actions) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No expired claims to fix.")
	}
	for _, a := range actions {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", a.IssueID, a.Reason)
	}
}

func doctorFixConfigHealth(appCtx *config.Context) error {
	if appCtx == nil {
		return nil
	}
	if err := doctor.UnhealthyConfig(appCtx.IssuesDir); err != nil {
		return skipCommandFailure(err)
	}
	return nil
}

func countBySeverity(r doctor.Report, s doctor.Severity) int {
	n := 0
	for _, f := range r.Checks {
		if f.Severity == s {
			n++
		}
	}
	return n
}
