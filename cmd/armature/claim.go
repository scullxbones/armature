package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scullxbones/armature/internal/adapters"
	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// resolveWorktreeGitDir resolves the actual git directory for a worktree path.
// In a git worktree, the .git entry is a file (not a directory) containing
// "gitdir: <path>" pointing to the real git dir (e.g., <parent>/.git/worktrees/<name>).
// This function reads that file and returns the resolved absolute path.
// It is used by both claim and harness-hook so that both read from the same location.
func resolveWorktreeGitDir(worktreePath string) (string, error) {
	gitPath := filepath.Join(worktreePath, ".git")
	// If .git is a directory (main worktree), return it directly.
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("stat .git: %w", err)
	}
	if info.IsDir() {
		return gitPath, nil
	}

	// .git is a file — read "gitdir: <path>" from it.
	//nolint:gosec // git paths are internal, not user-provided
	gitFileContent, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("read .git file: %w", err)
	}
	gitDirLine := strings.TrimSpace(string(gitFileContent))
	if !strings.HasPrefix(gitDirLine, "gitdir: ") {
		return "", fmt.Errorf("unexpected .git file format: %s", gitDirLine)
	}
	actualGitDir := strings.TrimPrefix(gitDirLine, "gitdir: ")
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	return actualGitDir, nil
}

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

// isWorktreeOf checks if a worktree at worktreePath is registered to the git repository at repoPath.
// It runs `git -C repoPath worktree list --porcelain` and checks if worktreePath appears in the list.
// This prevents claiming a worktree that belongs to a different git repository.
func isWorktreeOf(repoPath, worktreePath string) bool {
	// Use EvalSymlinks to resolve symlinks for accurate path comparison.
	// git worktree list --porcelain emits resolved paths, so we must match them.
	absWorktreePath, err := filepath.EvalSymlinks(worktreePath)
	if err != nil {
		// Fall back to Abs if EvalSymlinks fails (path may not exist yet).
		absWorktreePath, err = filepath.Abs(worktreePath)
		if err != nil {
			return false
		}
	}

	// Get the worktree registered to this repo by parsing `git worktree list --porcelain`
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Parse porcelain format: each worktree entry starts with "worktree <path>"
	for line := range strings.SplitSeq(string(output), "\n") {
		if rest, ok := strings.CutPrefix(line, "worktree "); ok {
			registeredPath := strings.TrimSpace(rest)
			// Use EvalSymlinks to resolve symlinks for accurate path comparison.
			// git worktree list --porcelain emits resolved paths, so we must match them.
			absRegisteredPath, err := filepath.EvalSymlinks(registeredPath)
			if err != nil {
				// Fall back to Abs if EvalSymlinks fails (path may not exist yet).
				absRegisteredPath, err = filepath.Abs(registeredPath)
				if err != nil {
					continue
				}
			}
			if absRegisteredPath == absWorktreePath {
				return true
			}
		}
	}

	return false
}

// checkExistingWorktreeBinding verifies that an existing worktree at path is bound
// to the expected issue and is on the expected branch. Returns an error if the
// worktree is bound to a different issue or is on a mismatched branch, preventing
// silent overwrite of the binding (fix for worktree mismatch governance gap).
func checkExistingWorktreeBinding(worktreePath, issueID, expectedBranch string) error {
	actualGitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return nil // can't resolve git dir; let later steps surface the error
	}

	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	existingBytes, err := os.ReadFile(taskIDFile) //nolint:gosec // internal path
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing binding: %w", err)
	}

	existingTaskID := strings.TrimSpace(string(existingBytes))
	if existingTaskID != "" && existingTaskID != issueID {
		return fmt.Errorf("worktree at %s is already bound to %s: use a different --worktree path",
			worktreePath, existingTaskID)
	}

	// Also verify the worktree's current branch matches the expected branch.
	headFile := filepath.Join(actualGitDir, "HEAD")
	headBytes, err := os.ReadFile(headFile) //nolint:gosec // internal path
	if err != nil {
		return nil // no HEAD yet (fresh or detached); allow claim to proceed
	}
	headStr := strings.TrimSpace(string(headBytes))
	// Skip branch check for detached HEAD only when already bound to this issue
	if !strings.HasPrefix(headStr, "ref: refs/heads/") {
		if existingTaskID == issueID {
			return nil // already bound to this issue, detached HEAD is acceptable (mid-rebase, etc.)
		}
		return fmt.Errorf("worktree at %s has a detached HEAD with no existing binding for %s: checkout the expected branch %q or use a different --worktree path",
			worktreePath, issueID, expectedBranch)
	}
	expectedRef := "ref: refs/heads/" + expectedBranch
	if headStr != expectedRef {
		actualBranch := strings.TrimPrefix(headStr, "ref: refs/heads/")
		return fmt.Errorf("worktree at %s is on branch %q but expected %q for issue %s: use a different --worktree path",
			worktreePath, actualBranch, expectedBranch, issueID)
	}

	return nil
}

