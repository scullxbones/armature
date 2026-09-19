package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/deliverygate"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/spf13/cobra"
)

func hookLogContainsEntry(gitDir, kind string) (bool, error) {
	logPath := filepath.Join(gitDir, "armature-hook.log")
	data, err := os.ReadFile(logPath) //nolint:gosec // log path is derived from trusted git directory
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read hook log %s: %w", logPath, err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		_, rest, found := strings.Cut(line, " ")
		if found && strings.HasPrefix(rest, kind) {
			return true, nil
		}
	}
	return false, nil
}

func resolveBoundWorktree(worktrees []worktree.Meta, issue materialize.Issue) (worktree.Meta, worktree.Resolution, error) {
	item, res := worktree.SelectByIssue(worktrees, issue.ID, issue.WorktreePath)
	if res == worktree.Ambiguous {
		return worktree.Meta{}, res, fmt.Errorf("issue %s has ambiguous bound worktrees; disambiguate before merging", issue.ID)
	}
	return item, res, nil
}

func findGateTarget(repoPath string, issue materialize.Issue) (gitDir, binding string, found bool, err error) {
	worktrees, err := worktree.List(repoPath)
	if err != nil {
		return "", "", false, fmt.Errorf("read worktree inventory: %w", err)
	}
	item, res, err := resolveBoundWorktree(worktrees, issue)
	if err != nil {
		return "", "", false, err
	}
	worktreePath := item.Path
	if res == worktree.NotFound {
		worktreePath = unboundWorktreeOnBranch(worktrees, issue)
	}
	if worktreePath == "" {
		return "", "", false, nil
	}

	gitDir, err = worktree.ResolveGitDir(worktreePath)
	if err != nil {
		return "", "", false, fmt.Errorf("resolve worktree git dir for %s: %w", worktreePath, err)
	}
	return gitDir, harnesshook.ReadIssueBindingFile(gitDir), true, nil
}

func unboundWorktreeOnBranch(worktrees []worktree.Meta, issue materialize.Issue) string {
	branchName := materialize.DeriveBranchName(issue.Type, issue.ID)
	if branchName == "" {
		return ""
	}
	wantRef := "refs/heads/" + branchName
	for _, candidate := range worktrees {
		if candidate.Binding == "" && candidate.Branch == wantRef {
			return candidate.Path
		}
	}
	return ""
}

func issueWorktreeHasViolations(repoPath string, issue materialize.Issue) (bool, error) {
	gitDir, binding, ok, err := findGateTarget(repoPath, issue)
	if err != nil {
		return false, err
	}
	if !ok {
		if issue.Status == ops.StatusDone {
			return false, fmt.Errorf("no worktree or hook-log target exists for done issue %s", issue.ID)
		}
		return false, nil
	}
	if binding != "" && binding != issue.ID {
		return false, nil
	}
	return hookLogContainsEntry(gitDir, "violation:")
}

type worktreeRemoveOutcome int

const (
	worktreeSkipped worktreeRemoveOutcome = iota
	worktreeRemoved
)

func removeClaimExclusionAfterWorktreeRemovalLocked(repoPath, destination, pattern string) error {
	if pattern == "" {
		return nil
	}
	worktrees, err := worktree.List(repoPath)
	if err != nil {
		return fmt.Errorf("inspect worktrees before exclusion cleanup: %w", err)
	}
	want := worktree.NormalizePathAllowingMissing(destination)
	for _, item := range worktrees {
		if worktree.NormalizePathAllowingMissing(item.Path) == want {
			return fmt.Errorf("worktree at %s still exists; retaining exclusion", destination)
		}
	}
	if _, err := updateGitExcludeTrackedLocked(repoPath, "", pattern); err != nil {
		return fmt.Errorf("remove claim exclusion %q: %w", pattern, err)
	}
	return nil
}

func removeWorktreeForIssue(repoPath string, issue materialize.Issue, errWriter io.Writer) error {
	_, err := removeWorktreeForIssueTracked(repoPath, issue, errWriter)
	return err
}

func removeWorktreeForIssueTracked(repoPath string, issue materialize.Issue, errWriter io.Writer) (worktreeRemoveOutcome, error) {
	worktrees, err := worktree.List(repoPath)
	if err != nil {
		return worktreeSkipped, fmt.Errorf("read worktree inventory: %w", err)
	}
	item, res, err := resolveBoundWorktree(worktrees, issue)
	if err != nil {
		return worktreeSkipped, err
	}
	if res != worktree.Bound {
		if path := unboundWorktreeOnBranch(worktrees, issue); path != "" {
			_, _ = fmt.Fprintf(errWriter,
				"Warning: worktree at %s is on branch %s but not bound to %s; skipping removal\n",
				path, materialize.DeriveBranchName(issue.Type, issue.ID), issue.ID)
		}
		return worktreeSkipped, nil
	}
	return removeWorktreeAtPathTracked(repoPath, issue, item.Path, errWriter)
}

