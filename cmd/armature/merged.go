package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/spf13/cobra"
)

// hookLogContainsEntry reads <git-dir>/armature-hook.log and reports whether any
// line contains an entry of the given kind (e.g. "violation:", "pass-through:").
// Entries are matched at the start of the line body (immediately after the
// RFC3339 timestamp), so injected newlines inside logged fields cannot forge
// entries mid-line (finding 8; fields are also sanitized at write time).
func hookLogContainsEntry(gitDir, kind string) bool {
	logPath := filepath.Join(gitDir, "armature-hook.log")
	data, err := os.ReadFile(logPath) //nolint:gosec // log path is derived from trusted git directory
	if err != nil {
		// Log doesn't exist or can't be read; no entries found
		return false
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		// Each entry is "<RFC3339 timestamp> <kind> ...": strip the timestamp
		// (first space-delimited token) and match the kind at line-body start.
		_, rest, found := strings.Cut(line, " ")
		if found && strings.HasPrefix(rest, kind) {
			return true
		}
	}
	return false
}

// readHookLogForPassThroughs reports whether the hook log contains pass-through entries.
func readHookLogForPassThroughs(gitDir string) bool {
	return hookLogContainsEntry(gitDir, "pass-through:")
}

// readHookLogForViolations reports whether the hook log contains violation entries.
func readHookLogForViolations(gitDir string) bool {
	return hookLogContainsEntry(gitDir, "violation:")
}

// resolveIssueWorktree locates the worktree for an issue's branch and resolves its
// git dir and binding. Returns ok=false, nil err ONLY when there is genuinely no
// bound worktree to gate on (CountByIssue==0 and no legacy branch fallback).
// A non-nil err means either the worktree inventory (or the found worktree's git
// dir) could not be read, OR the marker-bound set is AMBIGUOUS (CountByIssue>0 but
// SelectByIssue cannot uniquely resolve a target — e.g. duplicate legacy+canonical
// markers). In both error cases callers MUST fail closed rather than treat the
// worktree as "not found / nothing to gate", or `arm merged` would append the
// merged op and exit 0 while a worktree that may contain violations is never
// inspected (I5 deterministic gates fail closed; I6 done≠merged). binding is the
// content of armature-issue-id ("" when absent). Shared by the merged violation
// gate and worktree removal (finding 7).
func resolveIssueWorktree(repoPath string, issue materialize.Issue) (worktreePath, gitDir, binding string, ok bool, err error) {
	worktrees, err := worktree.List(repoPath)
	if err != nil {
		return "", "", "", false, fmt.Errorf("read worktree inventory: %w", err)
	}
	// Marker identity is authoritative. This also handles a detached worktree
	// and prevents an unrelated worktree holding the expected branch from being
	// selected ahead of the actually bound worktree. When more than one worktree
	// shares the issue's marker (e.g. a legacy explicit-path worktree alongside
	// the canonical .worktrees/<id> one), apply the WorktreePath-first precedence
	// used everywhere else (reconcile.go's selectGCRemoval, removeWorktreeAtPathTracked)
	// and FAIL CLOSED on ambiguity rather than force-removing/inspecting the wrong
	// worktree — the merged violation-gate and removal path force-remove (--force),
	// so guessing here could discard uncommitted changes in the wrong worktree.
	if worktree.CountByIssue(worktrees, issue.ID) > 0 {
		item, selectOK := worktree.SelectByIssue(worktrees, issue.ID, issue.WorktreePath)
		if !selectOK {
			// One or more worktrees carry this issue's marker but none uniquely
			// matches the recorded WorktreePath (e.g. a legacy explicit-path
			// worktree alongside the canonical .worktrees/<id> one sharing a
			// marker, with an empty/non-matching issue.WorktreePath). This is the
			// same duplicate-marker condition gc reports as GCAmbiguous and exits
			// non-zero on; merged must be consistent and FAIL CLOSED rather than
			// return "not found" and let the violation gate be skipped (I5/I6).
			return "", "", "", false, fmt.Errorf("issue %s has ambiguous marker-bound worktrees; disambiguate before merging", issue.ID)
		}
		worktreePath = item.Path
	} else if branchName := deriveBranchName(issue.Type, issue.ID); branchName != "" {
		// Legacy fallback: an unbound worktree on the expected branch may still
		// be torn down by merged, but a worktree bound to another issue is never
		// eligible merely because its branch happens to match.
		wantRef := "refs/heads/" + branchName
		for _, item := range worktrees {
			if item.IssueID == "" && item.Branch == wantRef {
				worktreePath = item.Path
				break
			}
		}
	}
	if worktreePath == "" {
		return "", "", "", false, nil
	}

	gitDir, err = resolveWorktreeGitDir(worktreePath)
	if err != nil {
		// A located-but-unresolvable worktree is UNREADABLE, not "not found":
		// propagate so callers fail closed instead of skipping the gate.
		return "", "", "", false, fmt.Errorf("resolve worktree git dir for %s: %w", worktreePath, err)
	}

	return worktreePath, gitDir, harnesshook.ReadIssueBindingFile(gitDir), true, nil
}

