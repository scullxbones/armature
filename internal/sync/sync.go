// Package sync checks whether a branch or worktree is merge-conflict-free relative to its target before promoting a task to merged.
package sync

import (
	"errors"
	"fmt"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

type MergeChecker interface {
	BranchMergedInto(branch, target string) (bool, error)
}

// DetectMerges accepts a pre-enumerated list of issues and returns the IDs
// of done issues whose Branch has been merged into targetBranch.
// issues is a slice of materialized Issue objects.
// Branches whose merge check fails are omitted from the returned IDs; those
// failures are joined into the returned error, which names each failing branch.
func DetectMerges(issues []materialize.Issue, targetBranch string, mc MergeChecker) ([]string, error) {
	var merged []string
	var errs []error
	for _, issue := range issues {
		if issue.Status != ops.StatusDone {
			continue
		}
		if issue.Branch == "" {
			continue
		}
		isMerged, err := mc.BranchMergedInto(issue.Branch, targetBranch)
		if err != nil {
			errs = append(errs, fmt.Errorf("branch %q: %w", issue.Branch, err))
			continue
		}
		if isMerged {
			merged = append(merged, issue.ID)
		}
	}
	return merged, errors.Join(errs...)
}
