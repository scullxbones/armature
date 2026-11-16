// Package delivery records an issue's delivered commit range in git.
package delivery

import (
	"errors"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/deliverygate"
	"github.com/scullxbones/armature/internal/materialize"
)

const refPrefix = "refs/armature/deliveries/"

// snapshot is the recorded delivery of one issue: the branch, the non-empty
// ancestor range base..tip, and the integration branch it will land on.
type snapshot struct {
	IssueID           string
	Branch            string
	Base              string
	Tip               string
	IntegrationBranch string
}

// Request is the input to Record. Base and Tip may be unresolved names.
type Request struct {
	IssueID           string
	IssueType         string
	Base              string
	Tip               string
	Branch            string
	IntegrationBranch string
	WorktreePath      string
}

// recordError is a snapshot validation failure (DELIVERY-1 at the CLI port).
type recordError struct {
	kind string
	msg  string
}

func (e *recordError) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

// MissingSnapshotError is TRANSITION-1: done was requested with no bound
// worktree and no --base/--tip, so the recovery is delivery record.
type MissingSnapshotError struct {
	IssueID string
}

func (e *MissingSnapshotError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("done requires a delivery snapshot for %s", e.IssueID)
}

// RecordArgv is the TRANSITION-1 next_actions recovery, with the issue id
// filled in and <sha> left as a placeholder (legacy / unknown provenance).
func RecordArgv(issueID string) string {
	return "arm delivery record --issue " + issueID + " --base <sha> --tip <sha>"
}

// RecordArgvFilled is the same recovery with concrete base/tip SHAs when the
// delivery range is already known (claim provenance / stored snapshot).
func RecordArgvFilled(issueID, base, tip string) string {
	return "arm delivery record --issue " + issueID + " --base " + base + " --tip " + tip
}

func RefName(issueID string) string {
	return refPrefix + issueID
}

// Record validates the range and writes refs/armature/deliveries/<id> at tip.
func Record(git *adapters.Client, req Request) (snapshot, error) {
	snap, err := validate(git, req)
	if err != nil {
		return snapshot{}, err
	}
	if err := git.UpdateRef(RefName(req.IssueID), snap.Tip); err != nil {
		return snapshot{}, fmt.Errorf("write delivery ref: %w", err)
	}
	return snap, nil
}

// validate accepts only a non-empty range of existing objects whose base
// ancestors the tip, sitting on recorded branch/claim provenance.
func validate(git *adapters.Client, req Request) (snapshot, error) {
	if req.IssueID == "" {
		return snapshot{}, newRecordError("usage", "issue ID is required")
	}
	if strings.TrimSpace(req.Base) == "" || strings.TrimSpace(req.Tip) == "" {
		return snapshot{}, newRecordError("usage", "base and tip are required")
	}

	base, baseErr := git.ResolveRevision(req.Base)
	tip, tipErr := git.ResolveRevision(req.Tip)
	if baseErr != nil || tipErr != nil {
		return snapshot{}, newRecordError("missing-objects", "delivery base or tip does not resolve to an existing object")
	}

	ancestor, err := git.IsAncestor(base, tip)
	if err != nil {
		return snapshot{}, newRecordError("missing-objects", "delivery base or tip does not resolve to an existing object")
	}
	if !ancestor {
		return snapshot{}, newRecordError("not-ancestor", "delivery base is not an ancestor of tip")
	}
	if base == tip {
		return snapshot{}, newRecordError("empty-range", "delivery range is empty: base and tip are the same commit")
	}
	changed, err := git.DiffNameOnlyRange(base, tip)
	if err != nil {
		return snapshot{}, fmt.Errorf("diff delivery range: %w", err)
	}
	if len(changed) == 0 {
		return snapshot{}, newRecordError("empty-range", "delivery range is empty: base..tip has no diff")
	}

	if err := checkWorktreeHEAD(git, req.WorktreePath, tip); err != nil {
		return snapshot{}, err
	}

	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = inferBranch(git, req)
	}

	if err := checkProvenance(git, req, tip, branch); err != nil {
		return snapshot{}, err
	}

	integration := strings.TrimSpace(req.IntegrationBranch)
	if integration == "" {
		integration = "main"
	}

	return snapshot{
		IssueID:           req.IssueID,
		Branch:            branch,
		Base:              base,
		Tip:               tip,
		IntegrationBranch: integration,
	}, nil
}

