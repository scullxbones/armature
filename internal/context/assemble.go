// Package context assembles and renders the layered context bundle (spec, scope, citations)
// passed to workers via render-context, with truncation to keep bundles within budget.
package context

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/materialize"
)

const (
	minMarkdownFence      = 3
	parentChainDisplayCap = 3
	recentNotesDisplayCap = 5
)

type Layer struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	Content  string `json:"content"`
}

type Context struct {
	IssueID string  `json:"issue_id"`
	Layers  []Layer `json:"layers"`
}

func Assemble(issueID string, state *materialize.State, reader FileReader) (*Context, error) {
	issue, ok := state.Issues[issueID]
	if !ok {
		return nil, fmt.Errorf("issue %s not found in state", issueID)
	}

	graph := materialize.GraphFromState(state)

	var layers []Layer

	layers = append(layers, buildCoreSpec(issue))
	layers = append(layers, buildContextFiles(issue, reader))
	layers = append(layers, buildSnippets(issue))
	layers = append(layers, buildBlockerOutcomes(issue, state))
	layers = append(layers, buildParentChain(issue, graph, state))
	layers = append(layers, buildDecisions(issue))
	layers = append(layers, buildNotes(issue))
	layers = append(layers, buildSiblingOutcomes(issue, graph, state))

	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Priority < layers[j].Priority
	})

	return &Context{
		IssueID: issueID,
		Layers:  layers,
	}, nil
}

func InferRepoRoot(stateDir string) string {
	if root := inferRepoRootByPath(stateDir); root != "" {
		return root
	}
	// Fallback: ask git, handles worktree layouts where .arm/.armature is not in stateDir's path.
	if root := inferRepoRootByGit(stateDir); root != "" {
		return root
	}
	return filepath.Clean(stateDir)
}

func inferRepoRootByPath(stateDir string) string {
	clean := filepath.Clean(stateDir)
	for dir := clean; dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		base := filepath.Base(dir)
		if base == ".arm" || base == config.StateDirName {
			return filepath.Dir(dir)
		}
	}
	return ""
}

func inferRepoRootByGit(stateDir string) string {
	dir := filepath.Clean(stateDir)
	for dir != "." && dir != string(filepath.Separator) {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		dir = filepath.Dir(dir)
	}
	if dir == "." || dir == string(filepath.Separator) {
		return ""
	}
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func buildContextFiles(issue *materialize.Issue, reader FileReader) Layer {
	if len(issue.ContextFiles) == 0 {
		return Layer{Name: "context_files", Priority: 2, Content: ""}
	}
	var sections []string
	for _, relPath := range issue.ContextFiles {
		data, err := reader.ReadFile(relPath)
		if err != nil {
			sections = append(sections, fmt.Sprintf("### %s\n(missing: %v)", relPath, err))
			continue
		}
		content := strings.TrimRight(string(data), "\n")
		fence := codeBlockFence(content)
		sections = append(sections, fmt.Sprintf("### %s\n%stext\n%s\n%s", relPath, fence, content, fence))
	}
	return Layer{
		Name:     "context_files",
		Priority: 2,
		Content:  "## Context Files\n" + strings.Join(sections, "\n\n"),
	}
}

// codeBlockFence returns a backtick fence string long enough that it cannot
// be closed prematurely by any backtick sequence in content.  The returned
// fence is at least three backticks and always one longer than the longest
// consecutive run of backticks found in content.
func codeBlockFence(content string) string {
	maxRun := 0
	cur := 0
	for _, ch := range content {
		if ch == '`' {
			cur++
			if cur > maxRun {
				maxRun = cur
			}
		} else {
			cur = 0
		}
	}
	fenceLen := maxRun + 1
	if fenceLen < minMarkdownFence {
		fenceLen = minMarkdownFence
	}
	return strings.Repeat("`", fenceLen)
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
		return Layer{Name: "snippets", Priority: 3, Content: ""}
	}
	var ctxMap map[string]any
	if err := json.Unmarshal(issue.Context, &ctxMap); err != nil {
		return Layer{Name: "snippets", Priority: 3, Content: ""}
	}
	if len(ctxMap) == 0 {
		return Layer{Name: "snippets", Priority: 3, Content: ""}
	}
	var lines []string
	for k, v := range ctxMap {
		lines = append(lines, fmt.Sprintf("%s: %v", k, v))
	}
	sort.Strings(lines)
	return Layer{Name: "snippets", Priority: 3, Content: strings.Join(lines, "\n")}
}

