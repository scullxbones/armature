package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/deliverygate"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/spf13/cobra"
)

func newTransitionCmd() *cobra.Command {
	var issueID, to, outcome, branch, pr, fieldFlag string
	var force, skipDeliveryGate bool

	cmd := &cobra.Command{
		Use:   "transition [issue-id]",
		Short: "Transition an issue to a new status",
		Long: `Move an issue to a new status (e.g., from in-progress to done or merged).

Valid status transitions depend on the current status and workflow rules. Provide the target
status with --to (required). You may optionally record an outcome description, branch name,
or PR number to document the completion context.

When transitioning to done, you cannot be on main/master branch unless you use --force.
This enforces branch + PR discipline.

Repeating a transition whose payload is byte-identical to the issue's current
recorded state is a no-op at exit 0: nothing is appended, and the command says
so. A same-status transition with a changed payload (for example a richer
outcome) appends as an amendment at exit 0.`,
		Example: `  # Transition an issue to done with an outcome
  $ arm transition E6-S4-T2 --to done --outcome "Implemented all required features"

  # Transition to merged and record the PR number
  $ arm transition --issue E6-S4-T2 --to merged --pr 1234

  # Override branch check with --force
  $ arm transition E6-S4-T2 --to done --outcome "..." --force`,
		Args: func(cmd *cobra.Command, args []string) error {
			return mapTransitionError(cobra.MaximumNArgs(1)(cmd, args))
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() { err = mapTransitionError(err) }()
			issueID, err = resolveIssueID(issueID, args)
			if err != nil {
				return err
			}
			if to == "" {
				return fmt.Errorf(`required flag(s) "to" not set`)
			}

			if !ops.ValidTransitionTargets[to] {
				valid := []string{}
				for s := range ops.ValidTransitionTargets {
					valid = append(valid, s)
				}
				sort.Strings(valid)
				return fmt.Errorf("invalid status %q: valid values are %v", to, valid)
			}
			if skipDeliveryGate && to != "done" {
				return fmt.Errorf("--skip-delivery-gate is only valid with --to done")
			}

			state := mustState(cmd)
			appCtx := state.ctx

			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return err
			}

			cfg := appCtx.Config

			store := newSnapshotStore(state.ctx)
			index, err := store.ReadIndex()
			swallowErr(err)
			if index == nil {
				index = make(materialize.Index)
			}
			currentStatus := ""
			var currentEntry *materialize.IndexEntry
			if entry, ok := index[issueID]; ok {
				currentStatus = entry.Status
				currentEntry = &entry
			}

			payload := ops.Payload{
				To:                  to,
				Outcome:             outcome,
				Branch:              branch,
				PR:                  pr,
				SkippedDeliveryGate: skipDeliveryGate,
			}
			liveIssue, allOps, replayErr := replayIssueOps(appCtx.IssuesDir, issueID)
			sameStatusAmendment := false
			if replayErr == nil && liveIssue != nil {
				currentStatus = liveIssue.Status
				if ops.IdenticalTransition(allOps, issueID, liveIssue.Status, liveIssue.Outcome, liveIssue.Branch, liveIssue.PR, payload) {
					if err := publishLocalArmatureTip(state); err != nil {
						return err
					}
					writeTransitionNoOp(cmd, issueID, to, fieldFlag)
					return nil
				}
				sameStatusAmendment = liveIssue.Status == to
			}

			if to == "done" && !force {
				repoPath := appCtx.RepoPath
				gc := adapters.New(repoPath)
				currentBranch, err := gc.CurrentBranch()
				if err == nil {
					if currentBranch == "main" || currentBranch == "master" {
						return fmt.Errorf("cannot transition to done while on %s branch: create a feature branch and open a PR\nUse --force to override", currentBranch)
					}
				}
			}

			if to == "done" && !force {
				if uncited := isIssueUncited(issueID, appCtx); uncited {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"WARNING: issue %s has no source citation.\n"+
							"Run 'arm sources link --issue %s --source-id <UUID>' to link to a source document,\n"+
							"or 'arm sources accept-citation --issue %s --rationale \"...\"' to accept a citation.\n"+
							"Use --force to suppress this warning.\n",
						issueID, issueID, issueID)
				}
			}

			hookInput := adapters.HookInput{
				IssueID:    issueID,
				FromStatus: currentStatus,
				ToStatus:   to,
				WorkerID:   workerID,
			}
			if err := config.RunPreTransition(&cfg, hookInput); err != nil {
				return err
			}

			if to == "done" && !skipDeliveryGate {
				gateIssue, _, err := replayIssueOps(appCtx.IssuesDir, issueID)
				if err != nil {
					return fmt.Errorf("determine current issue state for delivery gate %s: %w. Use --skip-delivery-gate to bypass", issueID, err)
				}
				gateRepoPath, _ := cmd.Flags().GetString("repo")
				if gateRepoPath == "" {
					gateRepoPath = "."
				}
				if resolved, resolveErr := deliverygate.ResolveWorktreeRoot(gateRepoPath); resolveErr == nil {
					gateRepoPath = resolved
				}

				runGate, resolvedGateRepoPath, gateErr := deliveryGateRequiredByMarkerThenTypeClaimedBy(appCtx.RepoPath, gateRepoPath, issueID, gateIssue)
				if gateErr != nil {
					return gateErr
				}
				gateRepoPath = resolvedGateRepoPath

				if runGate {
					if err := runDeliveryGateCheck(gateRepoPath, issueID, gateIssue.Type, gateIssue.ClaimedBy, gateIssue.Scope); err != nil {
						return err
					}
				}
			}

			if testBarrierAfterIdempotencyCheck != nil {
				testBarrierAfterIdempotencyCheck()
			}

			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID,
				Payload:  payload,
			}
			wrote, err := appendHighStakesOpIf(state, logPath, op, func() (bool, error) {
				live, all, replayErr := replayIssueOps(appCtx.IssuesDir, issueID)
				if replayErr != nil || live == nil {
					return true, nil
				}
				if ops.IdenticalTransition(all, issueID, live.Status, live.Outcome, live.Branch, live.PR, payload) {
					return false, nil
				}
				return true, nil
			})
			if err != nil {
				return err
			}
			if !wrote {
				writeTransitionNoOp(cmd, issueID, to, fieldFlag)
				return nil
			}

			if to == "done" && currentEntry != nil && currentEntry.Parent != "" {
				if err := checkAndWarnParentStoryStatus(index, issueID, cmd); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not check parent story status: %v\n", err)
				}
			}

			if writeTransitionFields(cmd, issueID, to, fieldFlag) {
				return nil
			}

			if sameStatusAmendment {
				writeCommandResult(cmd, map[string]any{"issue": issueID, "status": to, "amendment": true},
					"%s → %s (amendment)\n", issueID, to)
				return nil
			}
			writeCommandResult(cmd, map[string]string{"issue": issueID, "status": to},
				"%s → %s\n", issueID, to)
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&to, "to", "", "target status")
	cmd.Flags().StringVar(&outcome, "outcome", "", "outcome description")
	cmd.Flags().StringVar(&branch, "branch", "", "feature branch name")
	cmd.Flags().StringVar(&pr, "pr", "", "PR number")
	cmd.Flags().StringVar(&fieldFlag, "field", "", "comma-separated list of fields to extract (e.g., status)")
	cmd.Flags().BoolVar(&force, "force", false, "skip branch check when transitioning to done")
	cmd.Flags().BoolVar(&skipDeliveryGate, "skip-delivery-gate", false, "skip delivery gate check when transitioning to done")
	return cmd
}

