package main

import (
	"errors"
	"strings"

	armerrors "github.com/scullxbones/armature/internal/errors"
)

// opsPublishClass is the classified reason a high-stakes _armature push
// failed after the one FetchAndRebase retry.
type opsPublishClass string

const (
	opsPublishClassAuth  opsPublishClass = "auth"
	opsPublishClassNonFF opsPublishClass = "non-fast-forward"
	opsPublishClassOther opsPublishClass = "other"
)

func classifyGitPushFailure(output string) opsPublishClass {
	lower := strings.ToLower(output)
	if gitPushFailureLooksLikeAuth(lower) {
		return opsPublishClassAuth
	}
	if gitPushFailureLooksLikeNonFF(lower) {
		return opsPublishClassNonFF
	}
	return opsPublishClassOther
}

func classifyGitPushFailureErr(err error) opsPublishClass {
	if err == nil {
		return opsPublishClassOther
	}
	return classifyGitPushFailure(err.Error())
}

func gitPushFailureLooksLikeAuth(lower string) bool {
	return strings.Contains(lower, "http 403") ||
		strings.Contains(lower, "error: 403") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "could not read username") ||
		strings.Contains(lower, "publickey denied") ||
		strings.Contains(lower, "denied (publickey)")
}

func gitPushFailureLooksLikeNonFF(lower string) bool {
	return strings.Contains(lower, "non-fast-forward") ||
		strings.Contains(lower, "fetch first") ||
		strings.Contains(lower, "updates were rejected because the tip")
}

func nextActionsForOpsPublish(err error) []string {
	return nextActionsForOpsPublishClass(opsPublishClassOf(err))
}

func nextActionsForOpsPublishClass(class opsPublishClass) []string {
	switch class {
	case opsPublishClassAuth:
		return []string{
			"grant Contents: Write (fine-grained PAT) or use SSH with push access, then arm push-ops",
			"publish _armature from a write-capable environment, then git fetch origin _armature",
		}
	case opsPublishClassNonFF:
		// Fetch+rebase in the ops worktree only. The destination refspec
		// updates origin/_armature; `git fetch origin _armature` can leave
		// that tracking ref stale (FETCH_HEAD only).
		return []string{
			`git -C "$(git config armature.ops-worktree-path)" fetch origin refs/heads/_armature:refs/remotes/origin/_armature`,
			`git -C "$(git config armature.ops-worktree-path)" rebase origin/_armature`,
			"arm push-ops",
		}
	default:
		return []string{"arm push-ops", "arm doctor"}
	}
}

func wrapOpsPublishFailure(code string, err error) error {
	return armerrors.Map(code, err.Error(), nextActionsForOpsPublish(err), err)
}

func opsPublishClassOf(err error) opsPublishClass {
	var pub *opsPublishError
	if errors.As(err, &pub) && pub != nil && pub.class != "" {
		return pub.class
	}
	return classifyGitPushFailureErr(err)
}
