package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scullxbones/armature/internal/adapters"
	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/deliverygate"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/worktree"
)

// resolveWorktreeGitDir resolves the actual git directory for a worktree path.
// It is used by both claim and harness-hook so that both read from the same
// location. Delegates to internal/deliverygate.ResolveWorktreeGitDir, the
// single source of truth also used by the delivery gate's read-side
// base-commit resolution.
func resolveWorktreeGitDir(worktreePath string) (string, error) {
	return deliverygate.ResolveWorktreeGitDir(worktreePath)
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

// isWorktreeOf checks if a worktree at worktreePath is registered to the git
// repository at repoPath. It uses the shared marker-aware inventory so claim's
// foreign-repository guard cannot drift from list, GC, merged, or Doctor.
func isWorktreeOf(repoPath, worktreePath string) bool {
	worktrees, err := worktree.List(repoPath)
	if err != nil {
		return false
	}
	target := worktree.NormalizePath(worktreePath)
	for _, item := range worktrees {
		if worktree.NormalizePath(item.Path) == target {
			return true
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

	// Use the legacy-aware binding reader that falls back from armature-issue-id to armature-task-id.
	// This handles worktrees claimed before the rename to armature-issue-id. Unlike
	// ReadIssueBindingFile, the Err variant surfaces non-ENOENT read errors (e.g.
	// permission denied) so a binding file we can't read is not silently treated
	// as unbound, restoring the old fail-closed behavior.
	existingIssueID, err := harnesshook.ReadIssueBindingFileErr(actualGitDir)
	if err != nil {
		return fmt.Errorf("read existing binding: %w", err)
	}
	if existingIssueID != "" && existingIssueID != issueID {
		return fmt.Errorf("worktree at %s is already bound to %s: use a different --worktree path",
			worktreePath, existingIssueID)
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
		if existingIssueID == issueID {
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
// Delegates to materialize.DeriveBranchName, the shared implementation also used
// by internal/doctor for missing-worktree detection.
func deriveBranchName(issueType, issueID string) string {
	return materialize.DeriveBranchName(issueType, issueID)
}

// canonicalWorktreePath validates the issue ID before it is used in any
// filesystem or git operation. Slash-bearing IDs are supported, but absolute
// IDs and traversal outside the repository's canonical .worktrees root are
// rejected before the claim op is appended.
func canonicalWorktreePath(repoPath, issueID string) (string, error) {
	if issueID == "" || filepath.IsAbs(issueID) {
		return "", fmt.Errorf("invalid issue ID %q for canonical worktree path", issueID)
	}
	// Reject "." / ".." path components in the ID. The filepath.Rel containment
	// check below catches escapes, but an ID like "team/../task-1" cleans to
	// "task-1" — it stays under the root yet aliases the distinct valid ID
	// "task-1", so both would wedge at the same .worktrees/task-1 path. Upstream
	// (arm create / dag apply) does not reject "."/".." in IDs, so this guard is
	// reachable and must keep ID→path injective. Split on both "/" (the ID's own
	// separator) and the OS separator.
	for _, seg := range strings.FieldsFunc(issueID, func(r rune) bool {
		return r == '/' || r == filepath.Separator
	}) {
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("issue ID %q must not contain '.' or '..' path components", issueID)
		}
	}
	root := worktree.CanonicalRoot(repoPath)
	path := worktree.CanonicalPath(repoPath, issueID)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("issue ID %q escapes canonical worktree root %s", issueID, root)
	}
	return path, nil
}

// addWorktreeDetached provisions a linked worktree at worktreePath checked out
// detached at baseRef (a SHA or ref). Using a detached checkout means no branch
// is held by the new worktree yet, so a subsequent branch checkout inside it
// cannot trip git's "branch already checked out" guard. Uses raw git rather than
// the adapter so this reordering stays within cmd/armature (the adapter's
// AddWorktree only supports the branch-first form).
func addWorktreeDetached(repoPath, worktreePath, baseRef string) error {
	addArgs := []string{"worktree", "add", "--detach", worktreePath, baseRef}
	// If the managed worktree directory was deleted out from under git, git keeps
	// the administrative registration and marks it prunable. worktree.List skips
	// prunable blocks, so the adoption loop never sees the path and a plain
	// `git worktree add <path>` fails with "missing but already registered
	// worktree", leaving every re-claim to loop. Clear that stale registration
	// with an exact-path `add --force` (git's documented fix for this exact
	// error) rather than a broad `git worktree prune`, which could drop unrelated
	// registrations.
	prunable, err := worktree.HasPrunableRegistration(repoPath, worktreePath)
	if err != nil {
		return fmt.Errorf("check prunable worktree registration: %w", err)
	}
	if prunable {
		addArgs = []string{"worktree", "add", "--force", "--detach", worktreePath, baseRef}
	}
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repoPath}, addArgs...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add --detach: %w\n%s", err, out)
	}
	return nil
}

// checkoutBranchInWorktree creates or checks out branchName inside the worktree
// at worktreePath. If the branch already exists it is checked out as-is; if not,
// it is created at the worktree's current (detached) HEAD. Idempotent: a no-op
// when the worktree is already on branchName.
func checkoutBranchInWorktree(worktreePath, branchName string) error {
	// Fast-path idempotency and existing-branch handling: if the branch already
	// exists, check it out; otherwise create it from the current detached HEAD.
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	verify := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "rev-parse", "--verify", "refs/heads/"+branchName)
	branchExists := verify.Run() == nil

	args := []string{"-C", worktreePath, "checkout"}
	if branchExists {
		args = append(args, branchName)
	} else {
		args = append(args, "-b", branchName)
	}
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", branchName, err, out)
	}
	return nil
}

