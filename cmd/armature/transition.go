package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/deliverygate"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/hooks"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
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
This enforces branch + PR discipline.`,
		Example: `  # Transition an issue to done with an outcome
  $ arm transition E6-S4-T2 --to done --outcome "Implemented all required features"

  # Transition to merged and record the PR number
  $ arm transition --issue E6-S4-T2 --to merged --pr 1234

  # Override branch check with --force
  $ arm transition E6-S4-T2 --to done --outcome "..." --force`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
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

			// Check branch discipline when transitioning to done (unless --force)
			if to == "done" && !force {
				repoPath := appCtx.RepoPath
				gc := adapters.New(repoPath)
				currentBranch, err := gc.CurrentBranch()
				if err == nil {
					// Only reject if we successfully detected we're on main/master
					if currentBranch == "main" || currentBranch == "master" {
						return fmt.Errorf("cannot transition to done while on %s branch: create a feature branch and open a PR\nUse --force to override", currentBranch)
					}
				}
				// If we can't determine the branch, allow the transition (graceful degradation)
			}

			// Warn if transitioning to done and issue has no source-link or accept-citation (unless --force)
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

			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return err
			}

			cfg := appCtx.Config

			// Get current issue status from materialized index and load index entries for all issues
			store := newSnapshotStore(state.ctx)
			// A load error degrades gracefully to an empty index, matching the previous
			// behavior of silently ignoring missing-file errors from LoadIndex.
			index, _ := store.ReadIndex() //nolint:errcheck // missing index treated as empty; access uses ok-check
			if index == nil {
				index = make(materialize.Index)
			}
			currentStatus := ""
			var currentEntry *materialize.IndexEntry
			if entry, ok := index[issueID]; ok {
				currentStatus = entry.Status
				currentEntry = &entry
			}

			hookInput := adapters.HookInput{
				IssueID:    issueID,
				FromStatus: currentStatus,
				ToStatus:   to,
				WorkerID:   workerID,
			}
			if err := hooks.RunPreTransition(&cfg, hookInput); err != nil {
				return err
			}

			// Run delivery gate check when transitioning to done (unless
			// --skip-delivery-gate). This runs AFTER pre-transition hooks so it
			// validates the actual final state of the worktree: a hook (formatter,
			// code generator, test run) can modify files or leave artifacts, and
			// the gate must catch a resulting dirty tree or out-of-scope file
			// rather than checking a state that hooks are about to change.
			if to == "done" && !skipDeliveryGate {
				gateIssue, err := currentIssueFromOps(appCtx.IssuesDir, issueID)
				if err != nil {
					return fmt.Errorf("determine current issue state for delivery gate %s: %w. Use --skip-delivery-gate to bypass", issueID, err)
				}
				// The delivery gate validates a claimed issue's own worktree
				// binding, scope, and commits, so it applies to every issue
				// kind that claim actually binds to a worker's own worktree —
				// task, bug, feature, and (when actually claimed) story (see
				// internal/materialize/branch.go's worktree-branch mapping and
				// cmd/armature/claim.go's createWorktreeAndBranch, which all
				// four issue kinds go through). Epics never receive a worktree
				// at all (DeriveBranchName returns "" for them) and are
				// transitioned to done from a manually created/checked-out
				// branch outside the claimed-worktree workflow, so the gate
				// must not run for them (not even require
				// --skip-delivery-gate). Stories are dual-mode: claim.go also
				// supports claiming a story directly into its own worktree,
				// but the coordinator additionally transitions *unclaimed*
				// stories to done after the story-level auditor gate, from
				// its own checkout with no worktree binding at all — that
				// path must stay exempt too. Resolve the gate path first so
				// a cheap worktree-binding probe can tell an unclaimed story
				// apart from a claimed one before deciding whether to run
				// the full gate.
				//
				// Use the literal --repo flag value (the worker's actual
				// worktree), not appCtx.RepoPath: config resolution collapses
				// a linked worktree's path to the *main* repo root (see
				// resolveParentRepoFromWorktree in internal/config/context.go)
				// so it can locate the shared armature.ops-worktree-path
				// config -- but the delivery gate must run git
				// status/diff/log against the worker's own worktree, where
				// the actual dirty files, scoped diff, and commits live.
				gateRepoPath, _ := cmd.Flags().GetString("repo")
				if gateRepoPath == "" {
					gateRepoPath = "."
				}
				// Resolve to the worktree's top level before gating:
				// ResolveWorktreeGitDir (used by VerifyIssueWorktreeBinding
				// and friends) stats "<gateRepoPath>/.git" with no walk-up,
				// so the default "." fails with "stat .git" when arm
				// transition is invoked from a subdirectory of the worktree
				// rather than its root. git itself resolves this via
				// rev-parse --show-toplevel; if that fails (e.g. not
				// actually inside a git worktree), fall back to the
				// original value so the existing checks fail closed with
				// their normal error instead of a confusing new one here.
				if resolved, resolveErr := deliverygate.ResolveWorktreeRoot(gateRepoPath); resolveErr == nil {
					gateRepoPath = resolved
				}

				// A claim is authoritative evidence that this issue entered the
				// bound-worktree workflow. An amend can change its type without
				// clearing that claim, so type alone must never turn a live bound
				// worktree into a delivery-gate exemption.
				runGate := gateIssue.ClaimedBy != "" || gateIssue.Type == "task" || gateIssue.Type == "bug" || gateIssue.Type == "feature"
				if gateIssue.Type == "story" {
					if gateIssue.ClaimedBy != "" {
						if isClaimedWorktreeForIssue(gateRepoPath, issueID) {
							runGate = true
						} else {
							claimedPath, found, resolveErr := resolveClaimedStoryWorktree(appCtx.RepoPath, issueID)
							if resolveErr != nil {
								// Fail closed: we couldn't reliably determine
								// whether some other worktree is claimed for this
								// story (git worktree list failed, or a marker
								// file existed but couldn't be read for a reason
								// other than "missing"). Refuse the transition
								// rather than silently treating it as unclaimed —
								// see finding 2 in the Opus review of commit
								// 1ac1b2e5.
								return fmt.Errorf("could not determine claimed worktree for story %s: %w. Use --skip-delivery-gate to bypass", issueID, resolveErr)
							}
							if found {
								// The invoking checkout's own marker didn't match
								// (isClaimedWorktreeForIssue above), but a
								// repo-global scan found a *different* worktree
								// actually claimed for this story issue (see
								// docs/dogfood/findings/raw/2026-08-02T1600Z-
								// claude-workflow-story-gate-bypass-via-wrong-checkout.md).
								// Fail closed: run the gate against that claimed
								// worktree, not the invoking checkout, so a dirty
								// or out-of-scope claimed worktree can't slip a
								// "done" through by transitioning from elsewhere.
								runGate = true
								gateRepoPath = claimedPath
							} else {
								// No live marker is discoverable, but the authoritative ops
								// still record a claimant. That means this story entered
								// the claimed-worktree workflow and its worktree was likely
								// removed or pruned manually. Do not confuse that state with
								// an unclaimed coordinator-level story: the delivery gate
								// cannot validate a missing bound worktree, so fail closed.
								return fmt.Errorf("claimed story %s has no discoverable claimed worktree; restore or re-claim it, or use --skip-delivery-gate to bypass", issueID)
							}
						}
					}
				}

				if runGate {
					if err := runDeliveryGateCheck(gateRepoPath, issueID, gateIssue.Type, gateIssue.Scope); err != nil {
						return err
					}
				}
			}

			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID,
				Payload: ops.Payload{
					To:                  to,
					Outcome:             outcome,
					Branch:              branch,
					PR:                  pr,
					SkippedDeliveryGate: skipDeliveryGate,
				},
			}
			if err := appendHighStakesOp(state, logPath, op); err != nil {
				return err
			}

			// After successful transition, check if we should warn about parent story
			if to == "done" && currentEntry != nil && currentEntry.Parent != "" {
				if err := checkAndWarnParentStoryStatus(index, issueID, cmd); err != nil {
					// Log the error but don't fail the transition
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not check parent story status: %v\n", err)
				}
			}

			// If --field flag is set, extract and print only the requested field
			if fieldFlag != "" {
				// Create a minimal issue object with just the transition result
				// to extract the field from
				transitionResult := &materialize.Issue{
					ID:     issueID,
					Status: to,
				}
				fields := extractFieldsFromIssue(transitionResult, fieldFlag)
				for _, field := range fields {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), field)
				}
				return nil
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "status": to}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s → %s\n", issueID, to)
			}
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
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// currentIssueFromOps reads the append-only source of truth without updating
// derived state. Delivery-gate decisions must not rely on a stale snapshot:
// amend, unassign, and transition --to open append authoritative state changes
// but do not synchronously materialize them.
func currentIssueFromOps(issuesDir, issueID string) (*materialize.Issue, error) {
	allOps, _, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
	if err != nil {
		return nil, fmt.Errorf("read ops: %w", err)
	}
	state, _, err := materialize.Run("", allOps, nil, materialize.Options{WriteStateFiles: false})
	if err != nil {
		return nil, fmt.Errorf("replay ops: %w", err)
	}
	issue, ok := state.Issues[issueID]
	if !ok {
		return nil, fmt.Errorf("issue not found in current ops")
	}
	return issue, nil
}

// isIssueUncited returns true if the issue has no source-link or accept-citation.
// It reads the materialized issue directly from disk without triggering rematerialization.
// If the issue cannot be read (e.g. not yet materialized), it returns false to avoid false positives.
func isIssueUncited(issueID string, appCtx *config.Context) bool {
	store := newSnapshotStore(appCtx)
	// A read error degrades gracefully: treat the issue as not uncited, matching the previous
	// behavior of ignoring missing-file errors when the issue file was absent.
	issue, err := store.ReadIssue(issueID)
	if err != nil || issue == nil {
		// Cannot read — graceful degradation, don't warn
		return false
	}
	return len(issue.SourceLinks) == 0 && len(issue.CitationAcceptances) == 0
}

// checkAndWarnParentStoryStatus checks if all sibling tasks will be done after this transition,
// and the parent is still in-progress. Prints a warning to stderr if so.
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

	// Check if parent is still in-progress
	if parentEntry.Status != "in-progress" {
		return nil
	}

	// Check if all siblings are done or will be done after this transition.
	// We assume the current issue is transitioning to done.
	allSiblingsDone := true
	for _, siblingID := range parentEntry.Children {
		siblingEntry, ok := index[siblingID]
		if !ok {
			continue
		}
		// If this is the current issue being transitioned, assume it will be done
		if siblingID == currentIssueID {
			continue
		}
		// Otherwise, check if the sibling is already done
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

// isClaimedWorktreeForIssue reports whether worktreePath is a worktree
// actually claimed for issueID (the armature-issue-id marker file written by
// claim at claim time — same mechanism deliverygate.VerifyIssueWorktreeBinding
// enforces once the gate is known to apply). Used only to decide whether a
// story transition should be gated at all: an unclaimed coordinator-level
// story transition (no marker, or a marker for a different issue) must stay
// exempt, so any resolution failure or non-matching binding here is treated
// as "not claimed" rather than an error — the caller falls back to skipping
// the gate exactly as it always has for stories.
func isClaimedWorktreeForIssue(worktreePath, issueID string) bool {
	gitDir, err := deliverygate.ResolveWorktreeGitDir(worktreePath)
	if err != nil {
		return false
	}
	binding, err := harnesshook.ReadIssueBindingFileErr(gitDir)
	if err != nil || binding == "" {
		return false
	}
	return binding == issueID
}

// resolveClaimedStoryWorktree finds a worktree actually claimed for a story
// issue, regardless of which checkout `arm transition` is invoked from and
// regardless of what branch that worktree currently has checked out. It
// enumerates EVERY worktree linked to the repository via `git worktree list
// --porcelain` (listAllWorktreePaths) and checks each one's own
// armature-issue-id marker file directly, rather than first narrowing to
// "whichever worktree currently has refs/heads/feat/<id> checked out" the
// way findWorktreePathByBranch does. Branch-name lookup is not enough: a
// claimed worktree left in a detached HEAD (mid-rebase, mid-bisect) or
// checked out to an unrelated scratch branch has no worktree whose HEAD
// matches the story branch, so a branch-first scan would miss it entirely
// and the gate would silently skip — exactly the gap described in
// docs/dogfood/findings/raw/2026-08-02T1600Z-claude-workflow-story-gate-bypass-via-wrong-checkout.md
// and confirmed still exploitable against the branch-first version of this
// function by a follow-up review of commit 1ac1b2e5. Only the marker file,
// never the checked-out branch, decides whether a worktree is "claimed for
// issueID" here.
//
// Returns ("", false, nil) when no worktree's marker matches issueID —
// callers combine this with isClaimedWorktreeForIssue's own check of the
// invoking checkout, and both together are the only avenue for treating the
// story as unclaimed (a legitimate, ungated coordinator-level transition).
// Returns a non-nil error when the scan itself could not be trusted: `git
// worktree list` failing to execute, a worktree's git dir failing to resolve
// for a reason other than the worktree directory simply not existing
// (stale/prunable entries are skipped, not treated as errors), or a marker
// file existing but failing to read for a reason other than "missing" (e.g.
// permission denied, corrupt file) — see harnesshook.ReadIssueBindingFileErr,
// which already distinguishes "missing" (legitimately unclaimed) from "read
// failed" (must not be silently treated as unclaimed) for exactly this
// reason. Callers MUST fail the transition closed on a non-nil error rather
// than falling through to "not claimed", per the armature constitution's I5
// (deterministic gates decide, never silently skip).
func resolveClaimedStoryWorktree(repoPath, issueID string) (string, bool, error) {
	paths, err := listAllWorktreePaths(repoPath)
	if err != nil {
		return "", false, fmt.Errorf("list worktrees: %w", err)
	}
	for _, worktreePath := range paths {
		gitDir, err := resolveWorktreeGitDir(worktreePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Worktree directory no longer exists (stale/prunable
				// `git worktree list` entry) — nothing to check here.
				continue
			}
			return "", false, fmt.Errorf("resolve git dir for worktree %s: %w", worktreePath, err)
		}
		binding, err := harnesshook.ReadIssueBindingFileErr(gitDir)
		if err != nil {
			// A marker file existed but couldn't be read for a reason other
			// than "missing" — fail closed rather than silently skipping
			// this worktree as if it were unclaimed.
			return "", false, fmt.Errorf("read issue binding for worktree %s: %w", worktreePath, err)
		}
		if binding == issueID {
			return worktreePath, true, nil
		}
	}
	return "", false, nil
}

