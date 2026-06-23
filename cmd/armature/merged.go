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
// Returns true if a pass-through warning was found.
// The issueID parameter is used for warning messages when we can't access it from the issue.
func removeWorktreeForIssue(repoPath string, issue materialize.Issue, errWriter interface{}) (bool, error) {
	// Only remove worktrees for task and bug types
	if issue.Type != "task" && issue.Type != "bug" {
		return false, nil
	}

	// Derive the branch name from the issue type and ID
	var branchName string
	switch issue.Type {
	case "bug":
		branchName = "fix/" + issue.ID
	case "task":
		branchName = "task/" + issue.ID
	default:
		return false, nil
	}

	// List all worktrees and find the one for this branch
	gitClient := adapters.New(repoPath)

	// We need to find the worktree path that corresponds to this branch.
	// We can use git worktree list to find it.
	// Since Client doesn't expose a method for this, we'll construct a helper.
	// For now, use a simpler approach: iterate through known worktree locations
	// or use git worktree list --porcelain.

	// Actually, the simplest approach: use git to find the worktree for this branch
	// We can run: git worktree list --porcelain | grep <branch>
	// But we need to parse the output.

	// Alternative: use the git binary directly since adapters.Client has a cmd method
	// But that's private. Let's create a small helper that uses exec.

	// For now, let's use a different approach: the worktree path is printed by git worktree list
	// We can parse that output using os/exec directly in a portable way.

	worktreePath := findWorktreePathByBranch(repoPath, branchName)
	if worktreePath == "" {
		// Worktree not found; nothing to remove
		return false, nil
	}

	// Check for pass-through entries before removing
	hasPassThrough := false
	gitDir := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitDir) //nolint:gosec // internal git path
	if err == nil {
		// Parse the .git file to find the actual git directory
		gitDirLine := string(gitFileContent)
		if strings.HasPrefix(gitDirLine, "gitdir: ") {
			actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
			if !filepath.IsAbs(actualGitDir) {
				actualGitDir = filepath.Join(worktreePath, actualGitDir)
			}
			// Check for pass-through entries
			hasPassThrough = readHookLogForPassThroughs(actualGitDir)
			if hasPassThrough {
				fmt.Fprintf(errWriter.(io.Writer), "Warning: %s has pass-through entries in armature-hook.log\n", issue.ID) //nolint:errcheck // warning write not critical
			}
		}
	}

	// Remove the worktree
	if err := gitClient.RemoveWorktree(worktreePath); err != nil {
		return false, fmt.Errorf("remove worktree for %s: %w", issue.ID, err)
	}

	return hasPassThrough, nil
}

// findWorktreePathByBranch finds the worktree path for a given branch name by parsing git worktree list.
func findWorktreePathByBranch(repoPath, branchName string) string {
	// Use git worktree list to find the worktree for the branch.
	// Format: <path> <commit> <branch>
	// Example: /path/to/worktree abc1234 [branch]

	// We need to execute: git -C <repoPath> worktree list --porcelain
	// Then parse the output to find the branch

	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		// If command fails, worktree may not exist or git version doesn't support --porcelain
		return ""
	}

	// Parse the output
	// Each worktree entry has:
	// worktree <path>
	// branch <ref>
	// (empty line)

	lines := strings.Split(string(output), "\n")
	var currentPath string
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branchRef := strings.TrimPrefix(line, "branch ")
			// branchRef is like "refs/heads/task/ARCHIMP-S12-T1"
			if strings.HasSuffix(branchRef, branchName) || strings.Contains(branchRef, "/"+branchName) {
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
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			// Materialize to get current state
			allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}
			if _, err := materialize.Materialize(appCtx.StateDir, allOps, singleBranch, offsets); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			index, err := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
			if err != nil {
				return fmt.Errorf("load index: %w", err)
			}

			entry, ok := index[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			// In dual-branch mode, require current status to be "done"
			if !singleBranch && entry.Status != ops.StatusDone {
				return fmt.Errorf("issue %s is in status %q; arm merged requires status=done (transition it to done first)", issueID, entry.Status)
			}

			// Load the issue to get its type
			issue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", issueID+".json"))
			if err != nil {
				return fmt.Errorf("load issue %s: %w", issueID, err)
			}

			// Remove worktree if this is a task or bug type
			if _, err := removeWorktreeForIssue(appCtx.RepoPath, issue, cmd.ErrOrStderr()); err != nil {
				return err
			}

			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
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
			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}

			if singleBranch {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: in single-branch mode, done→merged is automatic. Op recorded for %s.\n", issueID)
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
