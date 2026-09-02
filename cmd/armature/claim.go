package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/issueid"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/worktree"
)

const codeClaim1 = "CLAIM-1"

func init() {
	armerrors.Register(codeClaim1)
}

// mapClaimError presents claim RunE errors as a Command Failure at the CLI
// port. Core helpers still return ordinary errors.
func mapClaimError(err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "issue ID is required"),
		strings.Contains(msg, "issue ID") && strings.Contains(msg, "must not"),
		strings.Contains(msg, "--worktree is required"),
		strings.Contains(msg, "--from requires an explicit --worktree"),
		strings.Contains(msg, "accepts at most"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm claim --help"}, 2, err)
	case strings.Contains(msg, "issue") && strings.Contains(msg, "not found") &&
		!strings.Contains(msg, "after claim"):
		return armerrors.Wrap(codeClaim1, msg, []string{"arm ready", "arm list"}, 1, err)
	case strings.Contains(msg, "use --force"):
		return armerrors.Wrap(codeClaim1, msg, []string{"arm claim --force --worktree"}, 1, err)
	case strings.Contains(msg, "is not an existing worktree of this repository"),
		strings.Contains(msg, "--from worktree") && strings.Contains(msg, "must be on a branch"):
		return armerrors.Wrap(codeClaim1, msg, []string{
			"arm claim --worktree <new-path> --from <existing-branch-attached-worktree>",
		}, 1, err)
	case strings.Contains(msg, "confidence=inferred"):
		return armerrors.Wrap(codeClaim1, msg, []string{"arm confirm <node-id>"}, 1, err)
	default:
		return armerrors.Wrap(codeClaim1, msg, []string{"arm doctor", "arm show"}, 1, err)
	}
}

// defaultWorktreeFlagValue preserves the established value-less --worktree
// form while allowing --worktree <path> for a caller-selected new worktree.
const defaultWorktreeFlagValue = ".armature-default-worktree"

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
// repository at repoPath. It uses the shared binding-aware inventory so claim's
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
// filesystem or git operation. Slash-bearing IDs are rejected to ensure
// prefix-free worktree paths — one managed worktree can never contain another.
// Absolute, traversal, and separator-bearing IDs are rejected before the claim
// op is appended.
func canonicalWorktreePath(repoPath, issueID string) (string, error) {
	if err := issueid.Validate(issueID); err != nil {
		return "", err
	}
	root := worktree.CanonicalRoot(repoPath)
	path := worktree.CanonicalPath(repoPath, issueID)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("issue ID %q escapes canonical worktree root %s", issueID, root)
	}
	return path, nil
}

// rejectInRepositoryCustomWorktreeDestination prevents an explicit worktree
// path inside the coordinator repository from using a clone-wide
// .git/info/exclude entry. That file is shared by every linked worktree, so a
// path-specific entry for an arbitrary repository-relative destination could
// silently hide unrelated files from another worktree. The canonical
// .worktrees root is safe because its one shared exclusion is an established
// repository invariant.
func rejectInRepositoryCustomWorktreeDestination(repoPath, destination string) error {
	repoRoot := worktree.NormalizePath(repoPath)
	normalizedDestination := worktree.NormalizePathAllowingMissing(destination)
	if !worktree.IsUnderRoot(normalizedDestination, repoRoot) {
		return nil
	}
	if !worktree.IsUnderRoot(normalizedDestination, worktree.CanonicalRoot(repoPath)) {
		return fmt.Errorf(
			"custom worktree destination %s is inside the repository; explicit destinations must be outside the repository or under canonical .worktrees",
			destination)
	}
	return nil
}

// rejectNestedWorktreeDestination refuses a custom destination below any
// linked worktree registered in this clone. The main coordinator worktree is
// intentionally absent from RegisteredPaths, so repository-relative custom
// destinations remain allowed there. Git interprets info/exclude relative to
// each worktree root; nesting would otherwise let the parent stage the child
// checkout as an embedded repository.
func rejectNestedWorktreeDestination(repoPath, destination string) error {
	registeredPaths, err := worktree.RegisteredPaths(repoPath)
	if err != nil {
		return fmt.Errorf("inspect registered worktree destinations: %w", err)
	}
	normalizedDestination := worktree.NormalizePathAllowingMissing(destination)
	for _, registeredPath := range registeredPaths {
		normalizedRegisteredPath := worktree.NormalizePathAllowingMissing(registeredPath)
		if normalizedDestination == normalizedRegisteredPath {
			prunable, err := worktree.HasPrunableRegistration(repoPath, registeredPath)
			if err != nil {
				return fmt.Errorf("inspect registered worktree %s: %w", registeredPath, err)
			}
			if prunable {
				continue
			}
		}
		if worktree.IsUnderRoot(normalizedDestination, normalizedRegisteredPath) {
			return fmt.Errorf("custom worktree destination %s is nested inside registered worktree %s", destination, registeredPath)
		}
	}
	return nil
}

