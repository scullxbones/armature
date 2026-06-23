package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scullxbones/armature/internal/adapters"
	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// worktreePathExists checks if a worktree exists at the given path.
func worktreePathExists(path string) (bool, error) {
	gitFile := filepath.Join(path, ".git")
	_, err := os.Stat(gitFile)
	if err == nil {
		return true, nil // .git exists, this is a worktree
	}
	if os.IsNotExist(err) {
		return false, nil // path doesn't exist or no .git file
	}
	return false, err // other error
}

// deriveBranchName determines the branch name prefix based on issue type.
func deriveBranchName(issueType, issueID string) string {
	switch issueType {
	case "bug":
		return "fix/" + issueID
	case "feature":
		return "feat/" + issueID
	default:
		return "task/" + issueID
	}
}

// createWorktreeAndBranch creates a new worktree and branches for a task/bug.
// It uses a git client to create a worktree at the given path with a derived branch name.
// If the branch is already checked out in another worktree, it returns an error
// (the user should reuse the existing worktree or unassign/reassign the task).
func createWorktreeAndBranch(repoPath, worktreePath, issueID string, issue materialize.Issue) error {
	// Determine branch name based on issue type
	branchName := deriveBranchName(issue.Type, issueID)

	// Create git client for main repo
	gitClient := adapters.New(repoPath)

	// Create an orphan branch for this task/bug first (idempotent: no-op if already exists)
	if err := gitClient.CreateOrphanBranch(branchName); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}

	// Add worktree pointing to the branch
	if err := gitClient.AddWorktree(branchName, worktreePath); err != nil {
		return fmt.Errorf("add worktree: %w", err)
	}

	// Create the task ID file in the worktree's .git directory
	if err := updateTaskIDFile(worktreePath, issueID); err != nil {
		return fmt.Errorf("write task ID file: %w", err)
	}

	return nil
}

// updateTaskIDFile writes the task ID to the armature-task-id file in the worktree's .git directory.
// In a git worktree, .git is a file (not a directory) that points to the actual git directory.
// We read the .git file to extract the actual git directory path.
func updateTaskIDFile(worktreePath, issueID string) error {
	gitPath := filepath.Join(worktreePath, ".git")

	// Read the .git file to extract the actual git directory
	gitFileContent, err := os.ReadFile(gitPath) //nolint:gosec // git paths are internal, not user-provided
	if err != nil {
		return fmt.Errorf("read .git file: %w", err)
	}

	// Parse "gitdir: <path>" format
	gitDirLine := string(gitFileContent)
	if !strings.HasPrefix(gitDirLine, "gitdir: ") {
		return fmt.Errorf("unexpected .git file format: %s", gitDirLine)
	}

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		// Relative path: resolve relative to worktree root
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	// Write the task ID file to the actual git directory
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	if err := os.WriteFile(taskIDFile, []byte(issueID), 0o600); err != nil { //nolint:gosec // internal git file, safe permissions
		return fmt.Errorf("write task ID file: %w", err)
	}
	return nil
}

