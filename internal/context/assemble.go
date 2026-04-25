package context

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
)

// Layer represents a single named, prioritized context layer.
type Layer struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"` // lower = higher priority (1 = highest)
	Content  string `json:"content"`
}

// Context holds all assembled layers for an issue.
type Context struct {
	IssueID string  `json:"issue_id"`
	Layers  []Layer `json:"layers"` // ordered by priority ascending
}

// Assemble builds a 7-layer context for the given issue from state.
func Assemble(issueID string, stateDir string, state *materialize.State) (*Context, error) {
	issue, ok := state.Issues[issueID]
	if !ok {
		return nil, fmt.Errorf("issue %s not found in state", issueID)
	}

	var layers []Layer

	// Layer 1: core_spec
	layers = append(layers, buildCoreSpec(issue))

	// Layer 2: snippets
	layers = append(layers, buildSnippets(issue))

	// Layer 3: blocker_outcomes
	layers = append(layers, buildBlockerOutcomes(issue, stateDir, state))

	// Layer 4: parent_chain
	layers = append(layers, buildParentChain(issue, stateDir, state))

	// Layer 5: decisions
	layers = append(layers, buildDecisions(issue))

	// Layer 6: notes
	layers = append(layers, buildNotes(issue))

	// Layer 7: sibling_outcomes
	layers = append(layers, buildSiblingOutcomes(issue, stateDir, state))

	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Priority < layers[j].Priority
	})

	return &Context{
		IssueID: issueID,
		Layers:  layers,
	}, nil
}

func buildCoreSpec(issue *materialize.Issue) Layer {
	scope := strings.Join(issue.Scope, ", ")
	if scope == "" {
		scope = "(none)"
	}
	priority := issue.Priority
	if priority == "" {
		priority = "(none)"
	}
	dod := issue.DefinitionOfDone
	if dod == "" {
		dod = "(none)"
	}
	content := fmt.Sprintf("# Issue: %s\nType: %s | Scope: %s | Priority: %s\n\n## Definition of Done\n%s",
		issue.Title, issue.Type, scope, priority, dod)
	return Layer{Name: "core_spec", Priority: 1, Content: content}
}

func buildSnippets(issue *materialize.Issue) Layer {
	if issue.Context == nil {
		return Layer{Name: "snippets", Priority: 2, Content: ""}
	}
	var ctxMap map[string]interface{}
	if err := json.Unmarshal(issue.Context, &ctxMap); err != nil {
		return Layer{Name: "snippets", Priority: 2, Content: ""}
	}
	if len(ctxMap) == 0 {
		return Layer{Name: "snippets", Priority: 2, Content: ""}
	}
	var lines []string
	for k, v := range ctxMap {
		lines = append(lines, fmt.Sprintf("%s: %v", k, v))
	}
	sort.Strings(lines)
	return Layer{Name: "snippets", Priority: 2, Content: strings.Join(lines, "\n")}
}

