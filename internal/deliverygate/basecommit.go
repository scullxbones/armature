package deliverygate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
)

// BaseCommitFileName is the name of the file (written into a worktree's
// actual git directory, alongside armature-issue-id) that records the SHA
// the task branch diverged from at claim time. The delivery gate reads this
// to scope-check against the real branch-point rather than merge-basing
// against a default branch, which is wrong whenever the task branch was cut
// from a story branch containing completed sibling-task commits.
const BaseCommitFileName = "armature-base-commit"

// ParentBranchConfigKey returns the git config key used to durably record,
// on the shared (main-repo) git config, the branch a task branch was cut
// from. Recorded as git config rather than a per-worktree file: git config
// --local written from a linked worktree lands in the main repo's shared
// .git/config (armature does not enable the worktreeConfig extension), so
// the record survives `arm merged` removing the worktree, and stays
// addressable by branch name if the worktree is later recreated.
func ParentBranchConfigKey(branchName string) string {
	return "branch." + branchName + ".armature-parent"
}

// CandidateBaseRefs are tried in order to find the branch a task diverged
// from. Remote-tracking refs are preferred over local branches: a local
// `main`/`master` in a long-lived coordinator checkout is frequently stale
// (fast-forwarded only on release), whereas `origin/main` reflects the
// actual upstream tip workers branched from.
var CandidateBaseRefs = []string{"origin/main", "origin/master", "main", "master"}

// ResolveWorktreeGitDir resolves the actual git directory for a worktree path.
// In a git worktree, the .git entry is a file (not a directory) containing
// "gitdir: <path>" pointing to the real git dir (e.g., <parent>/.git/worktrees/<name>).
// This function reads that file and returns the resolved absolute path.
// Shared by claim, harness-hook, and the delivery gate so all read from the
// same location.
func ResolveWorktreeGitDir(worktreePath string) (string, error) {
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

// VerifyIssueWorktreeBinding fails closed unless worktreePath is the actual
// worktree bound to issueID (the issue-ID marker file written by
// updateIssueIDFile at claim time — see harnesshook.ReadIssueBindingFileErr).
// This prevents `arm transition --to done --repo <some-other-checkout>` from
// running the delivery gate against a directory that isn't the claimed
// worktree for issueID, which would let a dirty or out-of-scope claimed
// worktree pass because the wrong directory was checked instead.
func VerifyIssueWorktreeBinding(worktreePath, issueID string) error {
	gitDir, err := ResolveWorktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve git dir for %s: %w. Use --skip-delivery-gate to bypass", worktreePath, err)
	}
	binding, err := harnesshook.ReadIssueBindingFileErr(gitDir)
	if err != nil {
		return fmt.Errorf("read issue binding for %s: %w. Use --skip-delivery-gate to bypass", worktreePath, err)
	}
	if binding == "" {
		return fmt.Errorf("%s is not bound to any issue (no armature-issue-id marker found):\n"+
			"cannot verify this is the claimed worktree for %s. Use --skip-delivery-gate to bypass",
			worktreePath, issueID)
	}
	if binding != issueID {
		return fmt.Errorf("%s is bound to issue %s, not %s: refusing to run delivery gate check\n"+
			"against the wrong worktree. Use --skip-delivery-gate to bypass",
			worktreePath, binding, issueID)
	}
	return nil
}

// VerifyIssueBranchBinding fails closed unless worktreePath's current git
// branch (HEAD) is the expected task branch for issueID, derived the same
// way claim.go's createWorktreeAndBranch does (see materialize.DeriveBranchName).
// The armature-issue-id marker file checked by VerifyIssueWorktreeBinding only
// proves the worktree was once claimed for this issue — it does not prove
// HEAD is still on the branch the coordinator will actually integrate. A
// worker could check out an unrelated scratch branch after claiming and
// still pass that check, silently stranding otherwise-valid commits off the
// task branch. issueType empty or unmapped (DeriveBranchName returns "")
// skips this check, matching the caller's existing task/bug/feature gating.
func VerifyIssueBranchBinding(worktreePath, issueID, issueType string) error {
	expectedBranch := materialize.DeriveBranchName(issueType, issueID)
	if expectedBranch == "" {
		return nil
	}

	git := adapters.New(worktreePath)
	currentBranch, err := git.CurrentBranch()
	if err != nil {
		return fmt.Errorf("determine current branch for %s: %w. Use --skip-delivery-gate to bypass", worktreePath, err)
	}
	if currentBranch != expectedBranch {
		return fmt.Errorf(
			"%s is on branch %q but the delivery gate expects %q for issue %s:\n"+
				"the coordinator integrates %[3]s, so commits on any other branch will not be picked up.\n"+
				"Check out %[3]s or use --skip-delivery-gate to override",
			worktreePath, currentBranch, expectedBranch, issueID)
	}
	return nil
}

