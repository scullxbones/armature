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

// hookLogContainsEntry reads <git-dir>/armature-hook.log and reports whether any
// line contains an entry of the given kind (e.g. "violation:", "pass-through:").
// Entries are matched at the start of the line body (immediately after the
// RFC3339 timestamp), so injected newlines inside logged fields cannot forge
// entries mid-line (finding 8; fields are also sanitized at write time).
func hookLogContainsEntry(gitDir, kind string) (bool, error) {
	logPath := filepath.Join(gitDir, "armature-hook.log")
	data, err := os.ReadFile(logPath) //nolint:gosec // log path is derived from trusted git directory
	if err != nil {
		if os.IsNotExist(err) {
			// A missing log means this worktree recorded no hook evidence.
			return false, nil
		}
		return false, fmt.Errorf("read hook log %s: %w", logPath, err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		// Each entry is "<RFC3339 timestamp> <kind> ...": strip the timestamp
		// (first space-delimited token) and match the kind at line-body start.
		_, rest, found := strings.Cut(line, " ")
		if found && strings.HasPrefix(rest, kind) {
			return true, nil
		}
	}
	return false, nil
}

// readHookLogForPassThroughs reports whether the hook log contains pass-through entries.
func readHookLogForPassThroughs(gitDir string) (bool, error) {
	return hookLogContainsEntry(gitDir, "pass-through:")
}

// readHookLogForViolations reports whether the hook log contains violation entries.
func readHookLogForViolations(gitDir string) (bool, error) {
	return hookLogContainsEntry(gitDir, "violation:")
}

// resolveBoundWorktree resolves the worktree an issue owns, by binding alone.
//
// This is the SELECTION half of the split (see worktree.SelectByIssue): it is
// the only resolver in this file whose result may reach a destructive git
// operation, and it therefore consults nothing but the issue binding. Branch
// name is not identity and is never used here.
//
// A non-nil error means the bound set is Ambiguous. Callers MUST fail closed:
// guessing would force-remove a worktree that may hold uncommitted work, and
// reporting "nothing found" would let `arm merged` append the merged op while a
// worktree that may contain violations is never inspected (I5 gates fail
// closed; I6 done≠merged).
//
// It takes an inventory the caller has already read rather than reading its
// own, so a caller that needs the inventory for more than selection does not
// shell out to git twice.
func resolveBoundWorktree(worktrees []worktree.Meta, issue materialize.Issue) (worktree.Meta, worktree.Resolution, error) {
	item, res := worktree.SelectByIssue(worktrees, issue.ID, issue.WorktreePath)
	if res == worktree.Ambiguous {
		// Several worktrees carry this issue's binding and the recorded path
		// picks none of them — e.g. a legacy explicit-path worktree alongside
		// the canonical .worktrees/<id> one. This is the same condition gc
		// reports as GCAmbiguous and exits non-zero on.
		return worktree.Meta{}, res, fmt.Errorf("issue %s has ambiguous bound worktrees; disambiguate before merging", issue.ID)
	}
	return item, res, nil
}

// findGateTarget returns the git dir whose hook log the merged violation gate
// must read, and the binding recorded in it ("" when absent).
//
// This is deliberately NOT resolveBoundWorktree, and deliberately returns a git
// dir rather than a worktree path: it is allowed to widen beyond the binding,
// so its result must be structurally incapable of reaching a removal. Only a
// read of a hook log can be done with what this returns.
//
// The widening: when no worktree carries the issue's binding, an UNBOUND
// worktree sitting on the issue's expected branch is still gated on. That is
// precisely the enforcement gap violations exist to catch — a worktree whose
// binding was never written or was deleted would otherwise pass unexamined. A
// worktree bound to a DIFFERENT issue is never eligible merely because its
// branch matches. Widening here is safe only because it is one-directional:
// it can add work for the gate, never authorize a destructive act.
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
		// A located-but-unresolvable worktree is UNREADABLE, not "not found":
		// propagate so callers fail closed instead of skipping the gate.
		return "", "", false, fmt.Errorf("resolve worktree git dir for %s: %w", worktreePath, err)
	}
	return gitDir, harnesshook.ReadIssueBindingFile(gitDir), true, nil
}

// unboundWorktreeOnBranch returns the path of a worktree that carries NO issue
// binding but sits on the issue's expected branch, or "" when there is none.
//
// This is the one place branch name is consulted, and it is deliberately not
// identity: the result is only ever used to widen a read (the violation gate)
// or to explain a refusal (the removal warning). A worktree bound to a
// different issue is never returned — its binding says whose it is.
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