func buildBlockerOutcomes(issue *materialize.Issue, stateDir string, state *materialize.State) Layer {
	if len(issue.BlockedBy) == 0 {
		return Layer{Name: "blocker_outcomes", Priority: 3, Content: ""}
	}
	var lines []string
	for _, blockerID := range issue.BlockedBy {
		outcome := "outcome unknown"
		var status string
		if blocker, ok := state.Issues[blockerID]; ok {
			status = blocker.Status
			if blocker.Outcome != "" {
				outcome = blocker.Outcome
			}
		} else {
			// Try loading from disk
			path := filepath.Join(stateDir, "issues", blockerID+".json")
			if b, err := materialize.LoadIssue(path); err == nil {
				status = b.Status
				if b.Outcome != "" {
					outcome = b.Outcome
				}
			}
		}
		// Include status alongside outcome for unambiguous signal
		if outcome == "outcome unknown" && status != "" {
			outcome = fmt.Sprintf("%s (outcome unknown)", status)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", blockerID, outcome))
	}
	content := "## Blocking Issue Outcomes\n" + strings.Join(lines, "\n")
	return Layer{Name: "blocker_outcomes", Priority: 3, Content: content}
}

func buildParentChain(issue *materialize.Issue, stateDir string, state *materialize.State) Layer {
	var lines []string
	currentParentID := issue.Parent
	for range 3 {
		if currentParentID == "" {
			break
		}
		parentID := currentParentID
		var parentTitle, parentStatus, nextParentID string
		if parent, ok := state.Issues[parentID]; ok {
			parentTitle = parent.Title
			parentStatus = parent.Status
			nextParentID = parent.Parent
		} else {
			path := filepath.Join(stateDir, "issues", parentID+".json")
			if p, err := materialize.LoadIssue(path); err == nil {
				parentTitle = p.Title
				parentStatus = p.Status
				nextParentID = p.Parent
			} else {
				break
			}
		}
		lines = append(lines, fmt.Sprintf("- %s: %s [%s]", parentID, parentTitle, parentStatus))
		currentParentID = nextParentID
	}
	if len(lines) == 0 {
		return Layer{Name: "parent_chain", Priority: 4, Content: ""}
	}
	content := "## Parent Chain\n" + strings.Join(lines, "\n")
	return Layer{Name: "parent_chain", Priority: 4, Content: content}
}

func buildDecisions(issue *materialize.Issue) Layer {
	if len(issue.Decisions) == 0 {
		return Layer{Name: "decisions", Priority: 5, Content: ""}
	}
	var lines []string
	for _, d := range issue.Decisions {
		lines = append(lines, fmt.Sprintf("- %s: %s — %s", d.Topic, d.Choice, d.Rationale))
	}
	content := "## Decisions\n" + strings.Join(lines, "\n")
	return Layer{Name: "decisions", Priority: 5, Content: content}
}

func buildNotes(issue *materialize.Issue) Layer {
	if len(issue.Notes) == 0 {
		return Layer{Name: "notes", Priority: 6, Content: ""}
	}
	notes := issue.Notes
	// Take most recent 5
	if len(notes) > 5 {
		notes = notes[len(notes)-5:]
	}
	var lines []string
	for _, n := range notes {
		ts := time.Unix(n.Timestamp, 0).UTC().Format(time.RFC3339)
		lines = append(lines, fmt.Sprintf("- [%s] %s", ts, n.Msg))
	}
	content := "## Notes\n" + strings.Join(lines, "\n")
	return Layer{Name: "notes", Priority: 6, Content: content}
}

func buildSiblingOutcomes(issue *materialize.Issue, stateDir string, state *materialize.State) Layer {
	if issue.Parent == "" {
		return Layer{Name: "sibling_outcomes", Priority: 7, Content: ""}
	}
	var children []string
	if parent, ok := state.Issues[issue.Parent]; ok {
		children = parent.Children
	} else {
		path := filepath.Join(stateDir, "issues", issue.Parent+".json")
		if p, err := materialize.LoadIssue(path); err == nil {
			children = p.Children
		}
	}

	var lines []string
	for _, sibID := range children {
		if sibID == issue.ID {
			continue
		}
		var sibStatus, sibOutcome string
		if sib, ok := state.Issues[sibID]; ok {
			sibStatus = sib.Status
			sibOutcome = sib.Outcome
		} else {
			path := filepath.Join(stateDir, "issues", sibID+".json")
			if s, err := materialize.LoadIssue(path); err == nil {
				sibStatus = s.Status
				sibOutcome = s.Outcome
			}
		}
		if sibStatus == "done" || sibStatus == "merged" {
			outcome := sibOutcome
			if outcome == "" {
				outcome = "(none)"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", sibID, outcome))
		}
	}
	if len(lines) == 0 {
		return Layer{Name: "sibling_outcomes", Priority: 7, Content: ""}
	}
	content := "## Sibling Outcomes\n" + strings.Join(lines, "\n")
	return Layer{Name: "sibling_outcomes", Priority: 7, Content: content}
}