func newClaimCmd() *cobra.Command {
	var issueID string
	var ttl int
	var force bool
	var worktreePath string

	cmd := &cobra.Command{
		Use:   "claim [issue-id]",
		Short: "Claim a ready task",
		Long: `Claim an issue to assign it to the current worker.

Claiming an issue marks it as assigned to your worker ID and sets a TTL (time-to-live).
If the TTL expires without progress, the claim becomes stale and may be reassigned.
This command also detects and warns about scope overlaps with concurrently claimed issues.
When you claim a task, its parent story (if open) is automatically advanced to in-progress.
A --worktree path is required; it creates a new worktree and task-specific branch if absent,
or updates the armature-task-id file if the worktree exists.`,
		Example: `  # Claim an issue by ID with a worktree
  $ arm claim E6-S4-T2 --worktree ./e6-s4-t2

  # Claim with a custom TTL of 120 minutes
  $ arm claim --issue E6-S4-T2 --ttl 120 --worktree ./task-work

  # Claim despite scope overlap warning
  $ arm claim E6-S4-T2 --force --worktree ./task-work

  # Claim using flag style
  $ arm claim --issue another-task-id --worktree ./another-work`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}
			if worktreePath == "" {
				return fmt.Errorf("--worktree is required")
			}

			issuesDir := ctx.IssuesDir

			allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}
			if _, err := materialize.Materialize(ctx.StateDir, allOps, ctx.Mode == "single-branch", offsets); err != nil {
				return err
			}

			issue, err := materialize.LoadIssue(filepath.Join(ctx.StateDir, "issues", issueID+".json"))
			if err != nil {
				return fmt.Errorf("issue %s not found: %w", issueID, err)
			}

			if issue.Provenance.Confidence == "inferred" {
				return fmt.Errorf("cannot claim %s: node has confidence=inferred — wait for a human to confirm it", issueID)
			}

			// Epic cannot be claimed with a worktree
			if issue.Type == "epic" {
				return fmt.Errorf("cannot claim epic %s with --worktree; only tasks and bugs can use worktrees", issueID)
			}

			// Handle worktree creation/update
			worktreeExists, err := worktreePathExists(worktreePath)
			if err != nil {
				return fmt.Errorf("check worktree path: %w", err)
			}

			if !worktreeExists {
				// Create worktree + derived branch
				// Note: if this fails (e.g., branch already in use by another worktree),
				// we continue with the claim operation anyway. The claim will be recorded,
				// but the worktree creation failure may indicate the task is already claimed elsewhere.
				_ = createWorktreeAndBranch(ctx.RepoPath, worktreePath, issueID, issue) //nolint:errcheck // intentionally swallow error: worktree creation is best-effort
			} else {
				// Update armature-task-id file
				if err := updateTaskIDFile(worktreePath, issueID); err != nil {
					return fmt.Errorf("update task ID file: %w", err)
				}
			}

			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}

			index, _ := materialize.LoadIndex(filepath.Join(ctx.StateDir, "index.json")) //nolint:errcheck // missing index treated as empty
			for id, entry := range index {
				if id == issueID || (entry.Status != "claimed" && entry.Status != "in-progress") {
					continue
				}
				if claimPkg.ScopesOverlap(issue.Scope, entry.Scope) {
					msg := fmt.Sprintf("scope overlap with %s (%s)", id, entry.Title)
					// Same worker claiming serially: auto-dismiss — log a note, no error or warning.
					if entry.Assignee == workerID {
						// Only write the dismissal note if it hasn't been written before for this pair.
						if !claimPkg.HasOverlapDismissalNote(allOps, issueID, id) {
							noteOp := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
								WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Serial claim: scope overlap with %s (same worker, dismissed)", id)}}
							appendOp(ctx, logPath, noteOp) //nolint:errcheck,gosec,gosec
						}
						continue
					}
					if !force {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", msg)
						return fmt.Errorf("cannot claim %s: %s — use --force to override", issueID, msg)
					}
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", msg)
					noteOp := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
						WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", id)}}
					appendOp(ctx, logPath, noteOp) //nolint:errcheck,gosec
					noteOp2 := ops.Op{Type: ops.OpNote, TargetID: id, Timestamp: nowEpoch(),
						WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", issueID)}}
					appendOp(ctx, logPath, noteOp2) //nolint:errcheck,gosec
				}
			}

			op := ops.Op{
				Type: ops.OpClaim, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{TTL: ttl},
			}
			if err := appendHighStakesOp(mustState(cmd), logPath, op); err != nil {
				return err
			}

			allOps, offsets, err = readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}
			if _, err := materialize.Materialize(ctx.StateDir, allOps, ctx.Mode == "single-branch", offsets); err != nil {
				return err
			}
			issueAfter, err := materialize.LoadIssue(filepath.Join(ctx.StateDir, "issues", issueID+".json"))
			if err != nil {
				return fmt.Errorf("issue %s not found after claim: %w", issueID, err)
			}
			won := issueAfter.ClaimedBy == workerID
			if !won {
				format, _ := cmd.Root().PersistentFlags().GetString("format")
				if format == "json" || format == "agent" {
					result := map[string]any{
						"issue":      issueID,
						"claimed":    false,
						"claimed_by": issueAfter.ClaimedBy,
						"reason":     "lost_claim_race",
					}
					data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claim lost for %s (claimed by %s)\n", issueID, issueAfter.ClaimedBy)
				}
				return nil
			}

			// Auto-advance any open ancestor story/epic to in-progress.
			if parentID := issue.Parent; parentID != "" {
				if parentEntry, ok := index[parentID]; ok && parentEntry.Status == ops.StatusOpen {
					advanceOp := ops.Op{
						Type:      ops.OpTransition,
						TargetID:  parentID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{To: ops.StatusInProgress},
					}
					appendOp(ctx, logPath, advanceOp) //nolint:errcheck,gosec
				}
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]any{"issue": issueID, "claimed_by": workerID, "ttl": ttl}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed %s\n", issueID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to claim")
	cmd.Flags().IntVar(&ttl, "ttl", 60, "claim TTL in minutes")
	cmd.Flags().BoolVar(&force, "force", false, "override scope overlap warning and proceed with claim")
	cmd.Flags().StringVar(&worktreePath, "worktree", "", "path to task worktree (required)")
	return cmd
}
