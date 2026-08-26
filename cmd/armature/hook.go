package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	armsync "github.com/scullxbones/armature/internal/sync"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Git hook management",
	}

	cmd.AddCommand(newHookRunCmd())
	return cmd
}

func newHookRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <hook-name> [args...]",
		Short: "Run an Armature git hook natively",
		Long: `Run an Armature git hook using native Go logic.

Supported hooks:
  pre-commit          Block .armature/ops/ commits on code branches in dual-branch mode
  post-commit         Send heartbeat for active claim; push ops in dual-branch mode
  post-merge          Sync merged branches and auto-transition done issues
  prepare-commit-msg  Prepend active claim ID to commit message

Examples:
  arm hook run pre-commit
  arm hook run post-commit
  arm hook run post-merge
  arm hook run prepare-commit-msg .git/COMMIT_EDITMSG`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hookName := args[0]
			hookArgs := args[1:]

			var err error
			switch hookName {
			case "pre-commit":
				err = runPreCommitHook(cmd)
			case "post-commit":
				runPostCommitHook(cmd)
			case "post-merge":
				err = runPostMergeHook(cmd)
			case "prepare-commit-msg":
				err = runPrepareCommitMsgHook(cmd, hookArgs)
			default:
				err = fmt.Errorf("unknown hook %q: supported hooks are pre-commit, post-commit, post-merge, prepare-commit-msg", hookName)
			}
			// ADR 0020 §6: arm hook stays on the git protocol, not the
			// agent Command Failure wire.
			return skipCommandFailure(err)
		},
	}
}

// hookCurrentBranch returns the current git branch name, or empty string on error.
func hookCurrentBranch(repoPath string) string {
	gc := adapters.New(repoPath)
	branch, err := gc.CurrentBranch()
	if err != nil {
		return ""
	}
	return branch
}

// hookFindActiveClaimID returns the active claim ID for the current worker, or empty string if none.
func hookFindActiveClaimID(ctx *config.Context) string {
	workerID, err := worker.GetWorkerID(ctx.RepoPath)
	if err != nil {
		return ""
	}

	logPath := fmt.Sprintf("%s/ops/%s.log", ctx.IssuesDir, workerIdentityWithSlot(workerID))

	allOps, err := ops.ReadLog(logPath)
	if err != nil {
		return ""
	}

	defaultTTL := ctx.Config.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = 60
	}
	now := time.Now().Unix()

	claimedAt := make(map[string]int64)
	lastHeartbeat := make(map[string]int64)
	claimTTL := make(map[string]int)
	transitioned := make(map[string]bool)
	lastTransitionAt := make(map[string]int64)

	for _, op := range allOps {
		switch op.Type {
		case ops.OpClaim:
			claimedAt[op.TargetID] = op.Timestamp
			claimTTL[op.TargetID] = op.Payload.TTL
		case ops.OpHeartbeat:
			if op.Timestamp > lastHeartbeat[op.TargetID] {
				lastHeartbeat[op.TargetID] = op.Timestamp
			}
		case ops.OpTransition:
			if op.Payload.To == ops.StatusDone || op.Payload.To == ops.StatusMerged ||
				op.Payload.To == ops.StatusCancelled {
				transitioned[op.TargetID] = true
			}
			// This is a per-worker log: every op here was authored by workerID,
			// so any transition seen for this issue is by construction claimant
			// activity — record it even for non-terminal transitions (e.g.
			// claimed -> in-progress) so it can extend claim liveness like a
			// heartbeat does.
			if op.Timestamp > lastTransitionAt[op.TargetID] {
				lastTransitionAt[op.TargetID] = op.Timestamp
			}
		}
	}

	for issueID, ca := range claimedAt {
		if transitioned[issueID] {
			continue
		}
		ttl := claimTTL[issueID]
		if ttl <= 0 {
			ttl = defaultTTL
		}
		if !claimPkg.IsClaimStale(ca, lastHeartbeat[issueID], lastTransitionAt[issueID], ttl, now) {
			return issueID
		}
	}
	return ""
}