// issueWorktreeHasViolations reports whether the hook log of the issue's worktree
// contains violation entries. The log is checked when the worktree is bound to this
// issue OR unbound (an unbound worktree on the issue's branch is exactly the
// enforcement-gap case violations exist to catch — finding 1); a worktree bound to
// a different issue is skipped.
func issueWorktreeHasViolations(repoPath string, issue materialize.Issue) (bool, error) {
	_, gitDir, binding, ok, err := resolveIssueWorktree(repoPath, issue)
	if err != nil {
		// Inventory/git-dir unreadable: fail closed — the caller must refuse to
		// mark merged rather than silently treat this as "no violations".
		return false, err
	}
	if !ok {
		return false, nil
	}
	if binding != "" && binding != issue.ID {
		// Bound to a different issue; not ours to gate on.
		return false, nil
	}
	return readHookLogForViolations(gitDir), nil
}

// worktreeRemoveOutcome distinguishes an actual worktree removal from a
// no-op skip, so callers that report removals (e.g. `arm worktree gc`) do not
// count skipped/not-found worktrees as removed.
type worktreeRemoveOutcome int

const (
	// worktreeSkipped means nothing was removed: the worktree was not found by
	// branch or binding, or the clone-local armature-issue-id binding did not match
	// the issue.
	worktreeSkipped worktreeRemoveOutcome = iota
	// worktreeRemoved means the worktree was located, binding-verified, and removed.
	worktreeRemoved
)

// removeWorktreeForIssue removes the git worktree for a given issue if it exists.
// If the worktree is found, it checks the hook log before removing it; if pass-through
// entries are present, a warning is emitted to errWriter. If the worktree is already
// gone (e.g., manually removed), no warning is emitted even if pass-throughs occurred.
func removeWorktreeForIssue(repoPath string, issue materialize.Issue, errWriter io.Writer) error {
	_, err := removeWorktreeForIssueTracked(repoPath, issue, errWriter)
	return err
}

// removeWorktreeForIssueTracked is removeWorktreeForIssue but additionally
// reports whether a worktree was actually removed (worktreeRemoved) versus
// skipped as a no-op (worktreeSkipped). A returned error is a genuine removal
// failure and is orthogonal to the outcome (outcome is worktreeSkipped on error).
func removeWorktreeForIssueTracked(repoPath string, issue materialize.Issue, errWriter io.Writer) (worktreeRemoveOutcome, error) {
	worktreePath, _, binding, ok, err := resolveIssueWorktree(repoPath, issue)
	if err != nil {
		// Inventory/git-dir unreadable: fail closed so a removal is never skipped
		// merely because the worktree could not be inspected.
		return worktreeSkipped, err
	}
	if !ok {
		// No branch for this type or worktree missing; nothing to remove.
		return worktreeSkipped, nil
	}

	// Verify binding before removal: if the armature-issue-id file is missing or
	// names a different issue, skip removal to protect user-created worktrees that
	// happen to match the branch name (P2 bug fix).
	if binding != issue.ID {
		branchName := deriveBranchName(issue.Type, issue.ID)
		_, _ = fmt.Fprintf(errWriter, "Warning: worktree at %s is on branch %s but not bound to %s (binding=%q); skipping removal\n",
			worktreePath, branchName, issue.ID, binding)
		return worktreeSkipped, nil
	}
	return removeWorktreeAtPathTracked(repoPath, issue, worktreePath, errWriter)
}

