package sync

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
)

// MergeChecker checks if a branch is merged into a target branch.
type MergeChecker interface {
	BranchMergedInto(branch, target string) (bool, error)
}

// DetectMerges scans all issues in issuesDir/state/issues/ and returns the IDs
// of done issues whose Branch has been merged into targetBranch.
func DetectMerges(issuesDir, targetBranch string, mc MergeChecker) ([]string, error) {
	issuesStateDir := filepath.Join(issuesDir, "state", "issues")
	entries, err := os.ReadDir(issuesStateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var merged []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		issue, err := materialize.LoadIssue(filepath.Join(issuesStateDir, entry.Name()))
		if err != nil {
			continue
		}
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
