package main

import (
	"context"
	"fmt"
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

// platformProtocolRoots names the command subtrees whose stdout belongs to an
// external platform rather than to the agent error contract: git drives
// `arm hook`, and the harness drives `arm harness-hook`. ADR 0020 §6 keeps both
// on the platform/git protocol on the wire — non-zero exit plus a stderr
// reason — even though they are Command Failures conceptually.
var platformProtocolRoots = map[string]bool{
	"hook":         true,
	"harness-hook": true,
}

func staysOnPlatformProtocol(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if platformProtocolRoots[c.Name()] {
			return true
		}
	}
	return false
}

// argvNamesPlatformProtocol reports whether argv mentions one of those subtrees
// before a `--` terminator.
//
// It exists because the parent-chain walk cannot be trusted when cobra failed to
// resolve a subcommand at all. Find strips flags before matching, skipping the
// token after any flag it does not recognize or that takes a value, so both
// `arm --bad-flag hook run pre-commit` (version skew between a hook script and
// the binary) and `arm --repo hook run pre-commit` (an unquoted empty $REPO in a
// wrapper) leave the hook subtree unentered and hand ExecuteC the root command.
//
// The rule is deliberately blunt — any bare `hook`/`harness-hook` token counts —
// because the two outcomes are not symmetric. A false positive only moves the
// reason for an already-unresolvable invocation from stdout to stderr; a false
// negative writes a Command Failure object onto a stdout that git or the harness
// owns, which is the violation this whole classification exists to prevent.
func argvNamesPlatformProtocol(argv []string) bool {
	for _, arg := range argv {
		if arg == "--" {
			return false
		}
		if platformProtocolRoots[arg] {
			return true
		}
	}
	return false
}

// platformProtocolError keeps err off the Command Failure wire when cmd is one
// of those subtrees. Callers use it wherever an error can reach the root
// handler without RunE having classified it: PersistentPreRunE, and the Execute
// seam for the cobra parse and Args errors that precede PersistentPreRunE.
func platformProtocolError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	if staysOnPlatformProtocol(cmd) {
		return skipCommandFailure(err)
	}
	return err
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

Examples:
  arm hook run pre-commit
  arm hook run post-commit
  arm hook run post-merge`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
				return skipCommandFailure(err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			hookName := args[0]

			var err error
			switch hookName {
			case "pre-commit":
				err = runPreCommitHook(cmd)
			case "post-commit":
				runPostCommitHook(cmd)
			case "post-merge":
				err = runPostMergeHook(cmd)
			default:
				err = fmt.Errorf("unknown hook %q: supported hooks are pre-commit, post-commit, post-merge", hookName)
			}
			// ADR 0020 §6: arm hook stays on the git protocol, not the
			// agent Command Failure wire.
			return skipCommandFailure(err)
		},
	}
}

func hookCurrentBranch(repoPath string) string {
	gc := adapters.New(repoPath)
	branch, err := gc.CurrentBranch()
	if err != nil {
		return ""
	}
	return branch
}

func hookFindActiveClaimID(ctx *config.Context) string {
	workerID, err := worker.GetWorkerID(ctx.RepoPath)
	if err != nil {
		return ""
	}

	logPath := fmt.Sprintf("%s/ops/%s.log", ctx.IssuesDir, slottedWorkerID(workerID).String())

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
			if isTerminalStatus(op.Payload.To) {
				transitioned[op.TargetID] = true
			}
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
		last := claimPkg.FoldLastActivity(ca, lastHeartbeat[issueID], lastTransitionAt[issueID])
		if !claimPkg.IsClaimStale(last, ttl, now) {
			return issueID
		}
	}
	return ""
}

func runPreCommitHook(cmd *cobra.Command) error {
	checkout := invocationRepoPath(cmd)
	branch := hookCurrentBranch(checkout)
	if branch == "_armature" {
		return nil
	}

	gitCmd := adapters.NonInteractiveGitCommand(checkout, "diff", "--cached", "--name-only", "--diff-filter=AM")
	out, err := gitCmd.Output()
	if err != nil {
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

func runPostCommitHook(cmd *cobra.Command) {
	appCtx := currentCtx(cmd)
	branch := hookCurrentBranch(invocationRepoPath(cmd))
	if branch == "_armature" {
		return
	}

	claimID := hookFindActiveClaimID(appCtx)
	if claimID == "" {
		return
	}

	workerID, logPath, err := resolveWorkerAndLog(appCtx)
	if err != nil {
		return
	}

	op := ops.Op{
		Type:      ops.OpHeartbeat,
		TargetID:  claimID,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
	}
	if err := appendLowStakesOp(mustState(cmd), logPath, op); err != nil {
		return
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Heartbeat recorded for %s\n", claimID)

	hookDetectScopeChanges(cmd, workerID, logPath)
}

func hookDetectScopeChanges(cmd *cobra.Command, workerID, logPath string) {
	appCtx := currentCtx(cmd)
	gitCmd := adapters.NonInteractiveGitCommand(invocationRepoPath(cmd), "diff", "--name-status", "--find-renames", "--diff-filter=RD", "HEAD~1", "HEAD")
	out, err := gitCmd.Output()
	if err != nil {
		return
	}

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
						if err := appendLowStakesOp(mustState(cmd), logPath, op); err != nil {
							_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to record scope-rename: %v\n", err)
							break
						}
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scope-rename: %s %s -> %s\n", issueID, oldPath, newPath)
						break
					}
				}
			}
		} else if status == "D" {
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
					if err := appendLowStakesOp(mustState(cmd), logPath, op); err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to record scope-delete: %v\n", err)
						continue
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scope-delete: %s %s\n", issueID, deletedPath)
				}
			}
		}
	}
}

func runPostMergeHook(cmd *cobra.Command) error {
	appCtx := currentCtx(cmd)
	branch := hookCurrentBranch(appCtx.RepoPath)
	if branch == "_armature" {
		return nil
	}

	store := newSnapshotStore(appCtx)
	snap, err := store.Load(context.Background())
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

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

	if _, err := store.Load(context.Background()); err != nil {
		return fmt.Errorf("refresh snapshot: %w", err)
	}

	return nil
}