// deriveBranchName determines the branch name for a worktree based on issue type.
// Returns an empty string for types that do not receive a worktree (e.g., epic).
// claim creates worktrees for task, bug, feature, and story; merged uses this to tear them down.
func deriveBranchName(issueType, issueID string) string {
	switch issueType {
	case "bug":
		return "fix/" + issueID
	case "feature":
		return "feat/" + issueID
	case "story":
		return "feat/" + issueID
	case "task":
		return "task/" + issueID
	default:
		// epic and unknown types do not have worktrees.
		return ""
	}
}

// createWorktreeAndBranch creates a new worktree and branches for a task/bug.
// It uses a git client to create a worktree at the given path with a derived branch name.
// If the branch is already checked out in another worktree or if worktree creation fails,
// it returns an error (the user should reuse the existing worktree or unassign/reassign the task).
func createWorktreeAndBranch(repoPath, worktreePath, issueID string, issue materialize.Issue) error {
	// Determine branch name based on issue type
	branchName := deriveBranchName(issue.Type, issueID)

	// Safety guard: empty branch name indicates an issue type that should not have a worktree
	if branchName == "" {
		return fmt.Errorf("cannot create worktree for issue type %q: no branch mapping", issue.Type)
	}

	// Create git client for main repo
	gitClient := adapters.New(repoPath)

	// Create a branch from HEAD for this task/bug (idempotent: no-op if already exists)
	// The branch inherits all commits and files from HEAD, unlike an orphan branch.
	if err := gitClient.CreateBranchFrom(branchName, "HEAD"); err != nil {
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
// We use resolveWorktreeGitDir to find the real git directory.
func updateTaskIDFile(worktreePath, issueID string) error {
	actualGitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree git dir: %w", err)
	}

	// Write the task ID file to the actual git directory
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	if err := os.WriteFile(taskIDFile, []byte(issueID), 0o600); err != nil {
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

			// Normalize worktreePath to an absolute path to ensure all subsequent
			// operations (worktreePathExists, updateTaskIDFile, etc.) resolve paths
			// relative to the worktree location, not the current working directory.
			absWorktreePath, err := filepath.Abs(worktreePath)
			if err != nil {
				return fmt.Errorf("resolve worktree path: %w", err)
			}
			worktreePath = absWorktreePath

			// Reject the main checkout as a worktree — it can't be removed by git worktree remove.
			// The main checkout has .git as a directory; a linked worktree has .git as a file.
			gitEntry := filepath.Join(worktreePath, ".git")
			if info, statErr := os.Stat(gitEntry); statErr == nil && info.IsDir() {
				return fmt.Errorf("--worktree %s is the main checkout; pass a linked worktree path (created with git worktree add) instead", worktreePath)
			}

			issuesDir := ctx.IssuesDir

			// allOps is retained here because HasOverlapDismissalNote (below) needs the raw
			// op log to detect prior dismissal notes — data the store's Index does not expose.
			// The store.Load call below independently materializes state; this read is not redundant.
			allOps, err := readAllOpsFromDir(filepath.Join(issuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}

			// Create store and load before first issue read
			store := newSnapshotStore(ctx)
			if _, err := store.Load(context.Background()); err != nil {
				return fmt.Errorf("load store: %w", err)
			}

			issue := store.Issue(issueID)
			if issue == nil {
				return fmt.Errorf("issue %s not found", issueID)
			}

			if issue.Provenance.Confidence == "inferred" {
				return fmt.Errorf("cannot claim %s: node has confidence=inferred — wait for a human to confirm it", issueID)
			}

			// Determine the expected branch for this issue type.
			// Issues with no branch mapping (epic, unknown) cannot use --worktree.
			expectedBranch := deriveBranchName(issue.Type, issueID)
			if expectedBranch == "" {
				return fmt.Errorf("cannot create worktree for issue type %q: no branch mapping", issue.Type)
			}

			// Check whether the worktree path already exists. Capture the state here
			// (no side effects yet) so worktree creation can be deferred until after
			// all claim validations pass.
			worktreeExists, err := worktreePathExists(worktreePath)
			if err != nil {
				return fmt.Errorf("check worktree path: %w", err)
			}

			// Verify the existing worktree is registered to this repo (not a foreign repo).
			// This prevents writing armature-task-id into a foreign repo's git dir,
			// which would cause later merged operations (which search only this repo's worktree list)
			// to permanently fail to find and clean up the worktree.
			if worktreeExists {
				if !isWorktreeOf(ctx.RepoPath, worktreePath) {
					return fmt.Errorf("worktree at %s is not registered to this repository; it may belong to a different clone", worktreePath)
				}
			}

			// If the worktree already exists, verify it is bound to the correct issue
			// and is on the correct branch. Reject silently overwriting a binding that
			// belongs to a different issue.
			if worktreeExists {
				if err := checkExistingWorktreeBinding(worktreePath, issueID, expectedBranch); err != nil {
					return err
				}
			}

			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}

			// Capture the prior status and claimed-by before writing the claim op.
			// If worktree setup fails, we'll use this to determine rollback behavior:
			// - Same-worker active claim (priorClaimedBy == workerID && !stale): keep prior status
			// - Stale same-worker claim (priorClaimedBy == workerID && stale): rollback to StatusOpen
			// - Different-worker takeover (priorClaimedBy != workerID): rollback to StatusOpen
			priorStatus := issue.Status
			priorClaimedBy := issue.ClaimedBy
			priorClaimedAt := issue.ClaimedAt
			priorLastHeartbeat := issue.LastHeartbeat
			priorClaimTTL := issue.ClaimTTL

			index := store.Index()
			for id, entry := range index {
				if id == issueID || (entry.Status != ops.StatusClaimed && entry.Status != ops.StatusInProgress) {
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

			// Refresh store after appending claim Op
			if _, err := store.Refresh(context.Background()); err != nil {
				return fmt.Errorf("refresh store after claim: %w", err)
			}

			issueAfter := store.Issue(issueID)
			if issueAfter == nil {
				return fmt.Errorf("issue %s not found after claim", issueID)
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

			// Worktree setup is deferred to here so it only happens after all claim
			// validations pass and this worker has won the claim race.
			if !worktreeExists {
				if err := createWorktreeAndBranch(ctx.RepoPath, worktreePath, issueID, *issue); err != nil {
					// Worktree creation failed after winning the claim race.
					// Determine rollback status based on whether this is a same-worker active claim or stale/takeover:
					// - Same-worker ACTIVE claim (priorClaimedBy == workerID && !stale): restore priorStatus (keep the claim)
					// - Same-worker STALE claim (priorClaimedBy == workerID && stale): rollback to StatusOpen (release)
					// - Different-worker takeover (priorClaimedBy != workerID or empty): rollback to StatusOpen (release)
					rollbackStatus := ops.StatusOpen
					priorWasActive := priorClaimedBy == workerID &&
						!claimPkg.IsClaimStale(priorClaimedAt, priorLastHeartbeat, priorClaimTTL, nowEpoch())
					if priorWasActive {
						rollbackStatus = priorStatus
					}
					rollbackOp := ops.Op{
						Type:      ops.OpTransition,
						TargetID:  issueID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{To: rollbackStatus},
					}
					if rbErr := appendHighStakesOp(mustState(cmd), logPath, rollbackOp); rbErr != nil {
						return fmt.Errorf("create worktree: %w; also failed to push claim release: %v (manual cleanup may be needed)", err, rbErr)
					}
					return fmt.Errorf("create worktree: %w (claim released; retry arm claim)", err)
				}
			} else {
				// Worktree exists and binding was already validated above; update the
				// task ID file to ensure the binding is current (idempotent).
				if err := updateTaskIDFile(worktreePath, issueID); err != nil {
					// Task ID update failed after winning the claim race.
					// Determine rollback status based on whether this is a same-worker active claim or stale/takeover:
					// - Same-worker ACTIVE claim (priorClaimedBy == workerID && !stale): restore priorStatus (keep the claim)
					// - Same-worker STALE claim (priorClaimedBy == workerID && stale): rollback to StatusOpen (release)
					// - Different-worker takeover (priorClaimedBy != workerID or empty): rollback to StatusOpen (release)
					rollbackStatus := ops.StatusOpen
					priorWasActive := priorClaimedBy == workerID &&
						!claimPkg.IsClaimStale(priorClaimedAt, priorLastHeartbeat, priorClaimTTL, nowEpoch())
					if priorWasActive {
						rollbackStatus = priorStatus
					}
					rollbackOp := ops.Op{
						Type:      ops.OpTransition,
						TargetID:  issueID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{To: rollbackStatus},
					}
					if rbErr := appendHighStakesOp(mustState(cmd), logPath, rollbackOp); rbErr != nil {
						return fmt.Errorf("update task ID file: %w; also failed to push claim release: %v (manual cleanup may be needed)", err, rbErr)
					}
					return fmt.Errorf("update task ID file: %w (claim released; retry arm claim)", err)
				}
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