// priorClaimState captures the issue's claim-related fields as they were before
// this claim's op was appended, so a failed post-claim worktree setup step can
// decide whether to keep the prior status or release the claim.
type priorClaimState struct {
	status                 string
	claimedBy              string
	claimedAt              int64
	lastHeartbeat          int64
	claimTTL               int
	claimingWorkerActivity int64
	// worktreePath is the issue's WorktreePath BEFORE this claim's op overwrote
	// it with the canonical .worktrees/<issue-id> path. On a same-worker active
	// retry over a legacy differently-pathed worktree, a provisioning failure
	// must restore this so the still-active claim doesn't point at a path that
	// was just removed, orphaning the legacy worktree.
	worktreePath string
}

// rollbackClaim releases (or restores) the claim after a post-claim worktree
// setup step fails, then returns the error to surface. A same-worker ACTIVE
// claim keeps its prior status; a stale same-worker claim or a different-worker
// takeover is released to open. opLabel names the failed step in the returned
// error. Shared by the create-worktree and update-issue-ID failure paths.
func rollbackClaim(cmd *cobra.Command, logPath, issueID, workerID, opLabel string, cause error, prior priorClaimState) error {
	rollbackStatus := ops.StatusOpen
	priorWasActive := prior.claimedBy == workerID &&
		!claimPkg.IsClaimStale(prior.claimedAt, prior.lastHeartbeat, prior.claimingWorkerActivity, prior.claimTTL, nowEpoch())
	payload := ops.Payload{To: rollbackStatus}
	if priorWasActive {
		rollbackStatus = prior.status
		payload.To = rollbackStatus
	}
	// The claim op already refreshed all lease timestamps before worktree setup
	// ran. Carry the complete pre-claim snapshot in the compensating op so replay
	// restores the exact lease, rather than only status and path. For stale or
	// foreign takeovers, the zero-valued snapshot deliberately clears every
	// lease field while the status transition releases the claim.
	payload.RestoreClaim = true
	if priorWasActive {
		payload.RestoreClaimedBy = prior.claimedBy
		payload.RestoreClaimedAt = prior.claimedAt
		payload.RestoreClaimTTL = prior.claimTTL
		payload.RestoreLastHeartbeat = prior.lastHeartbeat
		payload.RestoreLastClaimingWorkerActivity = prior.claimingWorkerActivity
	}
	// Always restore the pre-claim path, regardless of whether the failed claim
	// was a first claim, stale takeover, or active retry. The claim op is
	// append-only and materializes its canonical path before provisioning; if
	// provisioning fails that path may have been removed, so retaining it would
	// leave state pointing at a nonexistent worktree. An explicit clear signal
	// preserves an empty legacy path instead of being mistaken for "no change".
	if prior.worktreePath != "" {
		payload.WorktreePath = prior.worktreePath
	} else {
		payload.ClearWorktreePath = true
	}
	rollbackOp := ops.Op{
		Type:      ops.OpTransition,
		TargetID:  issueID,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   payload,
	}
	if rbErr := appendHighStakesOp(mustState(cmd), logPath, rollbackOp); rbErr != nil {
		return fmt.Errorf("%s: %w; also failed to push claim release: %v (manual cleanup may be needed)", opLabel, cause, rbErr)
	}
	return fmt.Errorf("%s: %w (claim released; retry arm claim)", opLabel, cause)
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

	// Resolve HEAD before branching: this is the actual point the task branch
	// diverges from the coordinator's checkout, which may already be a story
	// branch containing completed sibling-task commits (not necessarily main).
	// Persisted below so the delivery gate can scope-check against the real
	// branch-point instead of guessing via merge-base against a default branch.
	headSHA, headErr := gitClient.ResolveRevision("HEAD")

	// Capture the name of the branch this task branch is being cut from
	// (the coordinator's current checkout — often a story branch). This is
	// persisted as git config on the *main repo* (shared across all linked
	// worktrees, not per-worktree), so it survives worktree removal/recreation
	// (e.g. via `arm merged`'s RemoveWorktree) and lets the delivery gate
	// recompute the branch-point dynamically via merge-base at check time —
	// which also self-corrects if the task branch is later rebased onto an
	// updated parent tip, instead of trusting a SHA recorded once at claim time.
	parentBranch, parentErr := gitClient.CurrentBranch()

	// Provision the worktree detached at the base commit FIRST, then create or
	// check out the issue branch inside the worktree. The old order
	// (create-branch-then-add-worktree) could hit git's "branch already checked
	// out" failure when the branch pre-existed; provisioning detached and then
	// checking the branch out inside the worktree avoids that entirely.
	detachRef := "HEAD"
	if headErr == nil && headSHA != "" {
		detachRef = headSHA
	}
	adopted := false
	adoptedFrom := ""
	// Deliberate checked-out-branch policy: if the issue already owns a bound
	// worktree elsewhere, adopt it by moving the registered worktree to the
	// canonical path. This preserves its branch and uncommitted files and avoids
	// Git's branch-already-checked-out guard.
	//
	// Selection is by BINDING, never by branch. A worktree bound to this issue is
	// this issue's worktree whether it is on the issue branch, detached mid-rebase,
	// or parked on a scratch branch; filtering on branch here would skip it and
	// provision a SECOND canonical worktree for the same issue. The materialized
	// path would then select the new one at `arm merged`, leaving the original as
	// the sole terminal GC candidate for a `git worktree remove --force` that
	// discards whatever in-flight work it still held.
	//
	// An unbound or differently-bound worktree is never adopted; the
	// detached-first path will fail closed and clean up its temporary
	// registration instead.
	inventory, inventoryErr := worktree.List(repoPath)
	if inventoryErr != nil {
		return fmt.Errorf("inspect existing worktrees: %w", inventoryErr)
	}
	// Collect the FULL bound set before deciding anything. Stopping at the first
	// match would adopt whichever worktree git happens to list first and never
	// notice a second one carrying the same binding — leaving that duplicate
	// behind as a force-removable candidate holding in-flight work. Ambiguity
	// must be observed before it can be refused.
	var bound []worktree.Meta
	for _, item := range inventory {
		if item.IssueID == issueID {
			bound = append(bound, item)
			continue
		}
		// Someone else's worktree holding our branch is an error, but only
		// because of the branch: an unrelated worktree elsewhere is fine.
		if item.Branch == "refs/heads/"+branchName {
			return fmt.Errorf("branch %s is already checked out at %s; bind that worktree to %s before claiming", branchName, item.Path, issueID)
		}
	}
	if len(bound) > 1 {
		// Same condition worktree.SelectByIssue reports as Ambiguous and gc
		// reports as GCAmbiguous. Adoption cannot pick one without guessing, and
		// the loser of a guess stays behind as a --force removal candidate.
		paths := make([]string, 0, len(bound))
		for _, item := range bound {
			paths = append(paths, item.Path)
		}
		slices.Sort(paths)
		return fmt.Errorf(
			"issue %s is bound to %d worktrees (%s); remove the armature-issue-id binding from the ones you do not want before claiming",
			issueID, len(bound), strings.Join(paths, ", "))
	}
	// At most one bound worktree survives the ambiguity check above.
	if len(bound) == 1 {
		item := bound[0]
		switch {
		case worktree.NormalizePath(item.Path) == worktree.NormalizePath(worktreePath):
			// Already at the canonical path; nothing to adopt.
		case item.Branch != "refs/heads/"+branchName:
			// Bound to this issue but not on the issue branch: detached (very
			// likely mid-rebase) or moved to a scratch branch. FAIL CLOSED.
			//
			// Neither recovery is safe to automate. Moving it would relocate a
			// working directory whose in-progress operation state under
			// .git/worktrees/<n>/rebase-merge holds absolute paths that
			// `git worktree move` does not rewrite; checking the branch out
			// afterwards would destroy the very rebase the worker is running.
			// Refusing preserves the uncommitted work by not acting on it, and
			// matches how ambiguity is handled everywhere else in this lifecycle
			// (I5: deterministic gates fail closed, a human disambiguates).
			head := item.Branch
			if head == "" {
				head = "detached HEAD"
			}
			return fmt.Errorf(
				"worktree at %s is bound to %s but is on %s, not %s; finish or abandon the in-progress git operation there and check out %s before claiming",
				item.Path, issueID, head, branchName, branchName)
		default:
			_, statErr := os.Stat(worktreePath)
			switch {
			case statErr == nil:
				// Canonical path already occupied; leave the bound worktree alone.
			case !os.IsNotExist(statErr):
				return fmt.Errorf("check canonical worktree path: %w", statErr)
			default:
				if err := os.MkdirAll(filepath.Dir(worktreePath), 0o750); err != nil {
					return fmt.Errorf("create canonical worktree root: %w", err)
				}
				if err := gitClient.MoveWorktree(item.Path, worktreePath); err != nil {
					return fmt.Errorf("adopt bound worktree: %w", err)
				}
				adopted = true
				adoptedFrom = item.Path
			}
		}
	}
	if !adopted {
		if err := addWorktreeDetached(repoPath, worktreePath, detachRef); err != nil {
			return fmt.Errorf("add worktree: %w", err)
		}
	}

	// From here on the worktree exists on disk but is not yet fully provisioned
	// (unbound and/or detached). Any subsequent failure must remove it before
	// returning: a leftover unbound+detached worktree at .worktrees/<issue-id>
	// blocks a later re-claim (the path already exists but isn't bound to the
	// issue). Cleanup is best-effort and logged if it itself fails.
	cleanupPartialWorktree := func(cause error, label string) error {
		if adoptedFrom != "" {
			if moveErr := gitClient.MoveWorktree(worktreePath, adoptedFrom); moveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to restore adopted worktree at %s: %v\n", adoptedFrom, moveErr)
			}
		} else if rmErr := gitClient.RemoveWorktree(worktreePath); rmErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to clean up partial worktree at %s: %v\n", worktreePath, rmErr)
		}
		return fmt.Errorf("%s: %w", label, cause)
	}

	// Create-or-checkout the issue branch inside the worktree. Because the
	// worktree is detached, no other worktree holds the branch, so this never
	// trips git's "branch already checked out" guard.
	if !adopted {
		if err := checkoutBranchInWorktree(worktreePath, branchName); err != nil {
			return cleanupPartialWorktree(err, "checkout branch in worktree")
		}
	}

	if adopted {
		// Adoption moves a pre-existing worktree; it must never turn a
		// merge-base against a default ref into claim-time provenance. Only
		// metadata recorded by the original claim is trusted. Without it,
		// reject the move and leave the legacy worktree where it was so the
		// delivery gate cannot later misattribute sibling commits.
		adoptedGitClient := adapters.New(worktreePath)
		if !hasTrustedBranchPointMetadata(adoptedGitClient, worktreePath, branchName) {
			return cleanupPartialWorktree(fmt.Errorf(
				"adopted worktree has no recorded branch-point provenance; re-claim it from a managed worktree or use --skip-delivery-gate only with an explicit override"),
				"adopt worktree")
		}
	}

	// Best-effort project-isolation mitigation: if the MAIN tree uses a go.work
	// file, drop this worktree from its `use` directives so the main tree's
	// gopls does not walk the worktree. A no-op when there is no main-tree
	// go.work (the common case). Non-fatal: a failure only degrades IDE
	// ergonomics, so it must never fail the claim.
	if err := worktree.ApplyMitigations(repoPath, worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: apply worktree mitigations: %v\n", err)
	}

	// Create the issue ID file in the worktree's .git directory
	if err := updateIssueIDFile(worktreePath, issueID); err != nil {
		return cleanupPartialWorktree(err, "write issue ID file")
	}

	if adopted {
		// Keep any existing base/parent metadata untouched. The only new
		// claim-time marker that is safe to add is the immutable branch binding.
		if err := writeClaimedBranchFileIfAbsent(worktreePath, branchName); err != nil {
			return cleanupPartialWorktree(err, "persist claimed branch metadata")
		}
	} else if err := persistBranchPointMetadata(gitClient, worktreePath, branchName, headSHA, headErr, parentBranch, parentErr); err != nil {
		return cleanupPartialWorktree(err, "persist branch-point metadata")
	}
	return nil
}