// ComputeFromWorktree fills branch/base/tip from a bound worktree.
// Base is the recorded parent tip for a --from sub-task, otherwise the
// merge-base with the integration branch.
func ComputeFromWorktree(git *adapters.Client, worktreePath, issueID, issueType, integrationBranch string) (snapshot, error) {
	if integrationBranch == "" {
		integrationBranch = "main"
	}
	tip, err := git.HeadSHA()
	if err != nil {
		return snapshot{}, fmt.Errorf("read worktree HEAD: %w", err)
	}
	branch, err := git.CurrentBranch()
	if err != nil {
		return snapshot{}, fmt.Errorf("read worktree branch: %w", err)
	}
	if rec, ok, recErr := deliverygate.RecordedClaimedBranch(worktreePath); recErr == nil && ok {
		branch = rec
	}
	base, err := resolveBase(git, worktreePath, tip, branch, integrationBranch)
	if err != nil {
		return snapshot{}, err
	}
	return validate(git, Request{
		IssueID:           issueID,
		IssueType:         issueType,
		Base:              base,
		Tip:               tip,
		Branch:            branch,
		IntegrationBranch: integrationBranch,
		WorktreePath:      worktreePath,
	})
}

func resolveBase(git *adapters.Client, worktreePath, tip, branch, integrationBranch string) (string, error) {
	parentBranch, err := git.ReadGitConfig(deliverygate.ParentBranchConfigKey(branch))
	if err == nil && parentBranch != "" && parentBranch != "HEAD" {
		if _, resErr := git.ResolveRevision(parentBranch); resErr == nil {
			if parentBranch != integrationBranch {
				if sha, recErr := deliverygate.RecordedBaseCommit(worktreePath); recErr == nil && sha != "" {
					if ok, ancErr := git.IsAncestor(sha, tip); ancErr == nil && ok {
						return sha, nil
					}
				}
			}
			if base, mbErr := git.MergeBase(tip, parentBranch); mbErr == nil {
				return base, nil
			}
		}
	}
	base, err := git.MergeBase(tip, integrationBranch)
	if err != nil {
		return "", newRecordError("missing-objects", fmt.Sprintf("merge-base with integration branch %s: %v", integrationBranch, err))
	}
	return base, nil
}

func checkWorktreeHEAD(git *adapters.Client, worktreePath, tip string) error {
	if worktreePath == "" {
		return nil
	}
	head, err := git.HeadSHA()
	if err != nil {
		return newRecordError("head-unreadable", "bound worktree HEAD is unreadable")
	}
	if head != tip {
		return newRecordError("head-disagree", "worktree HEAD disagrees with delivery tip")
	}
	return nil
}

func inferBranch(git *adapters.Client, req Request) string {
	if req.WorktreePath != "" {
		if rec, ok, err := deliverygate.RecordedClaimedBranch(req.WorktreePath); err == nil && ok {
			return rec
		}
		if b, err := git.CurrentBranch(); err == nil && b != "" && b != "HEAD" {
			return b
		}
	}
	return materialize.DeriveBranchName(req.IssueType, req.IssueID)
}

func checkProvenance(git *adapters.Client, req Request, tip, branch string) error {
	claimedTip := resolveClaimedBranchTip(git, req, branch)
	claimHEAD := ""
	if req.WorktreePath != "" {
		if sha, err := deliverygate.RecordedBaseCommit(req.WorktreePath); err == nil {
			claimHEAD = sha
		}
	}

	if claimedTip == "" && claimHEAD == "" {
		if req.IssueType != "" && materialize.DeriveBranchName(req.IssueType, req.IssueID) == "" && branch == "" {
			return nil
		}
		return newRecordError("no-provenance", "no recorded branch or claim provenance for this issue")
	}

	if claimedTip != "" {
		if tip == claimedTip {
			return nil
		}
		if ok, err := git.IsAncestor(tip, claimedTip); err == nil && ok {
			return nil
		}
	}
	if claimHEAD != "" {
		if ok, err := git.IsAncestor(claimHEAD, tip); err == nil && ok {
			return nil
		}
	}
	return newRecordError("unrelated-range", "delivery tip is not on recorded branch or claim provenance")
}

func newRecordError(kind, msg string) error {
	return &recordError{kind: kind, msg: msg}
}

func resolveClaimedBranchTip(git *adapters.Client, req Request, branch string) string {
	name := branch
	if req.WorktreePath != "" {
		if rec, ok, err := deliverygate.RecordedClaimedBranch(req.WorktreePath); err == nil && ok {
			name = rec
		}
	}
	if name == "" {
		name = materialize.DeriveBranchName(req.IssueType, req.IssueID)
	}
	if name == "" {
		return ""
	}
	sha, err := git.ResolveRevision("refs/heads/" + name)
	if err != nil {
		return ""
	}
	return sha
}

// IsRecordError reports whether err is a delivery snapshot validation failure.
func IsRecordError(err error) bool {
	var rec *recordError
	return errors.As(err, &rec)
}
