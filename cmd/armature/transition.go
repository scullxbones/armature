package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/scullxbones/armature/internal/git"
	"github.com/scullxbones/armature/internal/hooks"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newTransitionCmd() *cobra.Command {
	var issueID, to, outcome, branch, pr, fieldFlag string
	var force bool

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

			// Check branch discipline when transitioning to done (unless --force)
			if to == "done" && !force {
				repoPath := appCtx.RepoPath
				gc := git.New(repoPath)
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
				if uncited := isIssueUncited(issueID); uncited {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"WARNING: issue %s has no source citation.\n"+
							"Run 'arm source-link --issue %s --source-id <UUID>' to link to a source document,\n"+
							"or 'arm accept-citation --issue %s --rationale \"...\"' to accept a citation.\n"+
							"Use --force to suppress this warning.\n",
						issueID, issueID, issueID)
				}
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			issuesDir := appCtx.IssuesDir
			cfg := appCtx.Config

			// Get current issue status from materialized index and load index entries for all issues
			index, _ := materialize.LoadIndex(filepath.Join(issuesDir, "index.json"))
			currentStatus := ""
			var currentEntry *materialize.IndexEntry
			if entry, ok := index[issueID]; ok {
				currentStatus = entry.Status
				currentEntry = &entry
			}

			hookInput := hooks.HookInput{
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
				Payload:  ops.Payload{To: to, Outcome: outcome, Branch: branch, PR: pr},
			}
			if err := appendHighStakesOp(logPath, op); err != nil {
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
				data, _ := json.Marshal(result)
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
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// isIssueUncited returns true if the issue has no source-link or accept-citation.
// It loads the materialized issue from the state directory. If the issue cannot be
// loaded (e.g. not yet materialized), it returns false to avoid false positives.
func isIssueUncited(issueID string) bool {
	issuePath := filepath.Join(appCtx.StateDir, "issues", issueID+".json")
	issue, err := materialize.LoadIssue(issuePath)
	if err != nil {
		// Cannot load — graceful degradation, don't warn
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