// runPreCommitHook implements the pre-commit hook logic natively.
// Unconditionally blocks additions/modifications to .armature/ops/ on non-_armature branches.
func runPreCommitHook(cmd *cobra.Command) error {
	appCtx := currentCtx(cmd)
	// Allow all commits on _armature branch
	branch := hookCurrentBranch(appCtx.RepoPath)
	if branch == "_armature" {
		return nil
	}

	// Check for staged .armature/ops/ additions/modifications
	gitCmd := adapters.NonInteractiveGitCommand(appCtx.RepoPath, "diff", "--cached", "--name-only", "--diff-filter=AM")
	out, err := gitCmd.Output()
	if err != nil {
		// If git fails (e.g., no commits yet), allow the commit
		return nil
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(line, ".armature/ops/") {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "ERROR: Refusing to commit .armature/ops/ changes on a code branch.")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "ops are written directly to the _armature branch.")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "If you are migrating to dual-branch mode, run: arm bootstrap --dual-branch")
			return fmt.Errorf("refusing to commit .armature/ops/ on branch %q", branch)
		}
	}
	return nil
}

// runPostCommitHook implements the post-commit hook logic natively.
// Sends a heartbeat for any active claim and, in dual-branch mode, pushes ops.
func runPostCommitHook(cmd *cobra.Command) {
	appCtx := currentCtx(cmd)
	// Skip on _armature branch
	branch := hookCurrentBranch(appCtx.RepoPath)
	if branch == "_armature" {
		return
	}

	claimID := hookFindActiveClaimID(appCtx)
	if claimID == "" {
		return
	}

	workerID, logPath, err := resolveWorkerAndLog(appCtx)
	if err != nil {
		// Best-effort — don't block the commit
		return
	}

	op := ops.Op{
		Type:      ops.OpHeartbeat,
		TargetID:  claimID,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
	}
	if err := appendLowStakesOp(mustState(cmd), logPath, op); err != nil {
		// Best-effort — don't block the commit
		return
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Heartbeat recorded for %s\n", claimID)

	hookDetectScopeChanges(cmd, workerID, logPath)
}

// hookDetectScopeChanges parses HEAD~1..HEAD for file renames and deletions,
// then emits scope-rename / scope-delete ops for any issue whose scope is affected.
// It skips silently when HEAD~1 is absent (initial commit) and swallows all errors.
func hookDetectScopeChanges(cmd *cobra.Command, workerID, logPath string) {
	appCtx := currentCtx(cmd)
	// --name-status with diff-filter covers renames (R*) and deletions (D).
	gitCmd := adapters.NonInteractiveGitCommand(appCtx.RepoPath, "diff", "--name-status", "--diff-filter=RD", "HEAD~1", "HEAD")
	out, err := gitCmd.Output()
	if err != nil {
		// HEAD~1 absent on initial commit, or any other git error — skip silently.
		return
	}

	// Load current materialized index to discover which issues are affected.
	store := newSnapshotStore(appCtx)
	index, err := store.ReadIndex()
	if err != nil {
		return
	}
	if index == nil {
		index = make(materialize.Index)
	}

	ts := nowEpoch()

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0]

		if strings.HasPrefix(status, "R") && len(fields) >= 3 {
			// Rename: fields = [RXXX, old_path, new_path]
			oldPath := fields[1]
			newPath := fields[2]
			for issueID, entry := range index {
				for _, s := range entry.Scope {
					if strings.Contains(s, oldPath) {
						op := ops.Op{
							Type:      ops.OpScopeRename,
							TargetID:  issueID,
							Timestamp: ts,
							WorkerID:  workerID,
							Payload: ops.Payload{
								OldPath: oldPath,
								NewPath: newPath,
							},
						}
						_ = appendLowStakesOp(mustState(cmd), logPath, op) //nolint:errcheck // low-stakes op; failure is non-critical
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scope-rename: %s %s -> %s\n", issueID, oldPath, newPath)
						break
					}
				}
			}
		} else if status == "D" {
			// Deletion: fields = [D, deleted_path]
			deletedPath := fields[1]
			for issueID, entry := range index {
				if slices.Contains(entry.Scope, deletedPath) {
					op := ops.Op{
						Type:      ops.OpScopeDelete,
						TargetID:  issueID,
						Timestamp: ts,
						WorkerID:  workerID,
						Payload: ops.Payload{
							DeletedPath: deletedPath,
						},
					}
					_ = appendLowStakesOp(mustState(cmd), logPath, op) //nolint:errcheck // low-stakes op; failure is non-critical
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scope-delete: %s %s\n", issueID, deletedPath)
				}
			}
		}
	}
}