// hasTrustedBranchPointMetadata reports whether a pre-existing worktree carries
// claim-time provenance that the delivery gate can use. Default-branch
// merge-bases are intentionally not considered: the current repository shape
// cannot prove where an adopted branch was originally cut.
func hasTrustedBranchPointMetadata(gitClient *adapters.Client, worktreePath, branchName string) bool {
	if _, err := deliverygate.RecordedBaseCommit(worktreePath); err == nil {
		return true
	}
	parentBranch, err := gitClient.ReadGitConfig(parentBranchConfigKey(branchName))
	return err == nil && parentBranch != "" && parentBranch != "HEAD"
}

// persistBranchPointMetadata records the branch-point metadata (parent branch
// git config, base-commit file) the delivery gate later reads via
// dynamicBaseCommit/recordedBaseCommit in transition.go. Both idempotent:
// safe to call whether or not either record already exists. It is used only
// for newly provisioned worktrees, where HEAD and the current parent branch
// are observed at the moment Armature creates the worktree. Pre-existing and
// adopted worktrees must preserve their original records or fail closed; the
// current repository's default refs are not provenance.
func persistBranchPointMetadata(
	gitClient *adapters.Client,
	worktreePath, branchName string,
	headSHA string, headErr error,
	parentBranch string, parentErr error,
) error {
	// Persist the parent branch name, but only if not already recorded: claim
	// is idempotent and may re-run after the worktree (but not the branch) was
	// removed, in which case gitClient.CurrentBranch() here would return
	// whatever the coordinator happens to be on *now* — not the original
	// parent — so an existing record must never be overwritten.
	// "HEAD" is the literal string CurrentBranch() returns when the
	// coordinator repo is in a detached-HEAD state (git rev-parse
	// --abbrev-ref HEAD prints "HEAD" itself, not a branch name). Persisting
	// that as the parent branch would later make the delivery gate resolve
	// the ref "HEAD" in the task worktree — the task's own current commit —
	// collapsing the merge-base to the task's HEAD and making every commit
	// range for CommitReferenceCheck empty. Treat it as no usable parent
	// branch so nothing is persisted, falling back to the existing
	// no-parent-branch-config behavior.
	if parentErr == nil && parentBranch != "" && parentBranch != "HEAD" {
		if err := writeParentBranchConfigIfAbsent(gitClient, branchName, parentBranch); err != nil {
			return fmt.Errorf("write parent branch config: %w", err)
		}
	}

	// Persist the branch-point SHA if it was already claimed (idempotent claim
	// re-runs against an existing branch skip this: HEAD may have moved since
	// the branch was first created, and re-persisting would overwrite the true
	// original branch-point with a later, incorrect value).
	if headErr == nil {
		if err := writeBaseCommitFileIfAbsent(worktreePath, headSHA); err != nil {
			return fmt.Errorf("write base commit file: %w", err)
		}
	}

	// Persist the branch the issue was actually claimed under, immutably, so
	// the delivery gate can later verify branch binding against what was
	// claimed rather than re-deriving it from the CURRENT (possibly
	// since-amended) issue type. branchName is already the branch derived
	// from the issue's type at claim time (DeriveBranchName), so it's the
	// correct value to record here for both the fresh-worktree and
	// existing-worktree claim paths.
	if err := writeClaimedBranchFileIfAbsent(worktreePath, branchName); err != nil {
		return fmt.Errorf("write claimed branch file: %w", err)
	}

	return nil
}

