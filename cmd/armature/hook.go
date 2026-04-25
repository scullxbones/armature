package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/git"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	trellissync "github.com/scullxbones/armature/internal/sync"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "hook",
		Short:             "Git hook management",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	cmd.AddCommand(newHookRunCmd())
	return cmd
}

func newHookRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <hook-name> [args...]",
		Short: "Run a Trellis git hook natively",
		Long: `Run a Trellis git hook using native Go logic.

Supported hooks:
  pre-commit          Block .issues/ops/ commits on code branches in dual-branch mode
  post-commit         Send heartbeat for active claim; push ops in dual-branch mode
  post-merge          Sync merged branches and auto-transition done issues
  prepare-commit-msg  Prepend active claim ID to commit message

Examples:
  trls hook run pre-commit
  trls hook run post-commit
  trls hook run post-merge
  trls hook run prepare-commit-msg .git/COMMIT_EDITMSG`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hookName := args[0]
			hookArgs := args[1:]

			switch hookName {
			case "pre-commit":
				return runPreCommitHook(cmd)
			case "post-commit":
				return runPostCommitHook(cmd)
			case "post-merge":
				return runPostMergeHook(cmd)
			case "prepare-commit-msg":
				return runPrepareCommitMsgHook(cmd, hookArgs)
			default:
				return fmt.Errorf("unknown hook %q: supported hooks are pre-commit, post-commit, post-merge, prepare-commit-msg", hookName)
			}
		},
	}
}

// hookCurrentBranch returns the current git branch name, or empty string on error.
func hookCurrentBranch() string {
	gc := git.New(appCtx.RepoPath)
	branch, err := gc.CurrentBranch()
	if err != nil {
		return ""
	}
	return branch
}

// hookIsDualBranch reports whether the repo is in dual-branch mode.
func hookIsDualBranch() bool {
	return appCtx.Mode == "dual-branch"
}

// hookFindActiveClaimID returns the active claim ID for the current worker, or empty string if none.
func hookFindActiveClaimID() string {
	workerID, err := worker.GetWorkerID(appCtx.RepoPath)
	if err != nil {
		return ""
	}

	logName := workerID
	if slot := os.Getenv("TRLS_LOG_SLOT"); slot != "" {
		logName = workerID + "~" + slot
	}
	logPath := fmt.Sprintf("%s/ops/%s.log", appCtx.IssuesDir, logName)

	allOps, err := ops.ReadLog(logPath)
	if err != nil {
		return ""
	}

	defaultTTL := appCtx.Config.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = 60
	}
	now := time.Now().Unix()

	claimedAt := make(map[string]int64)
	lastHeartbeat := make(map[string]int64)
	claimTTL := make(map[string]int)
	transitioned := make(map[string]bool)

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
		if !claimPkg.IsClaimStale(ca, lastHeartbeat[issueID], ttl, now) {
			return issueID
		}
	}
	return ""
}

// runPreCommitHook implements the pre-commit hook logic natively.
// In dual-branch mode, it blocks additions/modifications to .issues/ops/ on non-_trellis branches.
func runPreCommitHook(cmd *cobra.Command) error {
	// Allow all commits on _trellis branch
	branch := hookCurrentBranch()
	if branch == "_trellis" {
		return nil
	}

	// Single-branch mode: allow ops/ commits
	if !hookIsDualBranch() {
		return nil
	}

	// Check for staged .issues/ops/ additions/modifications
	gitCmd := exec.Command("git", "-C", appCtx.RepoPath,
		"diff", "--cached", "--name-only", "--diff-filter=AM")
	out, err := gitCmd.Output()
	if err != nil {
		// If git fails (e.g., no commits yet), allow the commit
		return nil
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(line, ".issues/ops/") {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "ERROR: Refusing to commit .issues/ops/ changes on a code branch.")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "In dual-branch mode, ops are written directly to the _trellis branch.")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "If you are migrating to dual-branch mode, run: trls init --dual-branch")
			return fmt.Errorf("refusing to commit .issues/ops/ on branch %q in dual-branch mode", branch)
		}
	}
	return nil
}

// runPostCommitHook implements the post-commit hook logic natively.
// Sends a heartbeat for any active claim and, in dual-branch mode, pushes ops.
func runPostCommitHook(cmd *cobra.Command) error {
	// Skip on _trellis branch
	branch := hookCurrentBranch()
	if branch == "_trellis" {
		return nil
	}

	claimID := hookFindActiveClaimID()
	if claimID == "" {
		return nil
	}

	workerID, logPath, err := resolveWorkerAndLog()
	if err != nil {
		// Best-effort — don't block the commit
		return nil
	}

	op := ops.Op{
		Type:      ops.OpHeartbeat,
		TargetID:  claimID,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
	}
	if err := appendLowStakesOp(logPath, op); err != nil {
		// Best-effort — don't block the commit
		return nil
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Heartbeat recorded for %s\n", claimID)
	return nil
}

// runPostMergeHook implements the post-merge hook logic natively.
// Runs the sync command to auto-transition done issues to merged.
func runPostMergeHook(cmd *cobra.Command) error {
	// Skip on _trellis branch
	branch := hookCurrentBranch()
	if branch == "_trellis" {
		return nil
	}

	issuesDir := appCtx.IssuesDir
	singleBranch := appCtx.Mode == "single-branch"

	if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, singleBranch); err != nil {
		return fmt.Errorf("materialize: %w", err)
	}

	gc := git.New(appCtx.RepoPath)
	mergedIDs, err := trellissync.DetectMerges(issuesDir, appCtx.StateDir, branch, gc)
	if err != nil {
		return fmt.Errorf("detect merges: %w", err)
	}

	if len(mergedIDs) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No merged branches detected.")
		return nil
	}

	workerID, logPath, err := resolveWorkerAndLog()
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
		if err := appendOp(logPath, op); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to transition %s: %v\n", id, err)
			continue
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Transitioned %s to merged\n", id)
	}

	if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, singleBranch); err != nil {
		return fmt.Errorf("re-materialize: %w", err)
	}

	return nil
}

// runPrepareCommitMsgHook implements the prepare-commit-msg hook logic natively.
// If there is an active claim, prepends its ID to the commit message file.
func runPrepareCommitMsgHook(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("prepare-commit-msg requires a commit message file path argument")
	}

	// Skip on _trellis branch
	branch := hookCurrentBranch()
	if branch == "_trellis" {
		return nil
	}

	claimID := hookFindActiveClaimID()
	if claimID == "" {
		return nil
	}

	msgFile := args[0]
	original, err := os.ReadFile(msgFile)
	if err != nil {
		return fmt.Errorf("read commit message file %q: %w", msgFile, err)
	}

	updated := claimID + ": " + string(original)
	if err := os.WriteFile(msgFile, []byte(updated), 0644); err != nil {
		return fmt.Errorf("write commit message file %q: %w", msgFile, err)
	}

	_ = cmd // cmd not used directly for output in this hook
	return nil
}
