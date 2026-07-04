package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
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

	for _, line := range strings.Split(string(data), "\n") {
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
// git dir and binding. Returns ok=false when the issue type has no branch, the
// worktree doesn't exist, or its git dir can't be resolved. binding is the content
// of armature-issue-id ("" when absent). Shared by the merged violation gate and
// worktree removal (finding 7).
func resolveIssueWorktree(repoPath string, issue materialize.Issue) (worktreePath, gitDir, binding string, ok bool) {
	// Only types that claim creates worktrees for have a derivable branch name.
	branchName := deriveBranchName(issue.Type, issue.ID)
	if branchName == "" {
		return "", "", "", false
	}

	worktreePath = findWorktreePathByBranch(repoPath, branchName)
	if worktreePath == "" {
		return "", "", "", false
	}

	gitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return "", "", "", false
	}

	return worktreePath, gitDir, harnesshook.ReadIssueBindingFile(gitDir), true
}

// issueWorktreeHasViolations reports whether the hook log of the issue's worktree
// contains violation entries. The log is checked when the worktree is bound to this
// issue OR unbound (an unbound worktree on the issue's branch is exactly the
// enforcement-gap case violations exist to catch — finding 1); a worktree bound to
// a different issue is skipped.
func issueWorktreeHasViolations(repoPath string, issue materialize.Issue) bool {
	_, gitDir, binding, ok := resolveIssueWorktree(repoPath, issue)
	if !ok {
		return false
	}
	if binding != "" && binding != issue.ID {
		// Bound to a different issue; not ours to gate on.
		return false
	}
	return readHookLogForViolations(gitDir)
}

// removeWorktreeForIssue removes the git worktree for a given issue if it exists.
// If the worktree is found, it checks the hook log before removing it; if pass-through
// entries are present, a warning is emitted to errWriter. If the worktree is already
// gone (e.g., manually removed), no warning is emitted even if pass-throughs occurred.
// Returns true if a pass-through warning was found and emitted.
func removeWorktreeForIssue(repoPath string, issue materialize.Issue, errWriter io.Writer) (bool, error) {
	worktreePath, gitDir, binding, ok := resolveIssueWorktree(repoPath, issue)
	if !ok {
		// No branch for this type, worktree missing, or git dir unresolvable; nothing to remove.
		return false, nil
	}

	// Check for pass-through entries before doing anything else, so the warning
	// is emitted regardless of whether the worktree gets removed.
	hasPassThrough := readHookLogForPassThroughs(gitDir)
	if hasPassThrough {
		_, _ = fmt.Fprintf(errWriter, "Warning: %s has pass-through entries in armature-hook.log\n", issue.ID)
	}

	// Verify binding before removal: if the armature-issue-id file is missing or
	// names a different issue, skip removal to protect user-created worktrees that
	// happen to match the branch name (P2 bug fix).
	if binding != issue.ID {
		branchName := deriveBranchName(issue.Type, issue.ID)
		_, _ = fmt.Fprintf(errWriter, "Warning: worktree at %s is on branch %s but not bound to %s (binding=%q); skipping removal\n",
			worktreePath, branchName, issue.ID, binding)
		return hasPassThrough, nil
	}

	// Remove the worktree.
	gitClient := adapters.New(repoPath)
	if err := gitClient.RemoveWorktree(worktreePath); err != nil {
		return hasPassThrough, fmt.Errorf("remove worktree for %s: %w", issue.ID, err)
	}

	return hasPassThrough, nil
}

// findWorktreePathByBranch finds the worktree path for a given branch name by parsing git worktree list.
// It matches exactly against the refs/heads/<branchName> ref to avoid false positives on
// branch names that are substrings of one another.
func findWorktreePathByBranch(repoPath, branchName string) string {
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Each worktree entry in porcelain format:
	//   worktree <path>
	//   HEAD <sha>
	//   branch refs/heads/<name>   (or "detached")
	//   (empty line)
	wantRef := "refs/heads/" + branchName
	lines := strings.Split(string(output), "\n")
	var currentPath string
	for _, line := range lines {
		if rest, ok := strings.CutPrefix(line, "worktree "); ok {
			currentPath = rest
		} else if rest, ok := strings.CutPrefix(line, "branch "); ok {
			if rest == wantRef {
				return currentPath
			}
		}
	}

	return ""
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
			if !force && issueWorktreeHasViolations(ctx.RepoPath, *issue) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s has violation entries in armature-hook.log\n", issueID)
				return fmt.Errorf("issue %s cannot be merged: hook log contains violations (use --force to override)", issueID)
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
			if _, err := removeWorktreeForIssue(ctx.RepoPath, *issue, cmd.ErrOrStderr()); err != nil {
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