// baseCommitFileName is the name of the file (written into a worktree's
// actual git directory, alongside armature-issue-id) that records the SHA
// the task branch diverged from at claim time. The delivery gate reads this
// (see internal/deliverygate.RecordedBaseCommit) to scope-check against the
// real branch-point rather than merge-basing against a default branch, which
// is wrong whenever the task branch was cut from a story branch containing
// completed sibling-task commits. Aliased to the deliverygate constant so
// the write side (here) and read side (deliverygate) can never drift apart.
const baseCommitFileName = deliverygate.BaseCommitFileName

// parentBranchConfigKey returns the git config key used to durably record,
// on the shared (main-repo) git config, the branch a task branch was cut
// from. Recorded as git config rather than a per-worktree file: git config
// --local written from a linked worktree lands in the main repo's shared
// .git/config (armature does not enable the worktreeConfig extension), so
// the record survives `arm merged` removing the worktree, and stays
// addressable by branch name if the worktree is later recreated. Delegates
// to internal/deliverygate.ParentBranchConfigKey, the same key the delivery
// gate's read side (DynamicBaseCommit) resolves.
func parentBranchConfigKey(branchName string) string {
	return deliverygate.ParentBranchConfigKey(branchName)
}

// writeParentBranchConfigIfAbsent records parentBranch as the branch
// branchName diverged from, but only if no such record exists yet — the
// same idempotency guard as writeBaseCommitFileIfAbsent, and for the same
// reason: an existing record reflects the true original parent and must
// never be overwritten by a later, possibly different, "current branch".
func writeParentBranchConfigIfAbsent(gitClient *adapters.Client, branchName, parentBranch string) error {
	key := parentBranchConfigKey(branchName)
	if existing, err := gitClient.ReadGitConfig(key); err == nil && existing != "" {
		return nil
	}
	return gitClient.SetGitConfig(key, parentBranch)
}