// listAllWorktreePaths returns the path of every worktree git knows about for
// the repository at repoPath (main worktree included), regardless of what
// branch each one currently has checked out. Used by
// resolveClaimedStoryWorktree, which must find a claimed worktree by its own
// armature-issue-id marker independent of branch state.
func listAllWorktreePaths(repoPath string) ([]string, error) {
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list --porcelain: %w", err)
	}

	var paths []string
	for _, line := range strings.Split(string(output), "\n") {
		if rest, ok := strings.CutPrefix(line, "worktree "); ok {
			paths = append(paths, rest)
		}
	}
	return paths, nil
}

// runDeliveryGateCheck runs the delivery gate checks when transitioning to done.
// It fails closed: if the worktree cannot be determined or the gate checks fail,
// it returns an error with per-check remediations.
func runDeliveryGateCheck(worktreePath string, issueID string, issueType string, scope []string) error {
	if worktreePath == "" {
		return fmt.Errorf("no repo path available: cannot run delivery gate check. Use --skip-delivery-gate to bypass")
	}

	// Verify that worktreePath (the --repo flag value, or "." if unset) is
	// actually the worktree bound to issueID before checking it: without
	// this, a caller could point --repo at some other clean checkout while
	// the real claimed worktree for issueID is dirty or out-of-scope, and
	// the gate would pass by checking the wrong directory. Fail closed both when the
	// binding doesn't match and when no binding marker exists at all —
	// never silently allow an unbound path through.
	if err := deliverygate.VerifyIssueWorktreeBinding(worktreePath, issueID); err != nil {
		return err
	}

	// Verify the worktree's actual current branch (HEAD) is the expected
	// task branch for this issue, not merely that the armature-issue-id
	// marker file matches. The marker file persists in .git across a `git
	// checkout` of an unrelated scratch branch, so without this check a
	// worker could claim, then check out some other branch, commit clean
	// correctly-scoped changes there, and pass the gate even though the
	// actual task/<issueID> branch the coordinator will integrate never
	// received the commit.
	if err := deliverygate.VerifyIssueBranchBinding(worktreePath, issueID, issueType); err != nil {
		return err
	}

	// Get the base commit. Prefer dynamically recomputing merge-base against
	// the recorded parent branch (git config, shared across worktrees — see
	// parentBranchConfigKey): this self-corrects both if the worktree was
	// removed and recreated (config survives that; a per-worktree file would
	// not) and if the task branch was later rebased onto an updated parent
	// tip (merge-base recomputed fresh each time, so it never goes stale).
	// Fall back to the SHA recorded once at claim time (worktrees claimed
	// before the parent-branch config was introduced), then to merge-base
	// against a default branch.
	git := adapters.New(worktreePath)
	baseCommit, err := deliverygate.ResolveBaseCommit(worktreePath, git)
	if err != nil {
		// If no candidate base branch resolves, fail closed
		return fmt.Errorf("%w. Use --skip-delivery-gate to bypass", err)
	}

	// Run the delivery gate check
	result := deliverygate.DeliveryGate(worktreePath, issueID, baseCommit, scope)

	// Collect failed checks with their remediations
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

	// If any check failed, return an error
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

// verifyIssueWorktreeBinding, verifyIssueBranchBinding, recordedBaseCommit,
// dynamicBaseCommit, candidateBaseRefs, and getBaseCommit have moved to
// internal/deliverygate (basecommit.go) as VerifyIssueWorktreeBinding,
// VerifyIssueBranchBinding, RecordedBaseCommit, DynamicBaseCommit,
// CandidateBaseRefs, and GetBaseCommit, so this read-side base-commit
// resolution logic is independently unit-testable via package import
// instead of living unexported in this cmd/armature (main package) file.
// candidateBaseRefs is kept here as a thin alias because claim.go's
// existing-worktree claim path also consults it when proving a worktree
// hasn't diverged before persisting branch-point metadata.
var candidateBaseRefs = deliverygate.CandidateBaseRefs
