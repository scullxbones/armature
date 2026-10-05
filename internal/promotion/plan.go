// Package promotion is the one ADR 0022 writer: match a recorded delivery on
// the integration branch and require a matching assessment (or post-delivery
// Release Override bound to those SHAs). arm merged and arm sync both call
// Evaluate; they append a merged transition only on a pass.
package promotion

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

const (
	KindPromote           = "promote"
	KindNotOnTarget       = "not-on-target"
	KindMissingAssessment = "missing-assessment"
	KindEmptyDiff         = "empty-diff"
	KindLegacy            = "legacy"
	KindCheckFailed       = "check-failed"
	KindNotLeaf           = "not-leaf"
	KindNotDone           = "not-done"
)

// Input is the borrowed snapshot Evaluate needs. PriorOps is the source of
// attestations, post-delivery overrides, and promotion-check dedup.
type Input struct {
	Issue       materialize.Issue
	PriorOps    []ops.Op
	Integration string
	PR          string
}

// Result is one check of one done leaf. Callers append CheckPayload when
// AppendCheck is set, and the merged transition only when Promote is true.
type Result struct {
	IssueID         string
	Kind            string
	Promote         bool
	TargetSHA       string
	CombinedPatchID string
	MatchedCommit   string
	CheckPayload    ops.Payload
	MergedPayload   ops.Payload
	AppendCheck     bool
	Recovery        string
	Err             error
}