// writeBaseCommitFileIfAbsent records headSHA as the task branch's
// branch-point, but only if no such record exists yet. Claim is idempotent
// and may be re-run against an already-created branch; without the
// absence-check, a later HEAD would silently overwrite the true origin
// branch-point.
func writeBaseCommitFileIfAbsent(worktreePath, headSHA string) error {
	actualGitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree git dir: %w", err)
	}

	baseCommitFile := filepath.Join(actualGitDir, baseCommitFileName)
	if _, err := os.Stat(baseCommitFile); err == nil {
		// Already recorded from the original claim; leave it alone.
		return nil
	}

	if err := os.WriteFile(baseCommitFile, []byte(headSHA), 0o600); err != nil {
		return fmt.Errorf("write base commit file: %w", err)
	}
	return nil
}

// claimedBranchFileName is the name of the file (written into a worktree's
// actual git directory, alongside armature-base-commit) that records the
// branch name the issue was claimed under. Aliased to the deliverygate
// constant so the write side (here) and read side (deliverygate) can never
// drift apart, matching the baseCommitFileName pattern above.
const claimedBranchFileName = deliverygate.ClaimedBranchFileName

// writeClaimedBranchFileIfAbsent records branchName as the branch the issue
// was claimed under, but only if no such record exists yet (the same
// idempotency guard as writeBaseCommitFileIfAbsent, and for the same reason:
// an existing record reflects the true original claim and must never be
// overwritten by a later, possibly different, value). Skips writing entirely
// when branchName is "" — a legitimately branchless issue type (e.g. epic,
// story before it gets a worktree) has nothing to record, and writing an
// empty marker would be indistinguishable from "not yet recorded" on the
// read side, defeating the pre-migration fallback in
// deliverygate.RecordedClaimedBranch.
func writeClaimedBranchFileIfAbsent(worktreePath, branchName string) error {
	if branchName == "" {
		return nil
	}

	actualGitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree git dir: %w", err)
	}

	claimedBranchFile := filepath.Join(actualGitDir, claimedBranchFileName)
	if _, err := os.Stat(claimedBranchFile); err == nil {
		// Already recorded from the original claim; leave it alone.
		return nil
	}

	if err := os.WriteFile(claimedBranchFile, []byte(branchName), 0o600); err != nil {
		return fmt.Errorf("write claimed branch file: %w", err)
	}
	return nil
}

