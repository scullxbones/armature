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

// BaseCommitFileName is the name of the file (written into a worktree's
// actual git directory, alongside armature-issue-id) that records the SHA
// the task branch diverged from at claim time. The delivery gate reads this
// to scope-check against the real branch-point rather than merge-basing
// against a default branch, which is wrong whenever the task branch was cut
// from a story branch containing completed sibling-task commits.
const BaseCommitFileName = "armature-base-commit"

// ClaimedBranchFileName is the name of the file (written into a worktree's
// actual git directory, alongside armature-issue-id and armature-base-commit)
// that records the branch name the issue was actually claimed under — derived
// from materialize.DeriveBranchName against the issue's TYPE AT CLAIM TIME.
// The delivery gate's VerifyIssueBranchBinding reads this (via
// RecordedClaimedBranch) and prefers it over re-deriving the expected branch
// from the CURRENT issue type, which may have been amended after claim (e.g.
// task -> epic) to route around the branch-binding check.
const ClaimedBranchFileName = "armature-claimed-branch"

// ParentBranchConfigKey returns the git config key used to durably record,
// on the shared (main-repo) git config, the branch a task branch was cut
// from. Recorded as git config rather than a per-worktree file: git config
// --local written from a linked worktree lands in the main repo's shared
// .git/config (armature does not enable the worktreeConfig extension), so
// the record survives `arm merged` removing the worktree, and stays
// addressable by branch name if the worktree is later recreated.
//
// Known limitation: the key is derived solely from the branch name, with no
// staleness check beyond the literal-"HEAD" guard in DynamicBaseCommit. If a
// branch name is recycled for a new, unrelated task after the old task
// merged and its marker was never cleaned up, DynamicBaseCommit would
// merge-base against the stale recorded parent. Branch-name recycling
// immediately after merge is out of scope for this fix.
func ParentBranchConfigKey(branchName string) string {
	return "branch." + branchName + ".armature-parent"
}

// CandidateBaseRefs are tried in order to find the branch a task diverged
// from. Remote-tracking refs are preferred over local branches: a local
// `main`/`master` in a long-lived coordinator checkout is frequently stale
// (fast-forwarded only on release), whereas `origin/main` reflects the
// actual upstream tip workers branched from.
var CandidateBaseRefs = []string{"origin/main", "origin/master", "main", "master"}

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

// VerifyIssueBranchBinding fails closed unless worktreePath's current git
// branch (HEAD) is the expected task branch for issueID, derived the same
// way claim.go's createWorktreeAndBranch does (see materialize.DeriveBranchName).
// The armature-issue-id marker file checked by VerifyIssueWorktreeBinding only
// proves the worktree was once claimed for this issue — it does not prove
// HEAD is still on the branch the coordinator will actually integrate. A
// worker could check out an unrelated scratch branch after claiming and
// still pass that check, silently stranding otherwise-valid commits off the
// task branch. issueType empty or unmapped (DeriveBranchName returns "")
// skips this check, matching the caller's existing task/bug/feature gating —
// UNLESS claimedBy is non-empty, in which case skipping is never safe (see
// below).
//
// claimedBy is the issue's CURRENT ClaimedBy (materialize.Issue.ClaimedBy),
// passed separately from issueType because both can be amended
// independently after claim. It is used only for the no-record fallback
// path below.
func VerifyIssueBranchBinding(worktreePath, issueID, issueType, claimedBy string) error {
	// Prefer the branch recorded immutably at claim time over re-deriving it
	// from the CURRENT issue type: the issue's type can be amended after
	// claim (e.g. task -> epic, which has no branch mapping), and re-deriving
	// from that amended type would make this check silently no-op, letting
	// commits on an arbitrary scratch branch through. If a claimed-branch
	// record exists, it wins even when DeriveBranchName(current type) would
	// return "" — that mismatch is exactly the bypass this guards against, so
	// it fails closed rather than skipping.
	expectedBranch, recorded, err := RecordedClaimedBranch(worktreePath)
	if err != nil {
		return fmt.Errorf("read recorded claimed branch for %s: %w. Use --skip-delivery-gate to bypass", worktreePath, err)
	}
	if !recorded {
		// No claimed-branch record (pre-migration worktree, claimed before this
		// record existed): fall back to re-deriving from the current issue type.
		expectedBranch = materialize.DeriveBranchName(issueType, issueID)
		if expectedBranch == "" {
			if claimedBy != "" {
				// A pre-migration worktree (no armature-claimed-branch marker)
				// for an issue that is STILL claimed must never be read as
				// "nothing to check" just because the current type has no
				// branch mapping — the issue may have been retyped (e.g.
				// task -> epic) after claim specifically to route around this
				// check while the claim, and whatever worktree/branch it's
				// bound to, is still live. Absence of a record is not
				// evidence of absence of a binding to verify: fail closed.
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
// claimed after this mechanism was introduced, so callers can fall back to
// GetBaseCommit.
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

// GatedBaseCommit returns the base commit the delivery gate must scope-check
// against, trusting ONLY facts actually recorded at claim time: the
// dynamically-recomputed merge-base against the recorded parent branch
// (DynamicBaseCommit — the parent branch NAME is the recorded fact,
// git-config, written once at claim; recomputing the merge-base against it
// fresh on every check is what lets a rebased task branch or a
// removed-and-recreated worktree keep resolving correctly, see
// TestDeliveryGateSurvivesWorktreeRecreation_REQ_LNGHZN_S4_T1 and
// TestDeliveryGateSurvivesRebaseOntoUpdatedParent_REQ_LNGHZN_S4_T1), then the
// SHA recorded once at claim time (RecordedBaseCommit) if no parent-branch
// record exists (worktrees claimed before that config was introduced).
//
// Deliberately does NOT fall through to GetBaseCommit: that tier has no
// recorded claim-time fact behind it at all — it merge-bases against
// whatever candidate default branch (origin/main, etc.) happens to resolve
// RIGHT NOW, so letting it stand in for gating purposes would let the gate
// pass against data nobody actually recorded for this claim, using the
// repository's current shape as if it were the claim's actual base. If
// neither a parent-branch config nor a recorded base-commit file exists for
// worktreePath (e.g. it was claimed before either mechanism existed), that
// must fail the gate closed rather than falling through to that guess.
func GatedBaseCommit(worktreePath, issueID string, git *adapters.Client) (string, error) {
	baseCommit, err := DynamicBaseCommit(git)
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

// ResolveBaseCommit runs the three-tier base-commit fallback chain for
// NON-GATING callers that want the best available guess at a task branch's
// divergence point (e.g. informational/reporting use). Gating (arm
// transition --to done) must use GatedBaseCommit instead — see its doc
// comment for why the fallback tiers here are unsafe as a gating input.
func ResolveBaseCommit(worktreePath string, git *adapters.Client) (string, error) {
	var lastErr error

	baseCommit, err := DynamicBaseCommit(git)
	if err == nil {
		return baseCommit, nil
	}
	lastErr = err

	baseCommit, err = RecordedBaseCommit(worktreePath)
	if err == nil {
		return baseCommit, nil
	}
	lastErr = fmt.Errorf("%w; recorded base commit also failed: %w", lastErr, err)

	baseCommit, err = GetBaseCommit(git)
	if err != nil {
		lastErr = fmt.Errorf("%w; default base branch lookup also failed: %w", lastErr, err)
		return "", fmt.Errorf("failed to determine base commit for delivery gate check: %w", lastErr)
	}
	return baseCommit, nil
}