func buildBlockerOutcomes(issue *materialize.Issue, state *materialize.State) Layer {
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
		}
		if outcome == "outcome unknown" && status != "" {
			outcome = fmt.Sprintf("%s (outcome unknown)", status)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", blockerID, outcome))
	}
	content := "## Blocking Issue Outcomes\n" + strings.Join(lines, "\n")
	return Layer{Name: "blocker_outcomes", Priority: 4, Content: content}
}

func buildParentChain(issue *materialize.Issue, graph *dag.Graph, state *materialize.State) Layer {
	var lines []string

	ancestors := graph.Ancestry(issue.ID)
	for _, parentID := range ancestors {
		if len(lines) >= parentChainDisplayCap {
			break
		}
		parentIssue, ok := state.Issues[parentID]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s [%s]", parentID, parentIssue.Title, parentIssue.Status))
	}

	if len(lines) == 0 {
		return Layer{Name: "parent_chain", Priority: 5, Content: ""}
	}
	content := "## Parent Chain\n" + strings.Join(lines, "\n")
	return Layer{Name: "parent_chain", Priority: 5, Content: content}
}

func buildDecisions(issue *materialize.Issue) Layer {
	if len(issue.Decisions) == 0 {
		return Layer{Name: "decisions", Priority: 6, Content: ""}
	}
	var lines []string
	for _, d := range issue.Decisions {
		lines = append(lines, fmt.Sprintf("- %s: %s — %s", d.Topic, d.Choice, d.Rationale))
	}
	content := "## Decisions\n" + strings.Join(lines, "\n")
	return Layer{Name: "decisions", Priority: 6, Content: content}
}

func buildNotes(issue *materialize.Issue) Layer {
	if len(issue.Notes) == 0 {
		return Layer{Name: "notes", Priority: 7, Content: ""}
	}
	notes := make([]materialize.Note, 0, len(issue.Notes))
	for _, note := range issue.Notes {
		if note.Deleted {
			continue
		}
		notes = append(notes, note)
	}
	if len(notes) == 0 {
		return Layer{Name: "notes", Priority: 7, Content: ""}
	}
	if len(notes) > recentNotesDisplayCap {
		notes = notes[len(notes)-recentNotesDisplayCap:]
	}
	var lines []string
	for _, n := range notes {
		ts := time.Unix(n.Timestamp, 0).UTC().Format(time.RFC3339)
		lines = append(lines, fmt.Sprintf("- [%s] %s", ts, n.Msg))
	}
	content := "## Notes\n" + strings.Join(lines, "\n")
	return Layer{Name: "notes", Priority: 7, Content: content}
}

func buildSiblingOutcomes(issue *materialize.Issue, graph *dag.Graph, state *materialize.State) Layer {
	if issue.Parent == "" {
		return Layer{Name: "sibling_outcomes", Priority: 8, Content: ""}
	}

	_, children := graph.Hierarchy(issue.Parent)

	var lines []string
	for _, sibID := range children {
		if sibID == issue.ID {
			continue
		}
		sib, ok := state.Issues[sibID]
		if !ok {
			continue
		}
		if sib.Status == "done" || sib.Status == "merged" {
			outcome := sib.Outcome
			if outcome == "" {
				outcome = "(none)"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", sibID, outcome))
		}
	}
	if len(lines) == 0 {
		return Layer{Name: "sibling_outcomes", Priority: 8, Content: ""}
	}
	content := "## Sibling Outcomes\n" + strings.Join(lines, "\n")
	return Layer{Name: "sibling_outcomes", Priority: 8, Content: content}
}
