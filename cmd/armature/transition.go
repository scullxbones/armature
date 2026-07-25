package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/deliverygate"
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

			// Run delivery gate check when transitioning to done (unless --skip-delivery-gate)
			if to == "done" && !skipDeliveryGate {
				if err := runDeliveryGateCheck(cmd, appCtx, issueID); err != nil {
					return err
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
func runDeliveryGateCheck(cmd *cobra.Command, appCtx *config.Context, issueID string) error {
	// Read the issue to get its scope
	store := newSnapshotStore(appCtx)
	issue, err := store.ReadIssue(issueID)
	if err != nil {
		return fmt.Errorf("failed to read issue for delivery gate check: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("issue %s not found (required for delivery gate check)", issueID)
	}

	// Get the repo path (worktree path)
	worktreePath := appCtx.RepoPath
	if worktreePath == "" {
		return fmt.Errorf("no repo path available: cannot run delivery gate check. Use --skip-delivery-gate to bypass")
	}

	// Get the base commit: use merge-base with origin/main
	git := adapters.New(worktreePath)
	baseCommit, err := getMergeBase(git, "HEAD", "origin/main")
	if err != nil {
		// If merge-base fails (e.g., origin/main doesn't exist), fail closed
		return fmt.Errorf("failed to determine base commit for delivery gate check: %w. Use --skip-delivery-gate to bypass", err)
	}

	// Run the delivery gate check
	result := deliverygate.DeliveryGate(worktreePath, issueID, baseCommit, issue.Scope)

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
		return fmt.Errorf(errMsg)
	}

	return nil
}

// getMergeBase returns the SHA of the merge-base between two revisions.
// It's a helper to get the base commit for the delivery gate check.
func getMergeBase(git *adapters.Client, rev1, rev2 string) (string, error) {
	// First try to resolve both revisions to SHAs to ensure they exist
	sha1, err := git.ResolveRevision(rev1)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", rev1, err)
	}
	sha2, err := git.ResolveRevision(rev2)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", rev2, err)
	}

	// Use the DiffNameOnlyRange method which uses three-dot notation (merge-base)
	// to get the files changed in the merge-base commit. We'll extract the base
	// by using git merge-base directly with resolved SHAs.
	// Since we can't call merge-base directly on the Client, we'll use the
	// rev-list approach: get the log starting from rev1, and stop at rev2's parent
	allCommits, err := git.LogBranch(rev1, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get commit log: %w", err)
	}

	// Find the first commit that is an ancestor of rev2
	// This is a simplified approach: iterate through commits from HEAD
	// and check if they're ancestors of rev2
	for _, entry := range allCommits {
		isAncestor, err := git.IsCommitOnBranch(entry.SHA, rev2)
		if err == nil && isAncestor {
			return entry.SHA, nil
		}
	}

	return "", fmt.Errorf("could not find merge-base between %s and %s", rev1, rev2)
}