// issueWorktreeHasViolations reports whether the hook log of the issue's worktree
// contains violation entries. The log is checked when the worktree is bound to this
// issue OR unbound (an unbound worktree on the issue's branch is exactly the
// enforcement-gap case violations exist to catch — finding 1); a worktree bound to
// a different issue is skipped.
func issueWorktreeHasViolations(repoPath string, issue materialize.Issue) (bool, error) {
	gitDir, binding, ok, err := findGateTarget(repoPath, issue)
	if err != nil {
		// Inventory/git-dir unreadable, or ambiguous bindings: fail closed — the
		// caller must refuse to mark merged rather than silently treat this as
		// "no violations".
		return false, err
	}
	if !ok {
		if issue.Status == ops.StatusDone {
			return false, fmt.Errorf("no worktree or hook-log target exists for done issue %s", issue.ID)
		}
		return false, nil
	}
	if binding != "" && binding != issue.ID {
		// Bound to a different issue; not ours to gate on.
		return false, nil
	}
	return readHookLogForViolations(gitDir)
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

// removeClaimExclusionAfterWorktreeRemoval removes a custom exclusion only
// after Git has removed the linked worktree. The exclusion lock serializes this
// check with claim-time exclusion updates, and the live inventory check keeps a
// concurrently recreated destination protected.
func removeClaimExclusionAfterWorktreeRemoval(repoPath, destination, pattern string) error {
	if pattern == "" {
		return nil
	}
	release, err := acquireGitExcludeLock(repoPath)
	if err != nil {
		return err
	}
	defer release()
	return removeClaimExclusionAfterWorktreeRemovalLocked(repoPath, destination, pattern)
}

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
	// Removal is destructive, so it resolves by binding ONLY — never through
	// findGateTarget's branch widening. An unbound worktree that merely holds
	// this issue's branch may be a user's own checkout; the gate may read it,
	// but nothing may remove it.
	worktrees, err := worktree.List(repoPath)
	if err != nil {
		return worktreeSkipped, fmt.Errorf("read worktree inventory: %w", err)
	}
	item, res, err := resolveBoundWorktree(worktrees, issue)
	if err != nil {
		// Bindings ambiguous: fail closed so a removal is never guessed at, and
		// never silently skipped as "not found".
		return worktreeSkipped, err
	}
	if res != worktree.Bound {
		// Nothing carries this issue's binding, so nothing may be removed. If an
		// unbound worktree is nonetheless sitting on the issue's branch, say so:
		// it is the worktree the operator probably expected to be torn down, and
		// silence would read as "already gone". This is a report, not a target —
		// the path is never passed to removal.
		if path := unboundWorktreeOnBranch(worktrees, issue); path != "" {
			_, _ = fmt.Fprintf(errWriter,
				"Warning: worktree at %s is on branch %s but not bound to %s; skipping removal\n",
				path, materialize.DeriveBranchName(issue.Type, issue.ID), issue.ID)
		}
		return worktreeSkipped, nil
	}
	return removeWorktreeAtPathTracked(repoPath, issue, item.Path, errWriter)
}

// removeWorktreeAtPathTracked removes exactly selectedPath after refreshing the
// inventory and revalidating its binding. GC passes the path selected by
// reconciliation; it must not re-resolve the issue's worktree by binding alone
// because a legacy and a canonical worktree can share one binding identity.
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
	hasPassThroughs, err := readHookLogForPassThroughs(gitDir)
	if err != nil {
		return worktreeSkipped, fmt.Errorf("read hook log for %s: %w", issue.ID, err)
	}
	if hasPassThroughs {
		_, _ = fmt.Fprintf(errWriter, "Warning: %s has pass-through entries in armature-hook.log\n", issue.ID)
	}

	// Read immutable claim provenance before removal while the worktree's
	// git-dir record still exists. A current scratch branch is not evidence of
	// the branch the issue was claimed under.
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

	// Normal teardown is deliberately non-force: dirty or locked worktrees
	// retain both their content and all provenance for recovery.
	if err := gitClient.RemoveWorktree(selected.Path); err != nil {
		releaseClaimExclusionLock()
		return worktreeSkipped, fmt.Errorf("remove worktree for %s: %w", issue.ID, err)
	}
	// Per-worktree metadata disappeared with the successful removal; only the
	// shared config must be cleared afterwards.
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
