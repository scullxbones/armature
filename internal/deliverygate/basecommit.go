package deliverygate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/worktree"
)

const BaseCommitFileName = "armature-base-commit"

const ClaimedBranchFileName = "armature-claimed-branch"

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

// ResolveWorktreeRoot resolves path to the top-level directory of the git
// worktree that contains it, walking up through parent directories the way
// git itself does (via `git rev-parse --show-toplevel`). worktree.ResolveGitDir
// (and the checks built on it, e.g. VerifyIssueWorktreeBinding) stat
// `<path>/.git` directly with no walk-up, so passing a subdirectory of a
// worktree fails with "stat .git: no such file or directory" even though the
// path IS inside a valid worktree. Callers that may receive a subdirectory
// (e.g. the default "." when a command is run from anywhere inside a
// worktree) should resolve through this first. If path is not inside a git
// working tree at all, both the resolution here and the direct stat down-
// stream fail the same way, so returning the original path on error is safe:
// it never widens what would otherwise pass.
func ResolveWorktreeRoot(path string) (string, error) {
	toplevel, err := adapters.New(path).Toplevel()
	if err != nil {
		return path, fmt.Errorf("resolve worktree top level for %s: %w", path, err)
	}
	return toplevel, nil
}

// VerifyIssueWorktreeBinding fails closed unless worktreePath is the actual
// worktree bound to issueID (the issue-ID marker file written by
// updateIssueIDFile at claim time — see harnesshook.ReadIssueBindingFileErr).
// This prevents `arm transition --to done --repo <some-other-checkout>` from
// running the delivery gate against a directory that isn't the claimed
// worktree for issueID, which would let a dirty or out-of-scope claimed
// worktree pass because the wrong directory was checked instead.
func VerifyIssueWorktreeBinding(worktreePath, issueID string) error {
	gitDir, err := worktree.ResolveGitDir(worktreePath)
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

func VerifyIssueBranchBinding(worktreePath, issueID, issueType, claimedBy string) error {
	expectedBranch, recorded, err := RecordedClaimedBranch(worktreePath)
	if err != nil {
		return fmt.Errorf("read recorded claimed branch for %s: %w. Use --skip-delivery-gate to bypass", worktreePath, err)
	}
	if !recorded {
		expectedBranch = materialize.DeriveBranchName(issueType, issueID)
		if expectedBranch == "" {
			if claimedBy != "" {
				return fmt.Errorf(
					"issue %s is claimed by %s but has no recorded claimed-branch marker and its "+
						"current type %q has no branch mapping: cannot verify branch binding for a "+
						"claimed issue. Re-claim to record the marker, or use --skip-delivery-gate to bypass",
					issueID, claimedBy, issueType)
			}
			return nil
		}
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
// claimed after this mechanism was introduced.
func RecordedBaseCommit(worktreePath string) (string, error) {
	actualGitDir, err := worktree.ResolveGitDir(worktreePath)
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

// RecordedClaimedBranch reads the branch name persisted at claim time (see
// writeClaimedBranchFileIfAbsent in cmd/armature/claim.go) from the
// worktree's actual git directory. Returns (_, false, nil) — not an error —
// when the marker file simply doesn't exist, since that's the expected state
// for worktrees claimed before this mechanism was introduced (or for
// branchless types like epic/story, for which the marker is never written);
// callers should fall back to re-deriving the expected branch in that case.
// Any other read error (e.g. permission denied, or an unresolvable worktree
// git dir) is returned as an error so it isn't silently treated the same as
// "not recorded".
func RecordedClaimedBranch(worktreePath string) (string, bool, error) {
	actualGitDir, err := worktree.ResolveGitDir(worktreePath)
	if err != nil {
		return "", false, fmt.Errorf("resolve worktree git dir: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(actualGitDir, ClaimedBranchFileName)) //nolint:gosec // G304: derived from a trusted git directory
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	branch := strings.TrimSpace(string(data))
	if branch == "" {
		return "", false, fmt.Errorf("claimed branch file is empty")
	}
	return branch, true, nil
}

func dynamicBaseCommit(git *adapters.Client) (string, error) {
	currentBranch, err := git.CurrentBranch()
	if err != nil || currentBranch == "" {
		return "", fmt.Errorf("determine current branch: %w", err)
	}
	parentBranch, err := git.ReadGitConfig(ParentBranchConfigKey(currentBranch))
	if err != nil || parentBranch == "" {
		return "", fmt.Errorf("no recorded parent branch for %s: %w", currentBranch, err)
	}
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

func GatedBaseCommit(worktreePath, issueID string, git *adapters.Client) (string, error) {
	baseCommit, err := dynamicBaseCommit(git)
	if err == nil {
		return baseCommit, nil
	}
	dynamicErr := err

	baseCommit, err = RecordedBaseCommit(worktreePath)
	if err == nil {
		return baseCommit, nil
	}

	return "", fmt.Errorf(
		"no recorded base commit for claimed issue %s (dynamic parent-branch merge-base failed: %v; recorded base-commit file also failed: %w)\n"+
			"this worktree predates delivery-gate claim recording: re-claim it, or use --skip-delivery-gate to bypass",
		issueID, dynamicErr, err)
}