func sourceAdvancedOnlyByArmature(repoPath, sourcePath, oldTip, newTip string) (bool, error) {
	if worktree.NormalizePath(repoPath) != worktree.NormalizePath(sourcePath) {
		return false, nil
	}
	// A high-stakes claim op may commit its own .armature bookkeeping in the
	// coordinator checkout after --from validation and before provisioning.
	// That internal advance does not change the validated source content; any
	// other changed path remains a source mutation and fails closed.
	// #nosec G204 - git binary is fixed; sourcePath and revisions were validated
	// from the repository's own worktree inventory and immutable claim inputs.
	cmd := exec.CommandContext(context.Background(), "git", "-C", sourcePath, "diff", "--name-only", oldTip, newTip)
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("inspect coordinator source advance: %w", err)
	}
	for path := range strings.SplitSeq(string(out), "\n") {
		if path != "" && !strings.HasPrefix(filepath.ToSlash(path), ".armature/") {
			return false, nil
		}
	}
	return true, nil
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

// branchTipIfExists returns the tip of branchName when it exists locally. An
// absent branch is a normal result; other git failures are surfaced to callers.
func branchTipIfExists(repoPath, branchName string) (string, bool, error) {
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "rev-parse", "--verify", "refs/heads/"+branchName)
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return "", false, nil
	}
	return "", false, fmt.Errorf("resolve branch %s: %w", branchName, err)
}

// branchConfigIfExists returns a branch-scoped config value when present. An
// unset key is a normal result; failures to query git config are surfaced.
func branchConfigIfExists(repoPath, key string) (string, bool, error) {
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "config", "--local", "--get", key)
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read config %s: %w", key, err)
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
	// claimToken is the issue's ClaimToken BEFORE this claim's op overwrote it.
	// Carried in the compensating rollback op's RestoreClaimToken alongside the
	// other lease fields when the prior claim was active, so a restored claim
	// keeps its own original token rather than picking up the just-superseded one.
	claimToken string
}

// claimExclusion records a safety pattern this claim added. Rollback can
// remove it only when the corresponding path has no remaining Git worktree;
// pre-existing or concurrently-created exclusions are never removed.
type claimExclusion struct {
	pattern     string
	destination string
	canonical   bool
}

const claimExclusionMarkerName = "armature-claim-exclusion"

// writeClaimExclusionMarker records that this claim added pattern. The marker
// lives in the worktree's private Git directory and therefore follows a
// registered worktree through moves until successful teardown removes it.
func writeClaimExclusionMarker(worktreePath, pattern string) error {
	if pattern == "" {
		return nil
	}
	gitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree git dir: %w", err)
	}
	markerPath := filepath.Join(gitDir, claimExclusionMarkerName)
	if data, readErr := os.ReadFile(markerPath); readErr == nil { //nolint:gosec // markerPath comes from Git's resolved private directory
		if strings.TrimSuffix(string(data), "\n") == pattern {
			return nil
		}
		return fmt.Errorf("claim exclusion marker already records a different pattern")
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("read claim exclusion marker: %w", readErr)
	}
	if err := os.WriteFile(markerPath, []byte(pattern+"\n"), 0o600); err != nil {
		return fmt.Errorf("write claim exclusion marker: %w", err)
	}
	return nil
}