// RecordedBaseCommit reads the branch-point SHA persisted at claim time
// (see writeBaseCommitFileIfAbsent in cmd/armature/claim.go) from the
// worktree's actual git directory. Returns an error if the worktree wasn't
// claimed after this mechanism was introduced, so callers can fall back to
// GetBaseCommit.
func RecordedBaseCommit(worktreePath string) (string, error) {
	actualGitDir, err := ResolveWorktreeGitDir(worktreePath)
	if err != nil {
		return "", fmt.Errorf("resolve worktree git dir: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(actualGitDir, BaseCommitFileName)) //nolint:gosec // G304: derived from a trusted git directory
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(data))
	if sha == "" {
		return "", fmt.Errorf("base commit file is empty")
	}
	return sha, nil
}

// DynamicBaseCommit recomputes the task branch's divergence point on demand
// by merge-basing the current branch against its recorded parent branch
// (see ParentBranchConfigKey / writeParentBranchConfigIfAbsent in
// cmd/armature/claim.go). Unlike a SHA recorded once at claim time, this is
// recomputed fresh on every gate check, so it stays correct even if the task
// branch was rebased onto an updated parent tip after claim — a stale
// recorded SHA would otherwise misattribute new sibling commits pulled in by
// the rebase as in-scope diff, reintroducing the sibling-attribution bug this
// mechanism exists to prevent. Returns an error if the current branch can't
// be determined, no parent is recorded (worktrees claimed before this
// existed), or the parent ref no longer resolves.
func DynamicBaseCommit(git *adapters.Client) (string, error) {
	currentBranch, err := git.CurrentBranch()
	if err != nil || currentBranch == "" {
		return "", fmt.Errorf("determine current branch: %w", err)
	}
	parentBranch, err := git.ReadGitConfig(ParentBranchConfigKey(currentBranch))
	if err != nil || parentBranch == "" {
		return "", fmt.Errorf("no recorded parent branch for %s: %w", currentBranch, err)
	}
	// A persisted literal "HEAD" is a stale record from before the
	// detached-HEAD guard existed in claim.go (see writeParentBranchConfigIfAbsent):
	// resolving the ref "HEAD" here would just mean the task branch's own tip,
	// collapsing the merge-base to the task's HEAD and making every commit
	// range for CommitReferenceCheck empty. Treat it the same as an
	// absent/empty value so old bad records self-heal by falling back to
	// RecordedBaseCommit / GetBaseCommit instead of silently producing a
	// wrong (empty) range.
	if parentBranch == "HEAD" {
		return "", fmt.Errorf("recorded parent branch for %s is the literal value \"HEAD\"\n"+
			"(stale pre-fix record): treating as no usable parent branch", currentBranch)
	}
	if _, err := git.ResolveRevision(parentBranch); err != nil {
		return "", fmt.Errorf("recorded parent branch %s does not resolve: %w", parentBranch, err)
	}
	base, err := git.MergeBase(currentBranch, parentBranch)
	if err != nil {
		return "", fmt.Errorf("merge-base %s %s: %w", currentBranch, parentBranch, err)
	}
	return base, nil
}

// GetBaseCommit finds the merge-base between HEAD and the first candidate
// base ref (CandidateBaseRefs) that resolves in this repo.
func GetBaseCommit(git *adapters.Client) (string, error) {
	var lastErr error
	for _, ref := range CandidateBaseRefs {
		if _, err := git.ResolveRevision(ref); err != nil {
			lastErr = err
			continue
		}
		base, err := git.MergeBase("HEAD", ref)
		if err != nil {
			lastErr = err
			continue
		}
		return base, nil
	}
	return "", fmt.Errorf("no candidate base branch (%v) resolves: %w", CandidateBaseRefs, lastErr)
}

// ResolveBaseCommit runs the three-tier base-commit fallback chain used by
// the delivery gate: dynamically recomputed merge-base against the recorded
// parent branch (DynamicBaseCommit), then the SHA recorded once at claim
// time (RecordedBaseCommit), then merge-base against a default branch
// candidate (GetBaseCommit). Each tier is tried in turn; the first one that
// succeeds wins. Returns an error only if all three tiers fail.
func ResolveBaseCommit(worktreePath string, git *adapters.Client) (string, error) {
	if baseCommit, err := DynamicBaseCommit(git); err == nil {
		return baseCommit, nil
	}
	if baseCommit, err := RecordedBaseCommit(worktreePath); err == nil {
		return baseCommit, nil
	}
	baseCommit, err := GetBaseCommit(git)
	if err != nil {
		return "", fmt.Errorf("failed to determine base commit for delivery gate check: %w", err)
	}
	return baseCommit, nil
}
