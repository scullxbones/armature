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
		if issue.Type != "task" || issue.Parent == "" {
			continue
		}
		byStory[issue.Parent] = append(byStory[issue.Parent], issue)
	}

	for _, tasks := range byStory {
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

		for surfaceGlob, docFiles := range censusedSurfaces {
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

			for _, codeTask := range codeTasks {
				for _, docTask := range docTasks {
					if codeTask.ID == docTask.ID {
						continue
					}
					owned := ownedDocFiles(docTask.Scope, docFiles)
					findings = append(findings, Finding{
						Severity: "error",
						Rule:     "E13",
						Message: fmt.Sprintf(
							"E13: %s touches censused surface %q while %s owns %s that surface's drift check reads; co-locate the census/doc lines with the code task",
							codeTask.ID, surfaceGlob, docTask.ID, strings.Join(owned, ", "),
						),
						CitedIDs: sortedIDs(codeTask.ID, docTask.ID),
						Key:      surfaceGlob + "\x00" + codeTask.ID + "\x00" + docTask.ID,
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
