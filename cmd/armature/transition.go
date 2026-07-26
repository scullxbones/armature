package main

import (
	"encoding/json"
	"fmt"
	"os"
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
				if currentEntry == nil {
					return fmt.Errorf("issue %s not found in materialized index (required for delivery gate check). Use --skip-delivery-gate to bypass", issueID)
				}
				// Use the literal --repo flag value (the worker's actual
				// worktree), not appCtx.RepoPath: config resolution collapses
				// a linked worktree's path to the *main* repo root (see
				// resolveParentRepoFromWorktree in internal/config/context.go)
				// so it can locate the shared armature.ops-worktree-path
				// config — but the delivery gate must run git
				// status/diff/log against the worker's own worktree, where
				// the actual dirty files, scoped diff, and commits live.
				gateRepoPath, _ := cmd.Flags().GetString("repo")
				if gateRepoPath == "" {
					gateRepoPath = "."
				}
				if err := runDeliveryGateCheck(gateRepoPath, issueID, currentEntry.Scope); err != nil {
					return err
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

// runDeliveryGateCheck runs the delivery gate checks when transitioning to done.
// It fails closed: if the worktree cannot be determined or the gate checks fail,
// it returns an error with per-check remediations.
func runDeliveryGateCheck(worktreePath string, issueID string, scope []string) error {
	if worktreePath == "" {
		return fmt.Errorf("no repo path available: cannot run delivery gate check. Use --skip-delivery-gate to bypass")
	}

	// Verify that worktreePath (the --repo flag value, or "." if unset) is
	// actually the worktree bound to issueID before checking it: without
	// this, a caller could point --repo at some other clean checkout while
	// the real claimed worktree for issueID is dirty or out-of-scope, and
	// the gate would pass by checking the wrong directory. Fail closed (like
	// the "not found in materialized index" check above) both when the
	// binding doesn't match and when no binding marker exists at all —
	// never silently allow an unbound path through.
	if err := verifyIssueWorktreeBinding(worktreePath, issueID); err != nil {
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
	baseCommit, err := dynamicBaseCommit(git)
	if err != nil {
		baseCommit, err = recordedBaseCommit(worktreePath)
		if err != nil {
			baseCommit, err = getBaseCommit(git)
			if err != nil {
				// If no candidate base branch resolves, fail closed
				return fmt.Errorf("failed to determine base commit for delivery gate check: %w. Use --skip-delivery-gate to bypass", err)
			}
		}
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
		errMsg := "delivery gate check failed:\n"
		for i, check := range failedChecks {
			errMsg += fmt.Sprintf("  %d. %s\n", i+1, check)
		}
		errMsg += "\nUse --skip-delivery-gate to override (audit trail will record the override)"
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// verifyIssueWorktreeBinding fails closed unless worktreePath is the actual
// worktree bound to issueID (the issue-ID marker file written by
// updateIssueIDFile at claim time — see harnesshook.ReadIssueBindingFileErr).
// This prevents `arm transition --to done --repo <some-other-checkout>` from
// running the delivery gate against a directory that isn't the claimed
// worktree for issueID, which would let a dirty or out-of-scope claimed
// worktree pass because the wrong directory was checked instead.
func verifyIssueWorktreeBinding(worktreePath, issueID string) error {
	gitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve git dir for %s: %w. Use --skip-delivery-gate to bypass", worktreePath, err)
	}
	binding, err := harnesshook.ReadIssueBindingFileErr(gitDir)
	if err != nil {
		return fmt.Errorf("read issue binding for %s: %w. Use --skip-delivery-gate to bypass", worktreePath, err)
	}
	if binding == "" {
		return fmt.Errorf("%s is not bound to any issue (no armature-issue-id marker found): cannot verify this is the claimed worktree for %s. Use --skip-delivery-gate to bypass", worktreePath, issueID)
	}
	if binding != issueID {
		return fmt.Errorf("%s is bound to issue %s, not %s: refusing to run delivery gate check against the wrong worktree. Use --skip-delivery-gate to bypass", worktreePath, binding, issueID)
	}
	return nil
}

// recordedBaseCommit reads the branch-point SHA persisted at claim time
// (see writeBaseCommitFileIfAbsent in claim.go) from the worktree's actual
// git directory. Returns an error if the worktree wasn't claimed after this
// mechanism was introduced, so callers can fall back to getBaseCommit.
func recordedBaseCommit(worktreePath string) (string, error) {
	actualGitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return "", fmt.Errorf("resolve worktree git dir: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(actualGitDir, baseCommitFileName)) //nolint:gosec // G304: derived from a trusted git directory
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(data))
	if sha == "" {
		return "", fmt.Errorf("base commit file is empty")
	}
	return sha, nil
}

// dynamicBaseCommit recomputes the task branch's divergence point on demand
// by merge-basing the current branch against its recorded parent branch
// (see parentBranchConfigKey / writeParentBranchConfigIfAbsent in claim.go).
// Unlike a SHA recorded once at claim time, this is recomputed fresh on
// every gate check, so it stays correct even if the task branch was rebased
// onto an updated parent tip after claim — a stale recorded SHA would
// otherwise misattribute new sibling commits pulled in by the rebase as
// in-scope diff, reintroducing the sibling-attribution bug this mechanism
// exists to prevent. Returns an error if the current branch can't be
// determined, no parent is recorded (worktrees claimed before this existed),
// or the parent ref no longer resolves.
func dynamicBaseCommit(git *adapters.Client) (string, error) {
	currentBranch, err := git.CurrentBranch()
	if err != nil || currentBranch == "" {
		return "", fmt.Errorf("determine current branch: %w", err)
	}
	parentBranch, err := git.ReadGitConfig(parentBranchConfigKey(currentBranch))
	if err != nil || parentBranch == "" {
		return "", fmt.Errorf("no recorded parent branch for %s: %w", currentBranch, err)
	}
	// A persisted literal "HEAD" is a stale record from before the
	// detached-HEAD guard existed in claim.go (see writeParentBranchConfigIfAbsent):
	// resolving the ref "HEAD" here would just mean the task branch's own tip,
	// collapsing the merge-base to the task's HEAD and making every commit
	// range for CommitReferenceCheck empty. Treat it the same as an
	// absent/empty value so old bad records self-heal by falling back to
	// recordedBaseCommit / getBaseCommit instead of silently producing a
	// wrong (empty) range.
	if parentBranch == "HEAD" {
		return "", fmt.Errorf("recorded parent branch for %s is the literal value \"HEAD\" (stale pre-fix record): treating as no usable parent branch", currentBranch)
	}
	if _, err := git.ResolveRevision(parentBranch); err != nil {
		return "", fmt.Errorf("recorded parent branch %s does not resolve: %w", parentBranch, err)
	}
	base, err := git.MergeBase(currentBranch, parentBranch)
	if err != nil {
		return "", fmt.Errorf("merge-base %s %s: %w", currentBranch, parentBranch, err)
	}
	return base, nil
}

// candidateBaseRefs are tried in order to find the branch a task diverged
// from. Remote-tracking refs are preferred over local branches: a local
// `main`/`master` in a long-lived coordinator checkout is frequently stale
// (fast-forwarded only on release), whereas `origin/main` reflects the
// actual upstream tip workers branched from.
var candidateBaseRefs = []string{"origin/main", "origin/master", "main", "master"}

// getBaseCommit finds the merge-base between HEAD and the first candidate
// base ref that resolves in this repo.
func getBaseCommit(git *adapters.Client) (string, error) {
	var lastErr error
	for _, ref := range candidateBaseRefs {
		if _, err := git.ResolveRevision(ref); err != nil {
			lastErr = err
			continue
		}
		base, err := git.MergeBase("HEAD", ref)
		if err != nil {
			lastErr = err
			continue
		}
		return base, nil
	}
	return "", fmt.Errorf("no candidate base branch (%v) resolves: %w", candidateBaseRefs, lastErr)
}
