// Package sync checks whether a branch or worktree is merge-conflict-free relative to its target before promoting a task to merged.
package sync

import (
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// MergeChecker checks if a branch is merged into a target branch.
type MergeChecker interface {
	BranchMergedInto(branch, target string) (bool, error)
}

// DetectMerges accepts a pre-enumerated list of issues and returns the IDs
// of done issues whose Branch has been merged into targetBranch.
// issues is a slice of materialized Issue objects.
func DetectMerges(issues []materialize.Issue, targetBranch string, mc MergeChecker) ([]string, error) {
	var merged []string
	for _, issue := range issues {
		if issue.Status != ops.StatusDone {
			continue
		}
		if issue.Branch == "" {
			continue
		}
		isMerged, err := mc.BranchMergedInto(issue.Branch, targetBranch)
		if err != nil {
			continue // skip on error, don't abort
		}
		if isMerged {
			merged = append(merged, issue.ID)
		}
	}
	return merged, nil
}