func readClaimExclusionMarker(worktreePath string) (string, bool, error) {
	gitDir, err := resolveWorktreeGitDir(worktreePath)
	if err != nil {
		return "", false, fmt.Errorf("resolve worktree git dir: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(gitDir, claimExclusionMarkerName)) //nolint:gosec // path is inside Git's resolved private directory
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read claim exclusion marker: %w", err)
	}
	pattern := strings.TrimSuffix(string(data), "\n")
	if pattern == "" {
		return "", false, fmt.Errorf("claim exclusion marker is empty")
	}
	return pattern, true, nil
}

func cleanupClaimExclusions(repoPath string, exclusions []claimExclusion) error {
	if len(exclusions) == 0 {
		return nil
	}
	release, err := acquireGitExcludeLock(repoPath)
	if err != nil {
		return err
	}
	defer release()
	return cleanupClaimExclusionsLocked(repoPath, exclusions)
}

func cleanupClaimExclusionsLocked(repoPath string, exclusions []claimExclusion) error {
	if len(exclusions) == 0 {
		return nil
	}
	worktrees, err := worktree.List(repoPath)
	if err != nil {
		return fmt.Errorf("inspect worktrees before exclusion rollback: %w", err)
	}

	for _, exclusion := range exclusions {
		protected := false
		for _, item := range worktrees {
			path := worktree.NormalizePathAllowingMissing(item.Path)
			want := worktree.NormalizePathAllowingMissing(exclusion.destination)
			if exclusion.canonical {
				protected = worktree.IsUnderRoot(path, want)
			} else {
				protected = path == want
			}
			if protected {
				break
			}
		}
		if protected {
			continue
		}
		if _, err := updateGitExcludeTrackedLocked(repoPath, "", exclusion.pattern); err != nil {
			return fmt.Errorf("remove claim exclusion %q: %w", exclusion.pattern, err)
		}
	}
	return nil
}

// newClaimToken generates a unique per-claim nonce (16 random bytes, hex
// encoded) to stamp on a claim op's Payload.ClaimToken. ClaimedAt alone has
// only 1-second resolution, so two claims by the same worker on the same
// issue in the same second are otherwise indistinguishable — which matters
// because rollbackClaim's compensating op must name the EXACT claim it is
// compensating for (see ops.Payload.IfClaimToken), not just "a claim by this
// worker at roughly this time".
func newClaimToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// claimStillOwnedBy reports whether issueID is, right now, claimed by
// workerID with exactly claimToken as its ClaimToken (the unique nonce of
// the specific claim op this process appended, not just "some claim by this
// worker"). It re-loads store from the op log to see the latest materialized
// state, including any claim op a second worker may have appended after this
// worker's claim went stale mid-provisioning, AND any transition (e.g. to
// in-progress or blocked) a different command may have applied to this issue
// in the meantime — transition commands do not take the per-issue claim lock
// (acquireClaimLock has exactly one caller, in this file), so that race is
// real, not theoretical.
//
// The actual ownership test is delegated entirely to materialize.Issue's
// ClaimHeldBy — the single canonical predicate, shared with
// materialize.applyTransition's IfClaimToken guard, for "is this still
// exactly this claim". Do not reintroduce an ad-hoc field comparison here;
// that duplication is exactly what let this predicate drift out of sync with
// its sibling copy across several review rounds before ClaimHeldBy existed.
//
// This is the fast-path ownership check consulted by rollbackClaim (before
// appending a compensating op) and createWorktreeAndBranch's
// cleanupPartialWorktree (before a destructive filesystem action) so they can
// produce a clear "superseded" error without appending a doomed op or
// attempting a doomed cleanup. It is UX only, NOT the correctness boundary:
// correctness now rests on materialize.applyTransition's replay-time
// IfClaimToken validation (see ops.Payload.IfClaimToken), which re-checks the
// exact same ClaimHeldBy condition when the compensating op is actually
// applied, wherever in the append-only log it lands. A worker that skips
// this precheck (or races between it and the destructive action) is still
// safe; this function exists purely to fail fast with a good message before
// that.
//
// If the reload itself fails, ownership is reported as false (fail safe):
// an unreadable store is not evidence this worker still owns the claim, so
// the caller must skip the destructive/compensating action just as it would
// for a confirmed takeover. The reload error is returned so callers can
// mention it in their own warning or error.
func claimStillOwnedBy(store *snapshot.Store, issueID, workerID, claimToken string) (bool, error) {
	if _, err := store.Load(context.Background()); err != nil {
		return false, err
	}
	issue := store.Issue(issueID)
	return issue.ClaimHeldBy(workerID, claimToken), nil
}

// rollbackClaim releases (or restores) the claim after a post-claim worktree
// setup step fails, then returns the error to surface. A same-worker ACTIVE
// claim keeps its prior status; a stale same-worker claim or a different-worker
// takeover is released to open. opLabel names the failed step in the returned
// error. Shared by the create-worktree and update-issue-ID failure paths.
//
// It consults claimStillOwnedBy as a fast-path check (using claimToken, the
// unique nonce of the claim op this process just appended) so a superseded
// claim can be reported clearly without appending a doomed op. But the real
// correctness guarantee is downstream: the compensating transition op below
// always stamps Payload.IfClaimToken = claimToken, so even if a second
// worker's legitimate takeover (a new claim op with a different token) lands
// — or a different command's transition of this issue to in-progress or
// blocked lands — between this check and the append below, or anywhere else
// in the append-only, last-write-wins op log relative to this op, replay
// itself (materialize.applyTransition, via Issue.ClaimHeldBy — the single
// canonical ownership predicate this function's own check also delegates to)
// refuses to apply the compensating op once the claim it targets no longer
// holds. Log ordering no longer matters.
func rollbackClaim(
	cmd *cobra.Command, store *snapshot.Store, logPath, issueID, workerID, opLabel string,
	cause error, prior priorClaimState, claimToken string, exclusionSets ...[]claimExclusion,
) error {
	return rollbackClaimWithExclusionLock(cmd, store, logPath, issueID, workerID, opLabel, cause, prior, claimToken, false, exclusionSets...)
}

func rollbackClaimLocked(
	cmd *cobra.Command, store *snapshot.Store, logPath, issueID, workerID, opLabel string,
	cause error, prior priorClaimState, claimToken string, exclusionSets ...[]claimExclusion,
) error {
	return rollbackClaimWithExclusionLock(cmd, store, logPath, issueID, workerID, opLabel, cause, prior, claimToken, true, exclusionSets...)
}

func rollbackClaimWithExclusionLock(
	cmd *cobra.Command, store *snapshot.Store, logPath, issueID, workerID, opLabel string,
	cause error, prior priorClaimState, claimToken string, exclusionLockHeld bool, exclusionSets ...[]claimExclusion,
) error {
	var exclusions []claimExclusion
	if len(exclusionSets) > 0 {
		exclusions = exclusionSets[0]
	}
	finish := func(base error) error {
		if len(exclusions) == 0 {
			return base
		}
		cleanup := cleanupClaimExclusions
		if exclusionLockHeld {
			cleanup = cleanupClaimExclusionsLocked
		}
		if cleanupErr := cleanup(mustState(cmd).ctx.RepoPath, exclusions); cleanupErr != nil {
			return fmt.Errorf("%w; exclusion rollback failed: %v", base, cleanupErr)
		}
		return base
	}

	owns, err := claimStillOwnedBy(store, issueID, workerID, claimToken)
	if err != nil {
		return finish(fmt.Errorf("%s: %w (claim superseded; no rollback appended: reload store failed: %v)", opLabel, cause, err))
	}
	if !owns {
		return finish(fmt.Errorf("%s: %w (claim superseded; no rollback appended)", opLabel, cause))
	}

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
		payload.RestoreClaimToken = prior.claimToken
	}
	// IfClaimToken makes this a conditional compensating op: replay applies it
	// only if the issue's ClaimToken still equals claimToken (the exact claim
	// this rollback is for) and ClaimedBy still equals workerID. See the
	// function doc comment above for why this — not the precheck above — is
	// what actually makes the rollback race-proof.
	payload.IfClaimToken = claimToken
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
		return finish(fmt.Errorf("%s: %w; also failed to push claim release: %v (manual cleanup may be needed)", opLabel, cause, rbErr))
	}
	return finish(fmt.Errorf("%s: %w (claim released; retry arm claim)", opLabel, cause))
}

