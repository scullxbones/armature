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
		strings.Contains(lower, "rejected")
}

func nextActionsForOpsPublish(err error) []string {
	return nextActionsForOpsPublishClass(opsPublishClassOf(err))
}

func nextActionsForOpsPublishClass(class opsPublishClass) []string {
	switch class {
	case opsPublishClassAuth:
		return []string{
			"grant Contents: Write (fine-grained PAT) or use SSH with push access",
			"publish _armature from a write-capable environment, then git fetch origin _armature",
		}
	case opsPublishClassNonFF:
		return []string{"rebase onto origin/_armature, then arm push-ops"}
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
