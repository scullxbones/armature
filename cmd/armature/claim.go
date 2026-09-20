package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

const defaultWorktreeFlagValue = ".armature-default-worktree"

func worktreePathExists(path string) (bool, error) {
	gitFile := filepath.Join(path, ".git")
	_, err := os.Stat(gitFile)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

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

func checkExistingWorktreeBinding(worktreePath, issueID, expectedBranch string) error {
	actualGitDir, err := worktree.ResolveGitDir(worktreePath)
	if err != nil {
		return nil
	}

	existingIssueID, err := harnesshook.ReadIssueBindingFileErr(actualGitDir)
	if err != nil {
		return fmt.Errorf("read existing binding: %w", err)
	}
	if existingIssueID != "" && existingIssueID != issueID {
		return fmt.Errorf("worktree at %s is already bound to %s: use a different --worktree path",
			worktreePath, existingIssueID)
	}

	headFile := filepath.Join(actualGitDir, "HEAD")
	headBytes, err := os.ReadFile(headFile) //nolint:gosec // internal path
	if err != nil {
		return nil
	}
	headStr := strings.TrimSpace(string(headBytes))
	if !strings.HasPrefix(headStr, "ref: refs/heads/") {
		if existingIssueID == issueID {
			return nil
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

func destLocationFacts(repoPath, destination string) (inRepo, underCanonical bool) {
	repoRoot := worktree.NormalizePath(repoPath)
	normalizedDestination := worktree.NormalizePathAllowingMissing(destination)
	inRepo = worktree.IsUnderRoot(normalizedDestination, repoRoot)
	underCanonical = worktree.IsUnderRoot(normalizedDestination, worktree.CanonicalRoot(repoPath))
	return inRepo, underCanonical
}

func nestedRegisteredWorktree(repoPath, destination string) (string, error) {
	registeredPaths, err := worktree.RegisteredPaths(repoPath)
	if err != nil {
		return "", fmt.Errorf("inspect registered worktree destinations: %w", err)
	}
	normalizedDestination := worktree.NormalizePathAllowingMissing(destination)
	for _, registeredPath := range registeredPaths {
		normalizedRegisteredPath := worktree.NormalizePathAllowingMissing(registeredPath)
		if normalizedDestination == normalizedRegisteredPath {
			prunable, err := worktree.HasPrunableRegistration(repoPath, registeredPath)
			if err != nil {
				return "", fmt.Errorf("inspect registered worktree %s: %w", registeredPath, err)
			}
			if prunable {
				continue
			}
		}
		if worktree.IsUnderRoot(normalizedDestination, normalizedRegisteredPath) {
			return registeredPath, nil
		}
	}
	return "", nil
}

func refuseCustomWorktreeDestination(repoPath, destination, issueID, expectedBranch string) error {
	nestedUnder, err := nestedRegisteredWorktree(repoPath, destination)
	if err != nil {
		return err
	}
	inRepo, underCanonical := destLocationFacts(repoPath, destination)
	plan, err := worktree.PlanProvision(worktree.ProvisionInput{
		IssueID:        issueID,
		Dest:           destination,
		ExpectedBranch: expectedBranch,
		NestedUnder:    nestedUnder,
		InRepo:         inRepo,
		UnderCanonical: underCanonical,
	})
	if err != nil {
		return err
	}
	if plan.Action == worktree.ProvisionRefuse {
		return errors.New(plan.RefuseReason)
	}
	return nil
}

func evaluateProvisionPlan(repoPath, dest, issueID, expectedBranch string) (worktree.ProvisionPlan, error) {
	inventory, err := worktree.List(repoPath)
	if err != nil {
		return worktree.ProvisionPlan{}, fmt.Errorf("inspect existing worktrees: %w", err)
	}
	return worktree.PlanProvision(provisionInputFromInventory(repoPath, dest, issueID, expectedBranch, inventory))
}

func provisionInputFromInventory(repoPath, dest, issueID, expectedBranch string, inventory []worktree.Meta) worktree.ProvisionInput {
	rows := make([]worktree.InventoryRow, 0, len(inventory))
	normalizedDest := worktree.NormalizePathAllowingMissing(dest)
	var adoptCandidate string
	bound := 0
	for _, item := range inventory {
		path := worktree.NormalizePath(item.Path)
		rows = append(rows, worktree.InventoryRow{
			Path:    path,
			Branch:  item.Branch,
			Binding: item.Binding,
		})
		if item.Binding == issueID {
			bound++
			if path != normalizedDest {
				adoptCandidate = item.Path
			}
		}
	}
	provenanceOK := false
	if bound == 1 && adoptCandidate != "" {
		provenanceOK = hasTrustedBranchPointMetadata(adapters.New(adoptCandidate), adoptCandidate, expectedBranch)
	}
	inRepo, underCanonical := destLocationFacts(repoPath, dest)
	return worktree.ProvisionInput{
		IssueID:        issueID,
		Dest:           normalizedDest,
		ExpectedBranch: expectedBranch,
		Inventory:      rows,
		InRepo:         inRepo,
		UnderCanonical: underCanonical,
		ProvenanceOK:   provenanceOK,
	}
}

func adoptSourcePath(inventory []worktree.Meta, adoptFrom string) string {
	for _, item := range inventory {
		if worktree.NormalizePath(item.Path) == adoptFrom {
			return item.Path
		}
	}
	return adoptFrom
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

func addWorktreeDetached(repoPath, worktreePath, baseRef string) error {
	addArgs := []string{"worktree", "add", "--detach", worktreePath, baseRef}
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

type priorClaimState struct {
	status                 string
	claimedBy              string
	claimedAt              int64
	lastHeartbeat          int64
	claimTTL               int
	claimingWorkerActivity int64
	worktreePath           string
	claimToken             string
}

type claimExclusion struct {
	pattern     string
	destination string
	canonical   bool
}

const claimExclusionMarkerName = "armature-claim-exclusion"

func writeClaimExclusionMarker(worktreePath, pattern string) error {
	if pattern == "" {
		return nil
	}
	gitDir, err := worktree.ResolveGitDir(worktreePath)
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
	gitDir, err := worktree.ResolveGitDir(worktreePath)
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

func newClaimToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func reloadStoreClaimHeldBy(store *snapshot.Store, issueID, workerID, claimToken string) (bool, error) {
	if _, err := store.Load(context.Background()); err != nil {
		return false, err
	}
	issue := store.Issue(issueID)
	return issue.ClaimHeldBy(workerID, claimToken), nil
}

func priorLeaseFacts(prior priorClaimState) claimPkg.LeaseFacts {
	return claimPkg.LeaseFacts{
		Status:                 prior.status,
		ClaimedBy:              prior.claimedBy,
		ClaimedAt:              prior.claimedAt,
		LastHeartbeat:          prior.lastHeartbeat,
		ClaimTTL:               prior.claimTTL,
		ClaimingWorkerActivity: prior.claimingWorkerActivity,
		WorktreePath:           prior.worktreePath,
		ClaimToken:             prior.claimToken,
	}
}

func compensateClaimIfHeldByToken(
	cmd *cobra.Command, store *snapshot.Store, logPath, issueID, workerID, opLabel string,
	cause error, prior priorClaimState, ifClaimToken string, exclusionLockHeld bool, exclusionSets ...[]claimExclusion,
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

	owns, err := reloadStoreClaimHeldBy(store, issueID, workerID, ifClaimToken)
	if err != nil {
		return finish(fmt.Errorf("%s: %w (claim superseded; no rollback appended: reload store failed: %v)", opLabel, cause, err))
	}
	if !owns {
		return finish(fmt.Errorf("%s: %w (claim superseded; no rollback appended)", opLabel, cause))
	}

	now := nowEpoch()
	payload, planErr := claimPkg.PlanCompensation(claimPkg.CompensationInput{
		Prior:        priorLeaseFacts(prior),
		WorkerID:     workerID,
		Now:          now,
		IfClaimToken: ifClaimToken,
	})
	if planErr != nil {
		return finish(fmt.Errorf("%s: %w; also failed to plan claim compensation: %v (manual cleanup may be needed)", opLabel, cause, planErr))
	}
	rollbackOp := ops.Op{
		Type:      ops.OpTransition,
		TargetID:  issueID,
		Timestamp: now,
		WorkerID:  workerID,
		Payload:   payload,
	}
	if rbErr := appendHighStakesOp(mustState(cmd), logPath, rollbackOp); rbErr != nil {
		return finish(fmt.Errorf("%s: %w; also failed to push claim release: %v (manual cleanup may be needed)", opLabel, cause, rbErr))
	}
	return finish(fmt.Errorf("%s: %w (claim released; retry arm claim)", opLabel, cause))
}

func createWorktreeAndBranchWithExclusion(
	repoPath, worktreePath, issueID string,
	issue materialize.Issue,
	stillOwns func() bool,
	exclusionPattern string,
	sourceArgs ...string,
) error {
	branchName := materialize.DeriveBranchName(issue.Type, issueID)

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

	gitClient := adapters.New(repoPath)
	var headSHA, parentBranch string
	var headErr, parentErr error
	if sourcePath != "" {
		headSHA, parentBranch = sourceTip, sourceBranch
	} else {
		headSHA, headErr = gitClient.ResolveRevision("HEAD")

		parentBranch, parentErr = gitClient.CurrentBranch()
	}

	detachRef := "HEAD"
	if headErr == nil && headSHA != "" {
		detachRef = headSHA
	}
	adopted := false
	adoptedFrom := ""
	alreadyAtDest := false
	inventory, inventoryErr := worktree.List(repoPath)
	if inventoryErr != nil {
		return fmt.Errorf("inspect existing worktrees: %w", inventoryErr)
	}
	for _, item := range inventory {
		if item.Binding != issueID && item.Branch == "refs/heads/"+branchName {
			return fmt.Errorf("branch %s is already checked out at %s; bind that worktree to %s before claiming", branchName, item.Path, issueID)
		}
	}
	plan, err := worktree.PlanProvision(provisionInputFromInventory(repoPath, worktreePath, issueID, branchName, inventory))
	if err != nil {
		return err
	}
	if plan.Action == worktree.ProvisionRefuse {
		return errors.New(plan.RefuseReason)
	}
	switch plan.Action {
	case worktree.ProvisionAlreadyAtDest:
		alreadyAtDest = true
	case worktree.ProvisionAdopt:
		adoptFrom := adoptSourcePath(inventory, plan.AdoptFrom)
		_, statErr := os.Stat(worktreePath)
		switch {
		case statErr == nil:
		case !os.IsNotExist(statErr):
			return fmt.Errorf("check canonical worktree path: %w", statErr)
		default:
			if err := os.MkdirAll(filepath.Dir(worktreePath), 0o750); err != nil {
				return fmt.Errorf("create canonical worktree root: %w", err)
			}
			if err := gitClient.MoveWorktree(adoptFrom, worktreePath); err != nil {
				return fmt.Errorf("adopt bound worktree: %w", err)
			}
			adopted = true
			adoptedFrom = adoptFrom
		}
	}
	if !adopted && !alreadyAtDest {
		if err := addWorktreeDetached(repoPath, worktreePath, detachRef); err != nil {
			return fmt.Errorf("add worktree: %w", err)
		}
	}

	cleanupPartialWorktree := func(cause error, label string) error {
		if !stillOwns() {
			fmt.Fprintf(os.Stderr, "worktree at %s superseded by a newer claim; leaving in place\n", worktreePath)
			return fmt.Errorf("%s: %w", label, cause)
		}
		if adoptedFrom != "" {
			if moveErr := gitClient.MoveWorktree(worktreePath, adoptedFrom); moveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to restore adopted worktree at %s: %v\n", adoptedFrom, moveErr)
			}
		} else if alreadyAtDest {
		} else if rmErr := gitClient.RemovePartiallyProvisionedWorktree(worktreePath); rmErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to clean up partial worktree at %s: %v\n", worktreePath, rmErr)
		}
		return fmt.Errorf("%s: %w", label, cause)
	}

	if !adopted && !alreadyAtDest {
		if err := checkoutBranchInWorktree(worktreePath, branchName); err != nil {
			return cleanupPartialWorktree(err, "checkout branch in worktree")
		}
	}

	if err := worktree.ApplyMitigations(repoPath, worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: apply worktree mitigations: %v\n", err)
	}

	if err := updateIssueIDFile(worktreePath, issueID); err != nil {
		return cleanupPartialWorktree(err, "write issue ID file")
	}

	if adopted {
		if err := writeClaimedBranchFileIfAbsent(worktreePath, branchName); err != nil {
			return cleanupPartialWorktree(err, "persist claimed branch metadata")
		}
	} else if alreadyAtDest {
		worktreeGitClient := adapters.New(worktreePath)
		if hasTrustedBranchPointMetadata(worktreeGitClient, worktreePath, branchName) {
			if err := writeClaimedBranchFileIfAbsent(worktreePath, branchName); err != nil {
				return cleanupPartialWorktree(err, "persist claimed branch metadata")
			}
		}
	} else if err := persistBranchPointMetadata(gitClient, worktreePath, branchName, headSHA, headErr, parentBranch, parentErr); err != nil {
		return cleanupPartialWorktree(err, "persist branch-point metadata")
	}
	if err := writeClaimExclusionMarker(worktreePath, exclusionPattern); err != nil {
		return cleanupPartialWorktree(err, "persist claim exclusion metadata")
	}
	return nil
}

func hasTrustedBranchPointMetadata(gitClient *adapters.Client, worktreePath, branchName string) bool {
	if _, err := deliverygate.RecordedBaseCommit(worktreePath); err == nil {
		return true
	}
	parentBranch, err := gitClient.ReadGitConfig(deliverygate.ParentBranchConfigKey(branchName))
	return err == nil && parentBranch != "" && parentBranch != "HEAD"
}

func persistBranchPointMetadata(
	gitClient *adapters.Client,
	worktreePath, branchName string,
	headSHA string, headErr error,
	parentBranch string, parentErr error,
) error {
	if parentErr == nil && parentBranch != "" && parentBranch != "HEAD" {
		if err := writeParentBranchConfigIfAbsent(gitClient, branchName, parentBranch); err != nil {
			return fmt.Errorf("write parent branch config: %w", err)
		}
	}

	if headErr == nil {
		if err := writeBaseCommitFileIfAbsent(worktreePath, headSHA); err != nil {
			return fmt.Errorf("write base commit file: %w", err)
		}
	}

	if err := writeClaimedBranchFileIfAbsent(worktreePath, branchName); err != nil {
		return fmt.Errorf("write claimed branch file: %w", err)
	}

	return nil
}

func writeParentBranchConfigIfAbsent(gitClient *adapters.Client, branchName, parentBranch string) error {
	key := deliverygate.ParentBranchConfigKey(branchName)
	if existing, err := gitClient.ReadGitConfig(key); err == nil && existing != "" {
		return nil
	}
	return gitClient.SetGitConfig(key, parentBranch)
}

func writeGitDirFileIfAbsent(worktreePath, filename, content string) error {
	actualGitDir, err := worktree.ResolveGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree git dir: %w", err)
	}
	path := filepath.Join(actualGitDir, filename)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}
	return nil
}

func writeBaseCommitFileIfAbsent(worktreePath, headSHA string) error {
	return writeGitDirFileIfAbsent(worktreePath, deliverygate.BaseCommitFileName, headSHA)
}

func writeClaimedBranchFileIfAbsent(worktreePath, branchName string) error {
	if branchName == "" {
		return nil
	}
	return writeGitDirFileIfAbsent(worktreePath, deliverygate.ClaimedBranchFileName, branchName)
}

func updateIssueIDFile(worktreePath, issueID string) error {
	actualGitDir, err := worktree.ResolveGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree git dir: %w", err)
	}

	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	if err := os.WriteFile(issueIDFile, []byte(issueID), 0o600); err != nil {
		return fmt.Errorf("write issue ID file: %w", err)
	}
	return nil
}

func planInputFromSnapshot(targetID string, targetScope []string, workerID string, force bool, snap *snapshot.Snapshot, prior []ops.Op) claimPkg.PlanInput {
	issues := map[string]claimPkg.IssueFacts{}
	if snap != nil && snap.State != nil {
		issues = make(map[string]claimPkg.IssueFacts, len(snap.State.Issues))
		for id, iss := range snap.State.Issues {
			if iss == nil {
				continue
			}
			issues[id] = claimPkg.IssueFacts{
				Type:      iss.Type,
				Status:    iss.Status,
				ClaimedBy: iss.ClaimedBy,
				Title:     iss.Title,
				Scope:     iss.Scope,
			}
		}
	}
	var graph claimPkg.HierarchyGraph
	if snap != nil && snap.State != nil {
		graph = materialize.GraphFromState(snap.State)
	}
	return claimPkg.PlanInput{
		TargetID:    targetID,
		TargetScope: targetScope,
		WorkerID:    workerID,
		Force:       force,
		Issues:      issues,
		Graph:       graph,
		PriorOps:    prior,
	}
}

func persistClaimPlanNotes(state *executionState, logPath, workerID string, notes []claimPkg.NoteIntent) error {
	if len(notes) == 0 {
		return nil
	}
	proposed := make([]ops.Op, 0, len(notes))
	for _, note := range notes {
		proposed = append(proposed, ops.Op{
			Type:      ops.OpNote,
			TargetID:  note.IssueID,
			Timestamp: nowEpoch(),
			WorkerID:  workerID,
			Payload:   ops.Payload{Msg: note.Message},
		})
	}
	return appendLowStakesOps(state, logPath, proposed)
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
				worktreePath, err = canonicalWorktreePath(ctx.RepoPath, issueID)
				if err != nil {
					return err
				}
			}
			issuesDir := ctx.IssuesDir

			cloneFlock, err := tryAcquirePessimisticCloneClaimFlock(ctx.RepoPath, issueID)
			if err != nil {
				return err
			}
			defer cloneFlock.Release()
			releaseGitExcludeLock, err := acquireGitExcludeLock(ctx.RepoPath)
			if err != nil {
				return err
			}
			defer releaseGitExcludeLock()

			if customWorktreePath {
				if err := refuseCustomWorktreeDestination(ctx.RepoPath, worktreePath, issueID, ""); err != nil {
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

			allOps, _, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}

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

			expectedBranch := materialize.DeriveBranchName(issue.Type, issueID)
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
				existingParent, exists, configErr := branchConfigIfExists(ctx.RepoPath, deliverygate.ParentBranchConfigKey(expectedBranch))
				if configErr != nil {
					return configErr
				}
				if exists && existingParent != fromBranch {
					return fmt.Errorf(
						"existing branch %s parent %s does not match --from branch %s",
						expectedBranch, existingParent, fromBranch)
				}
			}

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

			if worktreeExists {
				if !isWorktreeOf(ctx.RepoPath, worktreePath) {
					return fmt.Errorf("worktree at %s is not registered to this repository; it may belong to a different clone", worktreePath)
				}
			}

			if worktreeExists {
				if err := checkExistingWorktreeBinding(worktreePath, issueID, expectedBranch); err != nil {
					return err
				}
			}

			provisionPlan, err := evaluateProvisionPlan(ctx.RepoPath, worktreePath, issueID, expectedBranch)
			if err != nil {
				return err
			}
			if provisionPlan.Action == worktree.ProvisionRefuse {
				return errors.New(provisionPlan.RefuseReason)
			}

			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}

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
			plan, err := claimPkg.PlanClaim(planInputFromSnapshot(issueID, issue.Scope, workerID, force, snapshot, allOps))
			if err != nil {
				return err
			}
			for _, reason := range plan.BlockReasons {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", reason)
			}
			for _, warning := range plan.Warnings {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", warning)
			}
			if len(plan.BlockReasons) > 0 {
				return fmt.Errorf("cannot claim %s: %s — use --force to override", issueID, strings.Join(plan.BlockReasons, "; "))
			}
			if err := persistClaimPlanNotes(mustState(cmd), logPath, workerID, plan.Notes); err != nil {
				return fmt.Errorf("persist claim overlap notes: %w", err)
			}

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

			stillOwnsClaim := func() bool {
				owns, err := reloadStoreClaimHeldBy(store, issueID, workerID, claimToken)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: reload store to verify claim ownership failed: %v\n", err)
					return false
				}
				return owns
			}

			if _, err := store.Load(context.Background()); err != nil {
				return fmt.Errorf("refresh store after claim: %w", err)
			}

			issueAfter := store.Issue(issueID)
			if issueAfter == nil {
				return fmt.Errorf("issue %s not found after claim", issueID)
			}
			won := issueAfter.ClaimHeldBy(workerID, claimToken)
			if !won {
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
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(mustMarshal(result)))
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

			rebindExistingDest := worktreeExists &&
				(provisionPlan.Action == worktree.ProvisionAlreadyAtDest ||
					provisionPlan.Action == worktree.ProvisionFresh)
			if !rebindExistingDest {
				var sourceArgs []string
				if fromWorktreePath != "" {
					sourceArgs = []string{fromWorktreePath, fromBranch, fromTip}
				}
				if err := createWorktreeAndBranchWithExclusion(
					ctx.RepoPath, worktreePath, issueID, *issue,
					stillOwnsClaim, "", sourceArgs...,
				); err != nil {
					return compensateClaimIfHeldByToken(
						cmd, store, logPath, issueID, workerID, "create worktree",
						err, prior, claimToken, true, claimExclusions,
					)
				}
			} else {
				if err := updateIssueIDFile(worktreePath, issueID); err != nil {
					return compensateClaimIfHeldByToken(
						cmd, store, logPath, issueID, workerID, "update task ID file",
						err, prior, claimToken, true, claimExclusions,
					)
				}

				worktreeGitClient := adapters.New(worktreePath)
				if hasTrustedBranchPointMetadata(worktreeGitClient, worktreePath, expectedBranch) {
					if err := writeClaimedBranchFileIfAbsent(worktreePath, expectedBranch); err != nil {
						return compensateClaimIfHeldByToken(
							cmd, store, logPath, issueID, workerID, "persist claimed branch metadata",
							err, prior, claimToken, true, claimExclusions,
						)
					}
				}
			}
			if parentID := issue.Parent; parentID != "" {
				if parentEntry, ok := index[parentID]; ok && parentEntry.Status == ops.StatusOpen {
					advanceOp := ops.Op{
						Type:      ops.OpTransition,
						TargetID:  parentID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{To: ops.StatusInProgress},
					}
					swallowErr(appendOp(ctx, logPath, advanceOp))
				}
			}

			writeCommandResult(cmd, map[string]any{"issue": issueID, "claimed_by": workerID, "ttl": ttl},
				"Claimed %s\n", issueID)
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
