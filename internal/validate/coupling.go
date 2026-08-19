package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/scopematch"
)

// censusedSurfaces restates the Censused Surfaces table in
// docs/design/surface-census.md, which is authoritative. The two are held
// together by TestCensusedSurfacesMatchesCensusDoc_REQ_LNGHZN_S10_T5 -- without
// that gate, adding a surface to the census would silently stop E13 covering it.
var censusedSurfaces = map[string][]string{
	"cmd/**": {"docs/commands.md", "docs/design/surface-census.md"},
}

// checkE13VerticalSliceCoupling asks a per-task question: does this task touch a
// censused surface without owning any of the doc lines that surface's drift check
// reads, while a sibling in the same story owns them? A task that carries both its
// code and its own census/doc lines is a vertical slice and is exempt by
// construction. One finding per offending task, citing every implicated sibling.
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

			var docOwners []*materialize.Issue
			for _, t := range tasks {
				if len(ownedDocFiles(t.Scope, docFiles)) > 0 {
					docOwners = append(docOwners, t)
				}
			}
			if len(docOwners) == 0 {
				continue
			}

			for _, t := range tasks {
				if !scopeTouchesSurface(t.Scope, surfaceGlob) {
					continue
				}
				// Same-task ownership is co-location, not coupling.
				if len(ownedDocFiles(t.Scope, docFiles)) > 0 {
					continue
				}

				ownerIDs := make([]string, 0, len(docOwners))
				var owned []string
				for _, owner := range docOwners {
					ownerIDs = append(ownerIDs, owner.ID)
					owned = append(owned, ownedDocFiles(owner.Scope, docFiles)...)
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

// scopeTouchesSurface reports whether a scope entry definitely lands inside the
// censused surface. This is Allows-shaped (does the surface glob cover this
// entry), deliberately not Overlaps-shaped: Overlaps documents itself as an
// over-approximation calibrated for a warning with a --force escape, and would
// read a repo-wide scope such as "." or "**" as a phantom cmd/** code task.
func scopeTouchesSurface(scope []string, surfaceGlob string) bool {
	for _, entry := range scope {
		cleaned, _ := scopematch.CleanScope(entry)
		if scopematch.Allows([]string{surfaceGlob}, cleaned) {
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