func removeWorktreeAtPathTracked(repoPath string, issue materialize.Issue, selectedPath string, errWriter io.Writer) (worktreeRemoveOutcome, error) {
	items, err := worktree.List(repoPath)
	if err != nil {
		return worktreeSkipped, fmt.Errorf("revalidate worktree inventory: %w", err)
	}
	var selected worktree.Meta
	found := false
	for _, item := range items {
		if worktree.NormalizePath(item.Path) == worktree.NormalizePath(selectedPath) {
			selected = item
			found = true
			break
		}
	}
	if !found {
		return worktreeSkipped, nil
	}
	if selected.Binding != issue.ID {
		_, _ = fmt.Fprintf(errWriter, "Warning: worktree at %s is now bound to %s, not %s; skipping removal\n",
			selected.Path, selected.Binding, issue.ID)
		return worktreeSkipped, nil
	}

	gitDir, err := worktree.ResolveGitDir(selected.Path)
	if err != nil {
		return worktreeSkipped, fmt.Errorf("resolve selected worktree %s: %w", selected.Path, err)
	}
	binding, err := harnesshook.ReadIssueBindingFileErr(gitDir)
	if err != nil {
		return worktreeSkipped, fmt.Errorf("revalidate issue binding for %s: %w", selected.Path, err)
	}
	if binding != issue.ID {
		_, _ = fmt.Fprintf(errWriter, "Warning: worktree at %s is bound to %s, not %s; skipping removal\n",
			selected.Path, binding, issue.ID)
		return worktreeSkipped, nil
	}
	hasPassThroughs, err := hookLogContainsEntry(gitDir, "pass-through:")
	if err != nil {
		return worktreeSkipped, fmt.Errorf("read hook log for %s: %w", issue.ID, err)
	}
	if hasPassThroughs {
		_, _ = fmt.Fprintf(errWriter, "Warning: %s has pass-through entries in armature-hook.log\n", issue.ID)
	}

	branchName, recorded, err := deliverygate.RecordedClaimedBranch(selected.Path)
	if err != nil {
		return worktreeSkipped, fmt.Errorf("read recorded claimed branch for %s: %w", issue.ID, err)
	}
	if !recorded {
		branchName = materialize.DeriveBranchName(issue.Type, issue.ID)
	}
	claimExclusionPattern, hasClaimExclusion, err := readClaimExclusionMarker(selected.Path)
	if err != nil {
		return worktreeSkipped, fmt.Errorf("read claim exclusion for %s: %w", issue.ID, err)
	}
	releaseClaimExclusionLock := func() {}
	if hasClaimExclusion {
		release, lockErr := acquireGitExcludeLock(repoPath)
		if lockErr != nil {
			return worktreeSkipped, fmt.Errorf("acquire claim exclusion lock for %s: %w", issue.ID, lockErr)
		}
		releaseClaimExclusionLock = release
	}
	gitClient := adapters.New(repoPath)

	if err := gitClient.RemoveWorktree(selected.Path); err != nil {
		releaseClaimExclusionLock()
		return worktreeSkipped, fmt.Errorf("remove worktree for %s: %w", issue.ID, err)
	}
	clearParentBranchMetadata(gitClient, branchName)
	if hasClaimExclusion {
		if err := removeClaimExclusionAfterWorktreeRemovalLocked(repoPath, selected.Path, claimExclusionPattern); err != nil {
			releaseClaimExclusionLock()
			return worktreeRemoved, err
		}
	}
	releaseClaimExclusionLock()

	return worktreeRemoved, nil
}

func newMergedCmd() *cobra.Command {
	var issueID, pr string
	var force bool

	cmd := &cobra.Command{
		Use:   "merged",
		Short: "Mark a done issue as merged after its branch/PR is merged",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)

			store := newSnapshotStore(ctx)
			index, err := store.ReadIndex()
			if err != nil {
				return fmt.Errorf("read index: %w", err)
			}

			entry, ok := index[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			if entry.Status != ops.StatusDone && entry.Status != ops.StatusMerged {
				return fmt.Errorf("issue %s is in status %q; arm merged requires status=done (transition it to done first)", issueID, entry.Status)
			}

			issue, err := store.ReadIssue(issueID)
			if err != nil {
				return fmt.Errorf("load issue %s: %w", issueID, err)
			}

			if !force {
				hasViolations, err := issueWorktreeHasViolations(ctx.RepoPath, *issue)
				if err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: cannot verify hook log for %s: %v\n", issueID, err)
					return fmt.Errorf("issue %s cannot be merged: worktree inventory unreadable (use --force to override): %w", issueID, err)
				}
				if hasViolations {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s has violation entries in armature-hook.log\n", issueID)
					return fmt.Errorf("issue %s cannot be merged: hook log contains violations (use --force to override)", issueID)
				}
			}

			alreadyMerged := entry.Status == ops.StatusMerged
			prAlreadyRecorded := alreadyMerged && issue.PR == pr

			if !alreadyMerged || (pr != "" && !prAlreadyRecorded) {
				state := mustState(cmd)
				workerID, logPath, err := resolveWorkerAndLog(state.ctx)
				if err != nil {
					return err
				}

				op := ops.Op{
					Type:      ops.OpTransition,
					TargetID:  issueID,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload:   ops.Payload{To: ops.StatusMerged, PR: pr},
				}
				if err := appendOp(state.ctx, logPath, op); err != nil {
					return err
				}
			}

			if err := removeWorktreeForIssue(ctx.RepoPath, *issue, cmd.ErrOrStderr()); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Marked %s as merged", issueID)
			if pr != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), " (PR #%s)", pr)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&pr, "pr", "", "PR number or URL")
	cmd.Flags().BoolVar(&force, "force", false, "force merge despite violations in hook log")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