var testBarrierAfterIdempotencyCheck func()

func writeTransitionNoOp(cmd *cobra.Command, issueID, to, fieldFlag string) {
	if writeTransitionFields(cmd, issueID, to, fieldFlag) {
		return
	}
	writeCommandResult(cmd, map[string]any{"issue": issueID, "status": to, "noop": true},
		"no-op: identical payload, nothing appended\n")
}

func writeTransitionFields(cmd *cobra.Command, issueID, to, fieldFlag string) bool {
	if fieldFlag == "" {
		return false
	}
	fields := extractFieldsFromIssue(&materialize.Issue{ID: issueID, Status: to}, fieldFlag)
	for _, field := range fields {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), field)
	}
	return true
}

func replayIssueOps(issuesDir, issueID string) (*materialize.Issue, []ops.Op, error) {
	allOps, err := readAllOpsFromDir(filepath.Join(issuesDir, "ops"))
	if err != nil {
		return nil, nil, fmt.Errorf("read ops: %w", err)
	}
	state, _, err := materialize.Run("", allOps, nil, materialize.Options{WriteStateFiles: false})
	if err != nil {
		return nil, allOps, fmt.Errorf("replay ops: %w", err)
	}
	issue, ok := state.Issues[issueID]
	if !ok {
		return nil, allOps, fmt.Errorf("issue not found in current ops")
	}
	return issue, allOps, nil
}

