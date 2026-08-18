package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/scopematch"
)

var censusedSurfaces = map[string][]string{
	"cmd/**": {"docs/commands.md", "docs/design/surface-census.md"},
}

func checkE13VerticalSliceCoupling(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding

	byStory := make(map[string][]*materialize.Issue)
	for _, issue := range issues {
		if issue.Type != "task" || issue.Parent == "" || isTerminalStatus(issue.Status) {
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

			var codeTasks []*materialize.Issue
			for _, t := range tasks {
				if scopeTouchesSurface(t.Scope, surfaceGlob) {
					codeTasks = append(codeTasks, t)
				}
			}
			if len(codeTasks) == 0 {
				continue
			}

			var docTasks []*materialize.Issue
			for _, t := range tasks {
				for _, docFile := range docFiles {
					if scopematch.Allows(t.Scope, docFile) {
						docTasks = append(docTasks, t)
						break
					}
				}
			}
			if len(docTasks) == 0 {
				continue
			}

			seen := make(map[string]bool)
			for _, codeTask := range codeTasks {
				for _, docTask := range docTasks {
					if codeTask.ID == docTask.ID {
						continue
					}
					pairIDs := sortedIDs(codeTask.ID, docTask.ID)
					pairKey := surfaceGlob + "\x00" + pairIDs[0] + "\x00" + pairIDs[1]
					if seen[pairKey] {
						continue
					}
					seen[pairKey] = true

					owned := ownedDocFiles(docTask.Scope, docFiles)
					findings = append(findings, Finding{
						Severity: "error",
						Rule:     "E13",
						Message: fmt.Sprintf(
							"E13: %s touches censused surface %q while %s owns %s that surface's drift check reads; co-locate the census/doc lines with the code task",
							codeTask.ID, surfaceGlob, docTask.ID, strings.Join(owned, ", "),
						),
						CitedIDs: pairIDs,
						Key:      pairKey,
					})
				}
			}
		}
	}

	return findings
}

func scopeTouchesSurface(scope []string, surfaceGlob string) bool {
	for _, entry := range scope {
		if scopematch.Overlaps(entry, surfaceGlob) {
			return true
		}
	}
	return false
}

func ownedDocFiles(scope []string, docFiles []string) []string {
	var owned []string
	for _, docFile := range docFiles {
		if scopematch.Allows(scope, docFile) {
			owned = append(owned, docFile)
		}
	}
	return owned
}

func sortedIDs(a, b string) []string {
	ids := []string{a, b}
	sort.Strings(ids)
	return ids
}
