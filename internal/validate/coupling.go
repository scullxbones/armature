package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/scopematch"
)

func checkE13VerticalSliceCoupling(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding

	byStory := make(map[string][]*materialize.Issue)
	for _, issue := range issues {
		if issue.Type != "task" || issue.Parent == "" || ops.IsTerminalStatus(issue.Status) {
			continue
		}
		byStory[issue.Parent] = append(byStory[issue.Parent], issue)
	}

	storyIDs := make([]string, 0, len(byStory))
	for storyID := range byStory {
		storyIDs = append(storyIDs, storyID)
	}
	sort.Strings(storyIDs)

	surfaceGlobs := make([]string, 0, len(censusedSurfaces))
	for surfaceGlob := range censusedSurfaces {
		surfaceGlobs = append(surfaceGlobs, surfaceGlob)
	}
	sort.Strings(surfaceGlobs)

	for _, storyID := range storyIDs {
		tasks := byStory[storyID]
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

		for _, surfaceGlob := range surfaceGlobs {
			docFiles := censusedSurfaces[surfaceGlob]

			var docOwners []*materialize.Issue
			for _, t := range tasks {
				if len(docFilesOwnedExcludingRepoWide(t.Scope, docFiles)) > 0 {
					docOwners = append(docOwners, t)
				}
			}
			if len(docOwners) == 0 {
				continue
			}

			for _, t := range tasks {
				if !surfaceGlobAllowsScopeEntry(t.Scope, surfaceGlob) {
					continue
				}
				if len(docFilesOwnedExcludingRepoWide(t.Scope, docFiles)) > 0 {
					continue
				}

				ownerIDs := make([]string, 0, len(docOwners))
				var owned []string
				for _, owner := range docOwners {
					ownerIDs = append(ownerIDs, owner.ID)
					owned = append(owned, docFilesOwnedExcludingRepoWide(owner.Scope, docFiles)...)
				}
				if len(ownerIDs) == 0 {
					continue
				}

				cited := append([]string{t.ID}, ownerIDs...)
				sort.Strings(cited)
				findings = append(findings, Finding{
					Severity: "error",
					Rule:     "E13",
					Message: fmt.Sprintf(
						"E13: %s touches censused surface %q while %s owns %s that surface's drift check reads; co-locate the census/doc lines with the code task",
						t.ID, surfaceGlob, strings.Join(ownerIDs, ", "), strings.Join(dedupeSorted(owned), ", "),
					),
					CitedIDs: cited,
					Key:      surfaceGlob + "\x00" + t.ID,
				})
			}
		}
	}

	return findings
}

func surfaceGlobAllowsScopeEntry(scope []string, surfaceGlob string) bool {
	for _, entry := range scope {
		cleaned, _ := scopematch.CleanScope(entry)
		if scopematch.Allows([]string{surfaceGlob}, cleaned) {
			return true
		}
	}
	return false
}

func dropRepoWideScopeEntriesBeforeCouplingCheck(scope []string) []string {
	named := make([]string, 0, len(scope))
	for _, entry := range scope {
		if cleaned, _ := scopematch.CleanScope(entry); cleaned == "." || cleaned == "**" {
			continue
		}
		named = append(named, entry)
	}
	return named
}

func docFilesOwnedExcludingRepoWide(scope []string, docFiles []string) []string {
	named := dropRepoWideScopeEntriesBeforeCouplingCheck(scope)
	var owned []string
	for _, docFile := range docFiles {
		if scopematch.Allows(named, docFile) {
			owned = append(owned, docFile)
		}
	}
	return owned
}

func dedupeSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