// Error is a promotion refusal the CLI maps to MERGED-1 / sync rows.
type Error struct {
	IssueID string
	Kind    string
	Msg     string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

func (e *Error) RecoveryArgv() string {
	if e == nil {
		return "arm doctor"
	}
	return RecoveryArgv(e.IssueID, e.Kind)
}

// RecoveryArgv is next_actions[0] / help[0] with the issue id filled in.
func RecoveryArgv(issueID, kind string) string {
	switch kind {
	case KindMissingAssessment:
		return "arm review record --issue " + issueID + " --assessment <assessment.json>"
	case KindLegacy, KindEmptyDiff:
		return delivery.RecordArgv(issueID)
	case KindNotOnTarget, KindNotLeaf:
		return "arm merged --issue " + issueID
	default:
		return "arm doctor"
	}
}

// Evaluate is the one ADR 0022 promotion function.
func Evaluate(git *adapters.Client, in Input) Result {
	id := in.Issue.ID
	res := Result{IssueID: id, Recovery: RecoveryArgv(id, KindCheckFailed)}
	if in.Issue.Status != ops.StatusDone {
		res.Kind = KindNotDone
		res.Recovery = RecoveryArgv(id, KindNotDone)
		return res
	}
	if len(in.Issue.Children) > 0 {
		res.Kind = KindNotLeaf
		res.Recovery = RecoveryArgv(id, KindNotLeaf)
		res.Err = &Error{IssueID: id, Kind: KindNotLeaf, Msg: "issue " + id + " is not a leaf; parents roll up when children are merged"}
		return res
	}

	base, tip, integration := deliveryOf(in)
	if base == "" || tip == "" {
		res.Kind = KindLegacy
		res.Recovery = RecoveryArgv(id, KindLegacy)
		res.Err = &Error{IssueID: id, Kind: KindLegacy, Msg: "done issue " + id + " has no delivery snapshot"}
		return res
	}

	target, err := git.ResolveRevision(integration)
	if err != nil {
		res.Kind = KindCheckFailed
		res.Err = fmt.Errorf("resolve integration branch %s: %w", integration, err)
		res.Recovery = RecoveryArgv(id, KindCheckFailed)
		return failCheck(res, in, target, tip, KindCheckFailed)
	}
	res.TargetSHA = target

	patchID, matched, kind, matchErr := matchDelivery(git, base, tip, target)
	if matchErr != nil {
		res.Kind = KindCheckFailed
		res.CombinedPatchID = patchID
		res.Err = matchErr
		res.Recovery = RecoveryArgv(id, KindCheckFailed)
		return failCheck(res, in, target, tip, KindCheckFailed)
	}
	res.CombinedPatchID = patchID
	res.MatchedCommit = matched

	if kind == KindEmptyDiff || kind == KindNotOnTarget {
		res.Kind = kind
		res.Recovery = RecoveryArgv(id, kind)
		res.Err = &Error{IssueID: id, Kind: kind, Msg: checkMessage(id, kind)}
		return failCheck(res, in, target, tip, kind)
	}

	if !hasMatchingAssessment(in, base, tip) {
		res.Kind = KindMissingAssessment
		res.Recovery = RecoveryArgv(id, KindMissingAssessment)
		res.Err = &Error{IssueID: id, Kind: KindMissingAssessment, Msg: checkMessage(id, KindMissingAssessment)}
		return failCheck(res, in, target, tip, KindMissingAssessment)
	}

	res.Kind = KindPromote
	res.Promote = true
	res.Recovery = ""
	res.CheckPayload = checkPayload(target, tip, KindPromote, patchID, matched)
	res.AppendCheck = !alreadyChecked(in.PriorOps, id, target, tip, KindPromote)
	res.MergedPayload = ops.Payload{
		To:              ops.StatusMerged,
		PR:              in.PR,
		TargetSHA:       target,
		CombinedPatchID: patchID,
		MatchedCommit:   matched,
	}
	return res
}

func checkMessage(id, kind string) string {
	switch kind {
	case KindEmptyDiff:
		return "delivery range for " + id + " is empty; nothing matches"
	case KindNotOnTarget:
		return "delivery for " + id + " is not on the integration branch"
	case KindMissingAssessment:
		return "delivery for " + id + " is on the target but no matching assessment is recorded"
	default:
		return "promotion check for " + id + " failed"
	}
}

func failCheck(res Result, in Input, target, tip, kind string) Result {
	res.CheckPayload = checkPayload(target, tip, kind, res.CombinedPatchID, res.MatchedCommit)
	res.AppendCheck = target != "" && tip != "" && !alreadyChecked(in.PriorOps, in.Issue.ID, target, tip, kind)
	return res
}

func checkPayload(target, tip, result, patchID, matched string) ops.Payload {
	return ops.Payload{
		TargetSHA:       target,
		Tip:             tip,
		Result:          result,
		CombinedPatchID: patchID,
		MatchedCommit:   matched,
	}
}

func deliveryOf(in Input) (base, tip, integration string) {
	base = in.Issue.Base
	tip = in.Issue.Tip
	integration = strings.TrimSpace(in.Integration)
	if integration == "" {
		integration = strings.TrimSpace(in.Issue.IntegrationBranch)
	}
	if integration == "" {
		integration = "main"
	}
	if base != "" && tip != "" {
		return base, tip, integration
	}
	last, ok := ops.LastTransitionPayload(in.PriorOps, in.Issue.ID)
	if !ok || last.To != ops.StatusDone {
		return base, tip, integration
	}
	if base == "" {
		base = last.Base
	}
	if tip == "" {
		tip = last.Tip
	}
	if strings.TrimSpace(in.Integration) == "" && last.IntegrationBranch != "" {
		integration = last.IntegrationBranch
	}
	return base, tip, integration
}

func matchDelivery(git *adapters.Client, base, tip, target string) (patchID, matched, kind string, err error) {
	if base == tip {
		return "", "", KindEmptyDiff, nil
	}
	paths, err := git.DiffNameOnlyTwoDot(base, tip)
	if err != nil {
		return "", "", KindCheckFailed, err
	}
	diff, err := git.DiffTwoDot(base, tip)
	if err != nil {
		return "", "", KindCheckFailed, err
	}
	if len(paths) == 0 || strings.TrimSpace(diff) == "" {
		return "", "", KindEmptyDiff, nil
	}
	patchID, err = git.StablePatchID(diff)
	if err != nil {
		return "", "", KindCheckFailed, err
	}

	commits, err := git.FirstParentAfter(base, target)
	if err != nil {
		return patchID, "", KindCheckFailed, err
	}
	skipTreeOnly := containsRename(git, base, tip, paths)
	for _, sha := range commits {
		ok, matchErr := qualifies(git, sha, tip, diff, patchID, paths, skipTreeOnly)
		if matchErr != nil {
			return patchID, "", KindCheckFailed, matchErr
		}
		if ok {
			return patchID, sha, KindPromote, nil
		}
	}
	return patchID, "", KindNotOnTarget, nil
}

// containsRename is the ADR 0022 rename exception: git -M rename detection
// (including content-modifying renames). Mixed add/delete of dissimilar
// blobs must still use tree matching.
func containsRename(git *adapters.Client, base, tip string, _ []string) bool {
	entries, err := git.DiffNameStatusRange(base, tip)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Status, "R") {
			return true
		}
	}
	return false
}