// clearBranchPointMetadata unsets the persisted parent-branch git config and
// removes the base-commit file for branchName/worktreePath. Called from `arm
// merged` alongside RemoveWorktree so that if the branch is later deleted and
// the same branch name is reused for a genuinely different parent, the
// "if absent" guards in writeParentBranchConfigIfAbsent/
// writeBaseCommitFileIfAbsent don't see a stale leftover value and skip
// recording the fresh, correct one. Must be called BEFORE gitClient.RemoveWorktree,
// since resolveWorktreeGitDir needs the worktree to still exist to locate its
// git directory. Best-effort: errors are ignored, matching the rest of the
// cleanup in this area (RemoveWorktree failures are the only ones that block
// `arm merged`).
func clearBranchPointMetadata(gitClient *adapters.Client, worktreePath, branchName string) {
	_ = gitClient.UnsetGitConfig(parentBranchConfigKey(branchName)) //nolint:errcheck // best-effort cleanup
	if actualGitDir, err := resolveWorktreeGitDir(worktreePath); err == nil {
		_ = os.Remove(filepath.Join(actualGitDir, baseCommitFileName))    //nolint:errcheck // best-effort cleanup
		_ = os.Remove(filepath.Join(actualGitDir, claimedBranchFileName)) //nolint:errcheck // best-effort cleanup
	}
}

