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
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

// readHookLogForPassThroughs reads the armature-hook.log file and returns true if it contains pass-through entries.
func readHookLogForPassThroughs(gitDir string) bool {
	logPath := filepath.Join(gitDir, "armature-hook.log")
	data, err := os.ReadFile(logPath) //nolint:gosec // log path is derived from trusted git directory
	if err != nil {
		// Log doesn't exist or can't be read; no pass-throughs found
		return false
	}

	content := string(data)
	return strings.Contains(content, "pass-through:")
}

// removeWorktreeForIssue removes the git worktree for a given issue if it exists.
// If the worktree is found, it checks the hook log before removing it; if pass-through
// entries are present, a warning is emitted to errWriter. If the worktree is already
// gone (e.g., manually removed), no warning is emitted even if pass-throughs occurred.
// Returns true if a pass-through warning was found and emitted.
func removeWorktreeForIssue(repoPath string, issue materialize.Issue, errWriter io.Writer) (bool, error) {
	// Only remove worktrees for types that claim creates them for.
	// deriveBranchName returns a non-empty prefix for task, bug, feature, and story types.
	branchName := deriveBranchName(issue.Type, issue.ID)
	if branchName == "" {
		return false, nil
	}

	// Check for pass-through entries before doing anything else, so the warning
	// is emitted regardless of whether the worktree is still present.
	hasPassThrough := false

	worktreePath := findWorktreePathByBranch(repoPath, branchName)
	if worktreePath != "" {
		// Verify binding before proceeding: check if the worktree is actually bound
		// to this issue via the armature-task-id file in the git dir.
		// If the binding is missing or wrong, skip removal to protect user-created
		// worktrees that happen to match the branch name (P2 bug fix).
		if actualGitDir, err := resolveWorktreeGitDir(worktreePath); err == nil {
			bindingBytes, readErr := os.ReadFile(filepath.Join(actualGitDir, "armature-task-id")) //nolint:gosec // internal git dir path
			binding := strings.TrimSpace(string(bindingBytes))
			// If the file doesn't exist or contains a different ID, skip removal
			if readErr != nil || binding != issue.ID {
				_, _ = fmt.Fprintf(errWriter, "Warning: worktree at %s is on branch %s but not bound to %s (binding=%q); skipping removal\n",
					worktreePath, branchName, issue.ID, binding)
				worktreePath = "" // skip removal
			}
			hasPassThrough = readHookLogForPassThroughs(actualGitDir)
		}
	}

	if hasPassThrough {
		_, _ = fmt.Fprintf(errWriter, "Warning: %s has pass-through entries in armature-hook.log\n", issue.ID)
	}

	if worktreePath == "" {
		// Worktree not found; nothing to remove.
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

	cmd := &cobra.Command{
		Use:   "merged",
		Short: "Mark a done issue as merged after its branch/PR is merged",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)
			singleBranch := ctx.Mode == "single-branch"

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

			// Require status=done (dual-branch) or status=merged (single-branch, where done auto-advances)
			if singleBranch {
				if entry.Status != ops.StatusMerged && entry.Status != ops.StatusDone {
					return fmt.Errorf("issue %s is in status %q; arm merged in single-branch mode requires status=merged (or done)", issueID, entry.Status)
				}
			} else {
				if entry.Status != ops.StatusDone && entry.Status != ops.StatusMerged {
					return fmt.Errorf("issue %s is in status %q; arm merged requires status=done (transition it to done first)", issueID, entry.Status)
				}
			}

			// Read the issue to get its type and PR field.
			issue, err := store.ReadIssue(issueID)
			if err != nil {
				return fmt.Errorf("load issue %s: %w", issueID, err)
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

			if singleBranch {
				if entry.Status == ops.StatusMerged {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: %s already merged. Worktree cleaned up.\n", issueID)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: in single-branch mode, done→merged is automatic. Op recorded for %s.\n", issueID)
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Marked %s as merged", issueID)
				if pr != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), " (PR #%s)", pr)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&pr, "pr", "", "PR number or URL")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
