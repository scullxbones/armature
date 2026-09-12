package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/worker"
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
		Short: "Run repo health checks (D1-D10, D12); --fix reconciles expired claims",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Fall through to root PersistentPreRunE for normal config loading.
			// This correctly sets the execution state in the command context.
			rootErr := cmd.Root().PersistentPreRunE(cmd, args)
			if rootErr == nil {
				return nil
			}

			// Root context resolution failed; check if this is a legacy
			// single-branch layout by looking for .armature/ops in the repo root.
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
				attachDoctorExecutionState(cmd, layout)
				return nil
			}

			legacyArmaturePath := filepath.Join(absRepoPath, ".armature")
			legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")

			// Check if legacy layout exists
			info, statErr := os.Stat(legacyOpsPath)
			if statErr == nil && info.IsDir() {
				// Legacy layout detected: set up minimal execution state in the command context
				legacyCtx := &config.Context{
					RepoPath:  absRepoPath,
					IssuesDir: legacyArmaturePath,
					StateDir:  filepath.Join(legacyArmaturePath, "state"),
					// Note: WorktreePath is empty for legacy repos, which is expected
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

			// Neither modern nor legacy layout found; return the original context error
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
				return err
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if format == "json" || format == "agent" {
				if err := writeDoctorEnvelope(cmd.OutOrStdout(), report, explain); err != nil {
					return err
				}
			} else {
				renderDoctorHuman(cmd, report, verbose, explain)
			}

			// Determine exit condition. The report is already on stdout;
			// it is not a Command Failure (ADR 0020 §7).
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
	return cmd
}

const doctorCodesHelp = "see docs/validation-codes.md for doctor check codes"

type doctorCheckRow struct {
	Check        string          `json:"check"`
	Severity     doctor.Severity `json:"severity"`
	Message      string          `json:"message"`
	Items        []string        `json:"items,omitempty"`
	VerboseItems []string        `json:"verbose_items,omitempty"`
	Explanation  string          `json:"explanation,omitempty"`
	Suggested    string          `json:"suggested,omitempty"`
}

func writeDoctorEnvelope(w io.Writer, report doctor.Report, explain bool) error {
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
			row.Explanation, row.Suggested = doctorCheckGuidance(f.Check)
		}
		rows = append(rows, row)
	}
	return writeNamedEnvelope(w, "checks", rows, doctorHelp(report))
}

func doctorHelp(report doctor.Report) []string {
	if !report.HasErrors() && !report.HasWarnings() {
		return []string{"all doctor checks passed", doctorCodesHelp}
	}
	return []string{doctorCodesHelp}
}

func renderDoctorHuman(cmd *cobra.Command, report doctor.Report, verbose, explain bool) {
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
			explanation, suggested := doctorCheckGuidance(f.Check)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Explanation: %s\n", explanation)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Suggested: %s\n", suggested)
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

func doctorCheckGuidance(check string) (explanation, suggested string) {
	switch check {
	case "D1":
		return "Git commits reference issue IDs that are not done or merged, so history and the issue graph have drifted.",
			"arm transition <issue-id> --to done"
	case "D2":
		return "A worker's claim TTL expired without a heartbeat, which can block other workers from taking the issue.",
			"arm doctor --fix"
	case "D3":
		return "Ops reference issue IDs that are not in the materialized graph, usually after a revert or a truncated log.",
			"arm log --json"
	case "D4":
		return "A child issue names a parent that is not in the graph, so the hierarchy cannot be walked.",
			"arm reparent --issue <child-id> --parent <parent-id>"
	case "D5":
		return "A blocked_by cycle makes every issue in the loop permanently unready.",
			"arm unlink --source <issue-id> --dep <issue-id>"
	case "D6":
		return "Uncited issues have no documented origin, so they cannot be promoted until they cite a source or accept citation risk.",
			"arm sources link <issue-id> --source-id <source-uuid>"
	case "D7":
		return "Ops were excluded because the log file name does not match the worker ID recorded in the op.",
			"mv .armature/ops/<expected>.log .armature/ops/<got>.log"
	case "D8":
		return "Dirty or untracked paths sit outside the active task's declared scope.",
			"arm scope-rename or remove the stray path"
	case "D9":
		return "A checkout lives under .worktrees/ with no issue binding, so it is an unmanaged stray worktree.",
			"arm claim --issue <issue-id> --worktree <path>; or git worktree remove --force <path>"
	case "D10":
		return "config.json failed a strict decode or a present field is out of range.",
			"edit .armature/config.json and re-run arm doctor"
	case "D12":
		return "The ops worktree is behind origin/_armature, so this clone's coordination state is stale relative to the published ops branch.",
			"git -C .armature fetch origin _armature && git -C .armature rebase origin/_armature"
	default:
		return "Doctor reported a non-OK check; see the validation codes reference for remediation.",
			"see docs/validation-codes.md"
	}
}

// runDoctorFix plans and (unless dryRun) applies the deterministic claim-liveness
// remediations described in docs/design/recovery-state-machine.md.
func runDoctorFix(cmd *cobra.Command, appCtx *config.Context, dryRun bool) error {
	if err := doctorFixConfigHealth(appCtx); err != nil {
		return err
	}

	_, allIssues, err := doctor.LoadState(appCtx.IssuesDir, appCtx.StateDir)
	if err != nil {
		return err
	}

	workerID, logPath, err := resolveWorkerAndLog(appCtx)
	if err != nil {
		return err
	}

	actions := doctor.PlanFixes(allIssues, workerID, time.Now(), appCtx.RepoPath)

	format, _ := cmd.Root().PersistentFlags().GetString("format")

	if dryRun || len(actions) == 0 {
		renderDoctorFixPlan(cmd, format, actions)
		return nil
	}

	// Append via the same high-stakes path claim/transition/assign use: commit
	// (dual-branch) and push (best-effort) each op immediately.
	//
	// The plan is withheld until every op lands. ADR 0020 §7 exempts doctor
	// checks, not a failure while mutating state: an apply error is a real
	// Command Failure, and printing the plan first would both concatenate a
	// second value onto stdout and describe ops that were never attempted.
	state := mustState(cmd)
	for _, a := range actions {
		for _, op := range a.Ops {
			if err := appendHighStakesOp(state, logPath, op); err != nil {
				return err
			}
		}
	}

	renderDoctorFixPlan(cmd, format, actions)
	return nil
}

// renderDoctorFixPlan writes the fix plan to stdout in the caller's format.
// Callers invoke it for a dry run or after every op has been applied, so the
// plan on stdout always describes work that was planned only or done in full.
func renderDoctorFixPlan(cmd *cobra.Command, format string, actions []doctor.FixAction) {
	if format == "json" || format == "agent" {
		data, _ := json.MarshalIndent(actions, "", "  ") //nolint:errcheck // actions struct contains only serializable values
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

func attachDoctorExecutionState(cmd *cobra.Command, ctx *config.Context) {
	workerID, _ := worker.GetWorkerID(ctx.RepoPath) //nolint:errcheck // best-effort; missing worker ID falls back to empty
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
}

func doctorFixConfigHealth(appCtx *config.Context) error {
	if appCtx == nil {
		return nil
	}
	health := doctor.CheckD10ConfigHealth(filepath.Join(appCtx.IssuesDir, "config.json"))
	if health.Severity != doctor.SeverityError {
		return nil
	}
	msg := health.Message
	if len(health.Items) > 0 {
		msg = msg + ": " + strings.Join(health.Items, "; ")
	}
	return skipCommandFailure(fmt.Errorf("doctor: %s: %s", health.Check, msg))
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