func qualifies(git *adapters.Client, sha, tip, deliveryDiff, deliveryPatchID string, paths []string, skipTreeOnly bool) (bool, error) {
	isAnc, err := git.IsAncestor(tip, sha)
	if err != nil {
		return false, err
	}
	if isAnc {
		return true, nil
	}

	commitDiff, err := git.CommitDiff(sha)
	if err != nil {
		return false, err
	}
	commitPatchID, err := git.StablePatchID(commitDiff)
	if err != nil {
		return false, err
	}
	if deliveryPatchID != "" && deliveryPatchID == commitPatchID {
		if commitDiff == deliveryDiff {
			return true, nil
		}
		eq, treeErr := treeEqual(git, sha, tip, paths)
		if treeErr != nil {
			return false, treeErr
		}
		if eq {
			return true, nil
		}
	}
	if skipTreeOnly {
		return false, nil
	}
	return treeEqual(git, sha, tip, paths)
}

func treeEqual(git *adapters.Client, sha, tip string, paths []string) (bool, error) {
	if len(paths) == 0 {
		return false, nil
	}
	for _, p := range paths {
		modeC, oidC, err := git.TreeEntry(sha, p)
		if err != nil {
			return false, err
		}
		modeT, oidT, err := git.TreeEntry(tip, p)
		if err != nil {
			return false, err
		}
		if modeC != modeT || oidC != oidT {
			return false, nil
		}
	}
	return true, nil
}

func hasMatchingAssessment(in Input, base, tip string) bool {
	for _, att := range in.Issue.AssessmentAttestations {
		if att.BaseSHA == base && att.HeadSHA == tip {
			return true
		}
	}
	// PriorOps may arrive in per-worker filename order; scan in the
	// materializer's timestamp-then-type replay order so a later override
	// from another worker is still after its delivery transition.
	ordered := append([]ops.Op(nil), in.PriorOps...)
	claim.SortForReplay(ordered)
	deliverySeen := false
	for _, op := range ordered {
		if op.TargetID != in.Issue.ID {
			continue
		}
		switch op.Type {
		case ops.OpAssessmentAttested:
			var att struct {
				BaseSHA string `json:"base_sha"`
				HeadSHA string `json:"head_sha"`
			}
			if err := json.Unmarshal(op.Payload.Assessment, &att); err != nil {
				continue
			}
			if att.BaseSHA == base && att.HeadSHA == tip {
				return true
			}
		case ops.OpTransition:
			if op.Payload.To == ops.StatusDone && op.Payload.Base == base && op.Payload.Tip == tip {
				deliverySeen = true
			}
		case ops.OpDAGTransition:
			if !deliverySeen {
				continue
			}
			if op.Payload.SkippedValidateGate &&
				op.Payload.To == "verified" &&
				strings.TrimSpace(op.Payload.Rationale) != "" &&
				op.Payload.Base == base &&
				op.Payload.Tip == tip {
				return true
			}
		}
	}
	return false
}

func alreadyChecked(prior []ops.Op, issueID, targetSHA, tip, result string) bool {
	for _, op := range prior {
		if op.Type != ops.OpPromotionCheck || op.TargetID != issueID {
			continue
		}
		if op.Payload.TargetSHA == targetSHA && op.Payload.Tip == tip && op.Payload.Result == result {
			return true
		}
	}
	return false
}