func isIssueUncited(issueID string, appCtx *config.Context) bool {
	store := newSnapshotStore(appCtx)
	issue, err := store.ReadIssue(issueID)
	if err != nil || issue == nil {
		return false
	}
	return len(issue.SourceLinks) == 0 && len(issue.CitationAcceptances) == 0
}

func checkAndWarnParentStoryStatus(index materialize.Index, currentIssueID string, cmd *cobra.Command) error {
	currentEntry, ok := index[currentIssueID]
	if !ok {
		return fmt.Errorf("current issue not found in index: %s", currentIssueID)
	}

	parentID := currentEntry.Parent
	if parentID == "" {
		return nil
	}

	parentEntry, ok := index[parentID]
	if !ok {
		return fmt.Errorf("parent issue not found: %s", parentID)
	}

	if parentEntry.Status != "in-progress" {
		return nil
	}

	allSiblingsDone := true
	for _, siblingID := range parentEntry.Children {
		siblingEntry, ok := index[siblingID]
		if !ok {
			continue
		}
		if siblingID == currentIssueID {
			continue
		}
		if siblingEntry.Status != "done" {
			allSiblingsDone = false
			break
		}
	}

	if allSiblingsDone {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n"+
			"WARNING: All tasks under %s are done but the story is still in-progress.\n"+
			"Run: arm transition %s --to done --outcome \"...\"\n\n",
			parentID, parentID)
	}

	return nil
}

func deliveryGateRequiredByMarkerThenTypeClaimedBy(repoRoot, invokingRepoPath, issueID string, gateIssue *materialize.Issue) (bool, string, error) {
	if run, path, handled := deliveryGateFromInvokingMarker(invokingRepoPath); handled {
		return run, path, nil
	}

	claimedPath, found, err := discoverIssueBoundWorktreeIndependentOfCheckout(repoRoot, issueID)
	if err != nil {
		return false, invokingRepoPath, fmt.Errorf("could not determine claimed worktree for %s: %w. Use --skip-delivery-gate to bypass", issueID, err)
	}
	if found {
		return true, claimedPath, nil
	}

	return deliveryGateFromTypeOrClaimedByAbsence(invokingRepoPath, issueID, gateIssue)
}

func deliveryGateFromInvokingMarker(invokingRepoPath string) (run bool, path string, handled bool) {
	invokingBinding, bindingErr := worktreeIssueBinding(invokingRepoPath)
	if bindingErr != nil || invokingBinding == "" {
		return false, "", false
	}
	return true, invokingRepoPath, true
}

func deliveryGateFromTypeOrClaimedByAbsence(invokingRepoPath, issueID string, gateIssue *materialize.Issue) (bool, string, error) {
	if gateIssue.ClaimedBy != "" || gateIssue.Type == "task" || gateIssue.Type == "bug" || gateIssue.Type == "feature" {
		return false, invokingRepoPath, fmt.Errorf(
			"claimed issue %s has no discoverable claimed worktree; restore or re-claim it, or use --skip-delivery-gate to bypass",
			issueID)
	}
	return false, invokingRepoPath, nil
}