// updateIssueIDFile writes the issue ID to the armature-issue-id file in the worktree's .git directory.
// In a git worktree, .git is a file (not a directory) that points to the actual git directory.
// We use resolveWorktreeGitDir to find the real git directory.
func updateIssueIDFile(worktreePath, issueID string) error {
	actualGitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree git dir: %w", err)
	}

	// Write the issue ID file to the actual git directory
	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	if err := os.WriteFile(issueIDFile, []byte(issueID), 0o600); err != nil {
		return fmt.Errorf("write issue ID file: %w", err)
	}
	return nil
}

func newClaimCmd() *cobra.Command {
	var issueID string
	var ttl int
	var force bool
	var worktreeFlag bool
	var worktreePath string

	cmd := &cobra.Command{
		Use:   "claim [issue-id]",
		Short: "Claim a ready task",
		Long: `Claim an issue to assign it to the current worker.

Claiming an issue marks it as assigned to your worker ID and sets a TTL (time-to-live).
If the TTL expires without progress, the claim becomes stale and may be reassigned.
This command also detects and warns about scope overlaps with concurrently claimed issues.
When you claim a task, its parent story (if open) is automatically advanced to in-progress.
The --worktree flag is required; it provisions a worktree at .worktrees/<issue-id>
(relative to the repo root) with an issue-specific branch if absent, or updates the
armature-issue-id file if the worktree already exists.`,
		Example: `  # Claim an issue by ID with a worktree
  $ arm claim E6-S4-T2 --worktree

  # Claim with a custom TTL of 120 minutes
  $ arm claim --issue E6-S4-T2 --ttl 120 --worktree

  # Claim despite scope overlap warning
  $ arm claim E6-S4-T2 --force --worktree

  # Claim using flag style
  $ arm claim --issue another-task-id --worktree`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)
			var err error
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}
			if !worktreeFlag {
				return fmt.Errorf("--worktree is required")
			}

			// Validate and resolve the canonical path before any claim append or
			// filesystem/git mutation. This permits slash-bearing IDs but rejects
			// absolute and traversal IDs that would escape .worktrees.
			worktreePath, err = canonicalWorktreePath(ctx.RepoPath, issueID)
			if err != nil {
				return err
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
			snapshot, err := store.Load(context.Background())
			if err != nil {
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
			// This prevents writing armature-issue-id into a foreign repo's git dir,
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
			prior := priorClaimState{
				status:                 issue.Status,
				claimedBy:              issue.ClaimedBy,
				claimedAt:              issue.ClaimedAt,
				lastHeartbeat:          issue.LastHeartbeat,
				claimTTL:               issue.ClaimTTL,
				claimingWorkerActivity: issue.LastClaimingWorkerActivity,
				worktreePath:           issue.WorktreePath,
			}

			index := store.Index()
			// Build a graph from the materialized state for ancestor/descendant checking
			graph := materialize.GraphFromState(snapshot.State)

			for id, entry := range index {
				if id == issueID || (entry.Status != ops.StatusClaimed && entry.Status != ops.StatusInProgress) {
					continue
				}
				if claimPkg.ScopesOverlapEx(issue.Scope, entry.Scope, graph, issueID, id) {
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
				WorkerID: workerID, Payload: ops.Payload{TTL: ttl, WorktreePath: worktreePath},
			}
			if err := appendHighStakesOp(mustState(cmd), logPath, op); err != nil {
				return err
			}

			// Refresh store after appending claim Op
			if _, err := store.Load(context.Background()); err != nil {
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
					return rollbackClaim(cmd, logPath, issueID, workerID, "create worktree", err, prior)
				}
			} else {
				// Worktree exists and binding was already validated above; update the
				// task ID file to ensure the binding is current (idempotent).
				if err := updateIssueIDFile(worktreePath, issueID); err != nil {
					return rollbackClaim(cmd, logPath, issueID, workerID, "update task ID file", err, prior)
				}

				// A pre-existing canonical worktree may already carry trusted
				// claim-time provenance. Preserve it; never synthesize a base from
				// origin/main, main, or another current default ref. If it lacks
				// provenance, the claim remains replay-compatible but the delivery
				// gate will fail closed with an explicit re-claim/override action.
				// This keeps legacy worktrees usable for inspection without turning
				// the current repository shape into false evidence.
				worktreeGitClient := adapters.New(worktreePath)
				if hasTrustedBranchPointMetadata(worktreeGitClient, worktreePath, expectedBranch) {
					if err := writeClaimedBranchFileIfAbsent(worktreePath, expectedBranch); err != nil {
						return rollbackClaim(cmd, logPath, issueID, workerID, "persist claimed branch metadata", err, prior)
					}
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
	cmd.Flags().BoolVar(&worktreeFlag, "worktree", false, "provision a worktree at .worktrees/<issue-id> (required)")
	return cmd
}