// runPostMergeHook implements the post-merge hook logic natively.
// Runs the sync command to auto-transition done issues to merged.
func runPostMergeHook(cmd *cobra.Command) error {
	appCtx := currentCtx(cmd)
	// Skip on _armature branch
	branch := hookCurrentBranch(appCtx.RepoPath)
	if branch == "_armature" {
		return nil
	}

	// Load snapshot to get materialized state
	store := newSnapshotStore(appCtx)
	snap, err := store.Load(context.Background())
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	// Extract issues map from snapshot and convert to slice for DetectMerges
	issuesMap := snap.State.Issues
	if issuesMap == nil {
		issuesMap = make(map[string]*materialize.Issue)
	}
	issues := make([]materialize.Issue, 0, len(issuesMap))
	for _, issue := range issuesMap {
		if issue != nil {
			issues = append(issues, *issue)
		}
	}

	gc := adapters.New(appCtx.RepoPath)
	mergedIDs, err := armsync.DetectMerges(issues, branch, gc)
	if err != nil {
		return fmt.Errorf("detect merges: %w", err)
	}

	if len(mergedIDs) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No merged branches detected.")
		return nil
	}

	workerID, logPath, err := resolveWorkerAndLog(appCtx)
	if err != nil {
		return err
	}

	for _, id := range mergedIDs {
		op := ops.Op{
			Type:      ops.OpTransition,
			TargetID:  id,
			WorkerID:  workerID,
			Timestamp: nowEpoch(),
			Payload: ops.Payload{
				To:      ops.StatusMerged,
				Outcome: "auto-detected merge into " + branch,
			},
		}
		if err := appendOp(appCtx, logPath, op); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to transition %s: %v\n", id, err)
			continue
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Transitioned %s to merged\n", id)
	}

	// Refresh snapshot after writing ops
	if _, err := store.Load(context.Background()); err != nil {
		return fmt.Errorf("refresh snapshot: %w", err)
	}

	return nil
}

// runPrepareCommitMsgHook implements the prepare-commit-msg hook logic natively.
// If there is an active claim, prepends its ID to the commit message file.
func runPrepareCommitMsgHook(cmd *cobra.Command, args []string) error {
	appCtx := currentCtx(cmd)
	if len(args) == 0 {
		return fmt.Errorf("prepare-commit-msg requires a commit message file path argument")
	}

	// Skip on _armature branch
	branch := hookCurrentBranch(appCtx.RepoPath)
	if branch == "_armature" {
		return nil
	}

	claimID := hookFindActiveClaimID(appCtx)
	if claimID == "" {
		return nil
	}

	msgFile := args[0]
	original, err := os.ReadFile(msgFile) //nolint:gosec // G304: msgFile is the git-supplied commit message path from hook args
	if err != nil {
		return fmt.Errorf("read commit message file %q: %w", msgFile, err)
	}

	updated := claimID + ": " + string(original)
	if err := os.WriteFile(msgFile, []byte(updated), 0o600); err != nil { //nolint:gosec // msgFile is git's COMMIT_EDITMSG path, not user-controlled
		return fmt.Errorf("write commit message file %q: %w", msgFile, err)
	}

	_ = cmd // cmd not used directly for output in this hook
	return nil
}