func worktreeIssueBinding(worktreePath string) (string, error) {
	gitDir, err := worktree.ResolveGitDir(worktreePath)
	if err != nil {
		return "", err
	}
	return harnesshook.ReadIssueBindingFileErr(gitDir)
}

func discoverIssueBoundWorktreeIndependentOfCheckout(repoPath, issueID string) (string, bool, error) {
	items, err := worktree.List(repoPath)
	if err != nil {
		return "", false, fmt.Errorf("list worktrees: %w", err)
	}
	for _, item := range items {
		worktreePath := item.Path
		gitDir, err := worktree.ResolveGitDir(worktreePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", false, fmt.Errorf("resolve git dir for worktree %s: %w", worktreePath, err)
		}
		binding, err := harnesshook.ReadIssueBindingFileErr(gitDir)
		if err != nil {
			return "", false, fmt.Errorf("read issue binding for worktree %s: %w", worktreePath, err)
		}
		if binding == issueID {
			return worktreePath, true, nil
		}
	}
	return "", false, nil
}

func runDeliveryGateCheck(worktreePath string, issueID string, issueType string, claimedBy string, scope []string) error {
	if worktreePath == "" {
		return fmt.Errorf("no repo path available: cannot run delivery gate check. Use --skip-delivery-gate to bypass")
	}

	if err := deliverygate.VerifyIssueWorktreeBinding(worktreePath, issueID); err != nil {
		return err
	}

	if err := deliverygate.VerifyIssueBranchBinding(worktreePath, issueID, issueType, claimedBy); err != nil {
		return err
	}

	git := adapters.New(worktreePath)
	baseCommit, err := deliverygate.GatedBaseCommit(worktreePath, issueID, git)
	if err != nil {
		return fmt.Errorf("%w. Use --skip-delivery-gate to bypass", err)
	}

	result := deliverygate.DeliveryGate(worktreePath, issueID, baseCommit, scope)

	failedChecks := []string{}
	if !result.CleanTree.Pass {
		failedChecks = append(failedChecks, "CleanTree: "+result.CleanTree.Remediation)
	}
	if !result.ScopeContainment.Pass {
		failedChecks = append(failedChecks, "ScopeContainment: "+result.ScopeContainment.Remediation)
	}
	if !result.CommitReference.Pass {
		failedChecks = append(failedChecks, "CommitReference: "+result.CommitReference.Remediation)
	}

	if len(failedChecks) > 0 {
		var sb strings.Builder
		sb.WriteString("delivery gate check failed:\n")
		for i, check := range failedChecks {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, check)
		}
		sb.WriteString("\nUse --skip-delivery-gate to override (audit trail will record the override)")
		return fmt.Errorf("%s", sb.String())
	}

	return nil
}

const codeTransition1 = "TRANSITION-1"

func init() {
	armerrors.Register(codeTransition1)
}

func mapTransitionError(err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "issue ID is required"),
		strings.Contains(msg, "required flag"),
		strings.Contains(msg, "accepts at most"),
		strings.Contains(msg, "skip-delivery-gate is only valid"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm transition --help"}, err)
	case strings.Contains(msg, "invalid status"):
		return armerrors.Wrap(codeTransition1, msg, []string{"arm transition --to <valid-status>", "arm show"}, err)
	case strings.Contains(msg, "cannot transition to done"),
		strings.Contains(msg, "Use --force"):
		return armerrors.Wrap(codeTransition1, msg, []string{
			"git switch task/<issue-id>",
			"arm transition --to done --force",
		}, err)
	case strings.Contains(msg, "delivery gate"):
		return armerrors.Wrap(codeTransition1, msg, []string{"arm doctor", "arm show"}, err)
	case isLocalArmatureTipPublishError(err):
		return armerrors.Wrap(codeTransition1, msg, []string{"arm push-ops", "arm doctor"}, err)
	default:
		return armerrors.Wrap(codeTransition1, msg, []string{"arm doctor", "arm show"}, err)
	}
}