// removeWorktreeAtPathTracked removes exactly selectedPath after refreshing the
// inventory and revalidating its marker binding. GC passes the path selected by
// reconciliation; it must not re-resolve the issue's worktree by marker alone
// because a legacy and a canonical worktree can share one marker identity.
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
	if selected.IssueID != issue.ID {
		_, _ = fmt.Fprintf(errWriter, "Warning: worktree at %s is now bound to %s, not %s; skipping removal\n",
			selected.Path, selected.IssueID, issue.ID)
		return worktreeSkipped, nil
	}

	gitDir, err := resolveWorktreeGitDir(selected.Path)
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
	if readHookLogForPassThroughs(gitDir) {
		_, _ = fmt.Fprintf(errWriter, "Warning: %s has pass-through entries in armature-hook.log\n", issue.ID)
	}

	// Clear persisted branch-point metadata (parent-branch config, base-commit
	// file) BEFORE removing the worktree: resolveWorktreeGitDir needs the
	// worktree to still exist to locate its git directory. Without this, a
	// stale value would survive branch deletion/recreation and the
	// "if absent" guards in writeParentBranchConfigIfAbsent/
	// writeBaseCommitFileIfAbsent would never overwrite it with the fresh,
	// correct parent for a branch name later reused with a genuinely
	// different parent. Best-effort: never blocks the merged-confirmation flow.
	branchName := strings.TrimPrefix(selected.Branch, "refs/heads/")
	if branchName == "" || branchName == "detached" {
		branchName = deriveBranchName(issue.Type, issue.ID)
	}
	gitClient := adapters.New(repoPath)
	clearBranchPointMetadata(gitClient, selected.Path, branchName)

	// Remove the worktree.
	if err := gitClient.RemoveWorktree(selected.Path); err != nil {
		return worktreeSkipped, fmt.Errorf("remove worktree for %s: %w", issue.ID, err)
	}

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

			// Read index and issue directly from disk; no full rematerialization needed.
			store := newSnapshotStore(ctx)
			index, err := store.ReadIndex()
			if err != nil {
				return fmt.Errorf("read index: %w", err)
			}

			entry, ok := index[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			// Require status=done or status=merged
			if entry.Status != ops.StatusDone && entry.Status != ops.StatusMerged {
				return fmt.Errorf("issue %s is in status %q; arm merged requires status=done (transition it to done first)", issueID, entry.Status)
			}

			// Read the issue to get its type and PR field.
			issue, err := store.ReadIssue(issueID)
			if err != nil {
				return fmt.Errorf("load issue %s: %w", issueID, err)
			}

			// Check for violations in the hook log BEFORE recording the merge op.
			// If violations are found and --force is not set, exit with error and
			// do NOT proceed with the merge (preserving the worktree as evidence).
			if !force {
				hasViolations, err := issueWorktreeHasViolations(ctx.RepoPath, *issue)
				if err != nil {
					// Fail closed: refuse to mark merged when the worktree
					// inventory cannot be read, so an unreadable worktree that may
					// contain violations is never silently merged (I5/I6).
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: cannot verify hook log for %s: %v\n", issueID, err)
					return fmt.Errorf("issue %s cannot be merged: worktree inventory unreadable (use --force to override): %w", issueID, err)
				}
				if hasViolations {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s has violation entries in armature-hook.log\n", issueID)
					return fmt.Errorf("issue %s cannot be merged: hook log contains violations (use --force to override)", issueID)
				}
			}

			// Record the merge op FIRST, before removing the worktree.
			// This ensures that if appendOp fails, the worktree is still present
			// and recovery is possible (P2 bug fix).
			// Only skip op re-recording if already merged AND no new PR to attach OR
			// the issue already has the same PR recorded.
			// If a new --pr value is provided and not already on the issue, record a
			// new transition op to capture it.
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

			// Remove worktree if this is a task, bug, feature, or story type.
			// This happens AFTER the op is recorded, so on failure the worktree is preserved.
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