// createWorktreeAndBranch creates a new worktree and branches for a task/bug.
// It uses a git client to create a worktree at the given path with a derived branch name.
// If the branch is already checked out in another worktree or if worktree creation fails,
// it returns an error (the user should reuse the existing worktree or unassign/reassign the task).
//
// stillOwns is consulted by cleanupPartialWorktree before any destructive
// action on failure (see that closure below for why: the same claim-took-
// longer-than-TTL race that motivates rollbackClaim's ownership recheck
// applies here too, and --force discards uncommitted work). Callers build
// stillOwns from claimStillOwnedBy, which itself delegates to
// materialize.Issue's ClaimHeldBy — the single canonical ownership
// predicate — so this contract is exactly "is the issue, right now, in
// StatusClaimed with this exact workerID/claimToken pair", never a looser
// or differently-scoped check assembled ad hoc at this call site.
func createWorktreeAndBranch(repoPath, worktreePath, issueID string, issue materialize.Issue, stillOwns func() bool, sourceArgs ...string) error {
	return createWorktreeAndBranchWithExclusion(repoPath, worktreePath, issueID, issue, stillOwns, "", sourceArgs...)
}

func createWorktreeAndBranchWithExclusion(
	repoPath, worktreePath, issueID string,
	issue materialize.Issue,
	stillOwns func() bool,
	exclusionPattern string,
	sourceArgs ...string,
) error {
	// Determine branch name based on issue type
	branchName := deriveBranchName(issue.Type, issueID)

	// Safety guard: empty branch name indicates an issue type that should not have a worktree
	if branchName == "" {
		return fmt.Errorf("cannot create worktree for issue type %q: no branch mapping", issue.Type)
	}

	if len(sourceArgs) != 0 && len(sourceArgs) != 3 {
		return fmt.Errorf("validated claim source requires path, branch, and tip")
	}
	var sourcePath, sourceBranch, sourceTip string
	if len(sourceArgs) == 3 {
		sourcePath, sourceBranch, sourceTip = sourceArgs[0], sourceArgs[1], sourceArgs[2]
		if sourcePath == "" || sourceBranch == "" || sourceTip == "" {
			return fmt.Errorf("validated claim source is incomplete")
		}
		if !isWorktreeOf(repoPath, sourcePath) {
			return fmt.Errorf("validated claim source %s is no longer an existing worktree of this repository", sourcePath)
		}
		currentBranch, err := adapters.New(sourcePath).CurrentBranch()
		if err != nil {
			return fmt.Errorf("revalidate claim source branch: %w", err)
		}
		if currentBranch != sourceBranch {
			return fmt.Errorf("claim source branch changed from %s to %s", sourceBranch, currentBranch)
		}
		currentTip, err := adapters.New(sourcePath).ResolveRevision("HEAD")
		if err != nil {
			return fmt.Errorf("revalidate claim source tip: %w", err)
		}
		if currentTip != sourceTip {
			internalAdvance, err := sourceAdvancedOnlyByArmature(repoPath, sourcePath, sourceTip, currentTip)
			if err != nil {
				return fmt.Errorf("revalidate claim source tip: %w", err)
			}
			if !internalAdvance {
				return fmt.Errorf("claim source tip changed from %s to %s", sourceTip, currentTip)
			}
		}
	}

	// Create git client for main repo
	gitClient := adapters.New(repoPath)
	var headSHA, parentBranch string
	var headErr, parentErr error
	if sourcePath != "" {
		// Use the immutable values captured by newClaimCmd after the source
		// revalidation above. Never re-resolve a mutable source and fall back to
		// the coordinator HEAD after --from validation has succeeded.
		headSHA, parentBranch = sourceTip, sourceBranch
	} else {
		// Resolve HEAD before branching: this is the actual point the task branch
		// diverges from the coordinator's checkout, which may already be a story
		// branch containing completed sibling-task commits (not necessarily main).
		// Persisted below so the delivery gate can scope-check against the real
		// branch-point instead of guessing via merge-base against a default branch.
		headSHA, headErr = gitClient.ResolveRevision("HEAD")

		// Capture the name of the branch this task branch is being cut from.
		parentBranch, parentErr = gitClient.CurrentBranch()
	}

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
		if item.Binding == issueID {
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
		// Re-check ownership immediately before doing anything destructive: by
		// the time a post-provisioning step fails, this worker's claim may have
		// gone stale and a second worker may have legitimately claimed the
		// issue and be actively using a worktree bound at this same canonical
		// path. Ops are append-only and replay last-write-wins, so that second
		// worker's claim is now the truth regardless of what this process still
		// believes; moving the path back over their worktree (adopted case) or
		// `git worktree remove --force` (fresh case) would silently discard
		// whatever work they had already started. Leaving the path alone is the
		// only safe move once ownership cannot be confirmed.
		if !stillOwns() {
			fmt.Fprintf(os.Stderr, "worktree at %s superseded by a newer claim; leaving in place\n", worktreePath)
			return fmt.Errorf("%s: %w", label, cause)
		}
		if adoptedFrom != "" {
			if moveErr := gitClient.MoveWorktree(worktreePath, adoptedFrom); moveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to restore adopted worktree at %s: %v\n", adoptedFrom, moveErr)
			}
		} else if rmErr := gitClient.RemovePartiallyProvisionedWorktree(worktreePath); rmErr != nil {
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
		// claim-time record that is safe to add is the immutable branch binding.
		if err := writeClaimedBranchFileIfAbsent(worktreePath, branchName); err != nil {
			return cleanupPartialWorktree(err, "persist claimed branch metadata")
		}
	} else if err := persistBranchPointMetadata(gitClient, worktreePath, branchName, headSHA, headErr, parentBranch, parentErr); err != nil {
		return cleanupPartialWorktree(err, "persist branch-point metadata")
	}
	if err := writeClaimExclusionMarker(worktreePath, exclusionPattern); err != nil {
		return cleanupPartialWorktree(err, "persist claim exclusion metadata")
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
// empty record would be indistinguishable from "not yet recorded" on the
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

// clearParentBranchMetadata unsets shared parent-branch configuration after a
// successful removal. Per-worktree base and claimed-branch files disappear
// with their worktree and must stay intact if removal fails.
func clearParentBranchMetadata(gitClient *adapters.Client, branchName string) {
	_ = gitClient.UnsetGitConfig(parentBranchConfigKey(branchName)) //nolint:errcheck // best-effort cleanup
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
	var worktreePath string
	var fromWorktreePath string

	cmd := &cobra.Command{
		Use:   "claim [issue-id]",
		Short: "Claim a ready task",
		Long: `Claim an issue to assign it to the current worker.

Claiming an issue marks it as assigned to your worker ID and sets a TTL (time-to-live).
If the TTL expires without progress, the claim becomes stale and may be reassigned.
This command also detects and warns about scope overlaps with concurrently claimed issues.
When you claim a task, its parent story (if open) is automatically advanced to in-progress.
The --worktree flag is required. Without a value it provisions the canonical
.worktrees/<issue-id> path. With --worktree <new-path> --from <parent-worktree-path>,
it creates a new task worktree from the parent worktree's current branch and tip.`,
		Example: `  # Claim an issue by ID with a worktree
  $ arm claim E6-S4-T2 --worktree

  # Claim with a custom TTL of 120 minutes
  $ arm claim --issue E6-S4-T2 --ttl 120 --worktree

  # Claim despite scope overlap warning
  $ arm claim E6-S4-T2 --force --worktree

  # Claim using flag style
  $ arm claim --issue another-task-id --worktree`,
		Args: func(cmd *cobra.Command, args []string) error {
			return mapClaimError(cobra.MaximumNArgs(2)(cmd, args))
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() { err = mapClaimError(err) }()
			ctx := currentCtx(cmd)
			// default_ttl in config.json is the single source of --ttl's default;
			// an explicit --ttl always overrides it. It already governs staleness
			// detection elsewhere (hook.go, workers.go) — this makes claim's
			// default consistent with that same config value.
			if !cmd.Flags().Changed("ttl") && ctx.Config.DefaultTTL > 0 {
				ttl = ctx.Config.DefaultTTL
			}
			var fromBranch, fromTip string
			// pflag's optional-value support sets NoOptDefVal without consuming a
			// following token. Recover the documented spaced form here while
			// retaining the established value-less form for existing agents.
			if worktreePath == defaultWorktreeFlagValue {
				switch {
				case issueID != "" && len(args) == 1:
					worktreePath = args[0]
					args = nil
				case issueID == "" && len(args) == 2:
					issueID = args[0]
					worktreePath = args[1]
					args = nil
				}
			}
			// A value provided with --worktree=PATH has already been consumed by
			// pflag, so any remaining positional token is surplus when --issue is
			// also set. Do not silently ignore it after extracting the issue ID.
			if worktreePath != defaultWorktreeFlagValue && issueID != "" && len(args) > 0 {
				return fmt.Errorf("accepts at most 1 arg(s), received %d", len(args)+1)
			}
			issueID, err = resolveIssueID(issueID, args)
			if err != nil {
				return err
			}
			if worktreePath == "" {
				return fmt.Errorf("--worktree is required")
			}

			customWorktreePath := worktreePath != defaultWorktreeFlagValue
			if fromWorktreePath != "" && !customWorktreePath {
				return fmt.Errorf("--from requires an explicit --worktree <new-path> destination")
			}
			if customWorktreePath {
				worktreePath, err = filepath.Abs(worktreePath)
				if err != nil {
					return fmt.Errorf("resolve worktree path: %w", err)
				}
			} else {
				// Validate and resolve the canonical path before any claim append or
				// filesystem/git mutation. Separator-bearing, absolute, and traversal
				// IDs are rejected rather than becoming filesystem paths.
				worktreePath, err = canonicalWorktreePath(ctx.RepoPath, issueID)
				if err != nil {
					return err
				}
			}
			issuesDir := ctx.IssuesDir

			// Serialize same-clone claims for this issue with an OS-level advisory
			// lock, acquired here — before the FIRST read of issue or worktree
			// state — and held through the end of this command (claim-op append,
			// worktree provisioning, and any rollback). The lock must precede every
			// read whose value later informs a filesystem mutation or the rollback
			// snapshot (allOps, the store load/Issue lookup that seeds `prior`,
			// worktreePathExists, isWorktreeOf, checkExistingWorktreeBinding, and
			// store.Index() for the scope-overlap scan): acquiring it only around
			// the claim-op append left a window where two same-clone invocations
			// could both observe stale pre-claim state, both proceed, and the
			// second one's rollback would restore the FIRST one's stale `prior`
			// snapshot over an active claim it does not own. Acquiring earlier is
			// the fix, not sprinkling re-reads after a later lock. See
			// acquireClaimLock's doc comment for the full substrate model.
			//
			// One accepted side effect: a claim for a nonexistent issue now still
			// creates the lock file (it is created before the issue lookup below
			// can fail). That is harmless — 0600, confined to the git common dir,
			// intentionally never deleted, and transparently reused on retry.
			releaseClaimLock, err := acquireClaimLock(ctx.RepoPath, issueID)
			if err != nil {
				return err
			}
			defer releaseClaimLock()
			releaseGitExcludeLock, err := acquireGitExcludeLock(ctx.RepoPath)
			if err != nil {
				return err
			}
			defer releaseGitExcludeLock()

			// The exclusion lock is acquired before any explicit-destination
			// inspection and held through claim append and provisioning. This
			// serializes the check/remove performed by merged and GC with the
			// claim's check-through-provisioning handoff, so a teardown cannot
			// remove an exclusion after a new claim has reused the destination.
			if customWorktreePath {
				if err := rejectNestedWorktreeDestination(ctx.RepoPath, worktreePath); err != nil {
					return err
				}
				if err := rejectInRepositoryCustomWorktreeDestination(ctx.RepoPath, worktreePath); err != nil {
					return err
				}
			}
			if fromWorktreePath != "" {
				fromWorktreePath, err = filepath.Abs(fromWorktreePath)
				if err != nil {
					return fmt.Errorf("resolve --from worktree path: %w", err)
				}
				if !isWorktreeOf(ctx.RepoPath, fromWorktreePath) {
					return fmt.Errorf("--from path %s is not an existing worktree of this repository", fromWorktreePath)
				}
				fromBranch, err = adapters.New(fromWorktreePath).CurrentBranch()
				if err != nil {
					return fmt.Errorf("resolve --from worktree branch: %w", err)
				}
				if fromBranch == "" || fromBranch == "HEAD" {
					return fmt.Errorf("--from worktree %s must be on a branch", fromWorktreePath)
				}
				fromTip, err = adapters.New(fromWorktreePath).ResolveRevision("HEAD")
				if err != nil {
					return fmt.Errorf("resolve --from worktree tip: %w", err)
				}
			}

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
			if fromWorktreePath != "" {
				existingTip, exists, tipErr := branchTipIfExists(ctx.RepoPath, expectedBranch)
				if tipErr != nil {
					return tipErr
				}
				if exists && existingTip != fromTip {
					return fmt.Errorf(
						"existing branch %s tip %s does not match --from tip %s; use a new issue branch or align it manually",
						expectedBranch, existingTip, fromTip)
				}
				existingParent, exists, configErr := branchConfigIfExists(ctx.RepoPath, parentBranchConfigKey(expectedBranch))
				if configErr != nil {
					return configErr
				}
				if exists && existingParent != fromBranch {
					return fmt.Errorf(
						"existing branch %s parent %s does not match --from branch %s",
						expectedBranch, existingParent, fromBranch)
				}
			}

			// Check whether the worktree path already exists. Capture the state here
			// (no side effects yet) so worktree creation can be deferred until after
			// all claim validations pass.
			if customWorktreePath {
				if _, statErr := os.Lstat(worktreePath); statErr == nil {
					return fmt.Errorf("new worktree path %s must not exist", worktreePath)
				} else if !os.IsNotExist(statErr) {
					return fmt.Errorf("check new worktree path: %w", statErr)
				}
			}
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
				claimToken:             issue.ClaimToken,
			}

			index := store.Index()
			// Build a graph from the materialized state for ancestor/descendant checking
			graph := materialize.GraphFromState(snapshot.State)

			for id, entry := range index {
				// Only issues a worker can actually hold compete for scope: type
				// task, in claimed/in-progress state. A story's scope is by
				// design the union of its children's scopes, so an in-progress
				// story (which can persist long after the child that put it
				// there was claimed/completed by someone else) must never be
				// treated as a competing claimant. Mirrors the non-task filter
				// internal/validate applies in its W1 check.
				if id == issueID || entry.Type != "task" || (entry.Status != ops.StatusClaimed && entry.Status != ops.StatusInProgress) {
					continue
				}
				if claimPkg.ScopesOverlapEx(issue.Scope, entry.Scope, graph, issueID, id) {
					holder := entry.Assignee
					if holder == "" {
						holder = "unknown"
					}
					msg := fmt.Sprintf("scope overlap with %s (%s), a %s %s held by %s", id, entry.Title, entry.Type, entry.Status, holder)
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

			// The canonical managed-worktree root must be excluded before the claim
			// is recorded. Older installations may not have received bootstrap's
			// exclusion; leaving the claim in place when this safety precondition
			// fails would make a later broad `git add .` stage a linked worktree as
			// a gitlink. This intentionally applies to both new and existing
			// canonical worktrees. The repository exclusion lock is already held
			// from destination validation through provisioning, so use the locked
			// helper rather than attempting to acquire it again.
			var claimExclusions []claimExclusion
			canonicalAdded, err := updateGitExcludeTrackedLocked(ctx.RepoPath, ".worktrees/", "")
			if err != nil {
				return fmt.Errorf("exclude managed worktree directory: %w", err)
			}
			if canonicalAdded {
				claimExclusions = append(claimExclusions, claimExclusion{
					pattern: ".worktrees/", destination: worktree.CanonicalRoot(ctx.RepoPath), canonical: true,
				})
			}

			// claimToken is this process's own claim op's unique nonce, generated
			// once here so it can be compared (not just ClaimedBy) against the
			// materialized state later. A worker can re-claim the same issue
			// serially, possibly within the same wall-clock second (ClaimedAt has
			// only 1-second resolution); comparing only ClaimedBy — or ClaimedAt —
			// would treat a later re-claim by the same worker as "still my claim"
			// when it is really a different one, so both rollbackClaim and
			// stillOwnsClaim below key off this exact token.
			claimToken, err := newClaimToken()
			if err != nil {
				if cleanupErr := cleanupClaimExclusionsLocked(ctx.RepoPath, claimExclusions); cleanupErr != nil {
					return fmt.Errorf("generate claim token: %w; exclusion rollback failed: %v", err, cleanupErr)
				}
				return fmt.Errorf("generate claim token: %w", err)
			}
			claimTimestamp := nowEpoch()
			op := ops.Op{
				Type: ops.OpClaim, TargetID: issueID, Timestamp: claimTimestamp,
				WorkerID: workerID, Payload: ops.Payload{TTL: ttl, WorktreePath: worktreePath, ClaimToken: claimToken},
			}
			if err := appendHighStakesOp(mustState(cmd), logPath, op); err != nil {
				if cleanupErr := cleanupClaimExclusionsLocked(ctx.RepoPath, claimExclusions); cleanupErr != nil {
					return fmt.Errorf("%w; exclusion rollback failed: %v", err, cleanupErr)
				}
				return err
			}

			// stillOwnsClaim re-loads the store to confirm this worker's claim
			// (identified by claimToken) has not been superseded, before either
			// rollbackClaim appends a compensating op or createWorktreeAndBranch's
			// cleanup does anything destructive. See claimStillOwnedBy's doc
			// comment for the canonical ownership predicate this delegates to
			// (materialize.Issue.ClaimHeldBy).
			stillOwnsClaim := func() bool {
				owns, err := claimStillOwnedBy(store, issueID, workerID, claimToken)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: reload store to verify claim ownership failed: %v\n", err)
					return false
				}
				return owns
			}

			// Refresh store after appending claim Op
			if _, err := store.Load(context.Background()); err != nil {
				return fmt.Errorf("refresh store after claim: %w", err)
			}

			issueAfter := store.Issue(issueID)
			if issueAfter == nil {
				return fmt.Errorf("issue %s not found after claim", issueID)
			}
			// won asks "did THIS claim op win?", not the looser "is *a* claim by
			// this worker current?" that a bare issueAfter.ClaimedBy == workerID
			// comparison would ask. The two questions coincide on every legitimate
			// path -- materialize.applyClaim sets Status/ClaimedBy/ClaimToken
			// unconditionally and atomically on a won claim, so immediately after
			// the append-and-reload above, this op's own claimToken is current
			// whenever we genuinely won -- but they diverge exactly in the case
			// this predicate exists to catch: the same workerID claiming the same
			// issue concurrently from two different clones (acquireClaimLock's
			// flock is per-clone; nothing enforces a worker ID's global
			// uniqueness across clones). Both reload and both see
			// ClaimedBy == workerID; only ClaimHeldBy's claimToken comparison
			// tells the loser it lost. This is the third call site delegating to
			// materialize.Issue.ClaimHeldBy -- alongside claimStillOwnedBy in this
			// file and applyTransition's IfClaimToken guard in
			// internal/materialize/engine.go -- and, per ClaimHeldBy's own doc
			// comment, no ad-hoc ClaimedBy == comparison belongs anywhere on the
			// claim path; delegate to it instead of writing a fourth copy.
			won := issueAfter.ClaimHeldBy(workerID, claimToken)
			if !won {
				// supersededBySameWorker distinguishes losing to a genuinely
				// different claimant from being superseded by a different claim
				// that happens to carry this same workerID (the two-clones race
				// above). Without this, "claimed_by" reporting our own workerID
				// back to us reads as nonsense ("lost the race to yourself").
				// claimed_by/reason/claimed/issue keep their exact prior meaning
				// and values for the ordinary different-worker case so existing
				// agent consumers and TestClaimCommand_LostRaceReportsClearResult
				// (main_test.go) keep working unchanged; this only adds a field.
				supersededBySameWorker := issueAfter.ClaimedBy == workerID
				format, _ := cmd.Root().PersistentFlags().GetString("format")
				switch {
				case format == "json" || format == "agent":
					result := map[string]any{
						"issue":                     issueID,
						"claimed":                   false,
						"claimed_by":                issueAfter.ClaimedBy,
						"reason":                    "lost_claim_race",
						"superseded_by_same_worker": supersededBySameWorker,
					}
					data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				case supersededBySameWorker:
					_, _ = fmt.Fprintf(cmd.OutOrStdout(),
						"Claim lost for %s (superseded by a different claim from this same worker ID, %s)\n",
						issueID, issueAfter.ClaimedBy)
				default:
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claim lost for %s (claimed by %s)\n", issueID, issueAfter.ClaimedBy)
				}
				if cleanupErr := cleanupClaimExclusionsLocked(ctx.RepoPath, claimExclusions); cleanupErr != nil {
					return fmt.Errorf("claim lost: exclusion rollback failed: %w", cleanupErr)
				}
				return nil
			}

			// Worktree setup is deferred to here so it only happens after all claim
			// validations pass and this worker has won the claim race.
			if !worktreeExists {
				var sourceArgs []string
				if fromWorktreePath != "" {
					sourceArgs = []string{fromWorktreePath, fromBranch, fromTip}
				}
				if err := createWorktreeAndBranchWithExclusion(
					ctx.RepoPath, worktreePath, issueID, *issue,
					stillOwnsClaim, "", sourceArgs...,
				); err != nil {
					return rollbackClaimLocked(cmd, store, logPath, issueID, workerID, "create worktree", err, prior, claimToken, claimExclusions)
				}
			} else {
				// Worktree exists and binding was already validated above; update the
				// task ID file to ensure the binding is current (idempotent).
				if err := updateIssueIDFile(worktreePath, issueID); err != nil {
					return rollbackClaimLocked(cmd, store, logPath, issueID, workerID, "update task ID file", err, prior, claimToken, claimExclusions)
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
						return rollbackClaimLocked(cmd, store, logPath, issueID, workerID, "persist claimed branch metadata", err, prior, claimToken, claimExclusions)
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
	cmd.Flags().StringVar(&worktreePath, "worktree", "", "provision a worktree (required); omit its value for .worktrees/<issue-id>")
	cmd.Flags().Lookup("worktree").NoOptDefVal = defaultWorktreeFlagValue
	cmd.Flags().StringVar(&fromWorktreePath, "from", "", "create the new worktree from this parent worktree's current tip")
	return cmd
}
