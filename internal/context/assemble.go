// Package context is the Deep Module for Working Memory: it assembles, truncates,
// and renders the layered context bundle passed to workers via render-context.
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

	"github.com/scullxbones/armature/internal/dag"
)

const (
	minMarkdownFence      = 3
	parentChainDisplayCap = 3
	recentNotesDisplayCap = 5
)

type layer struct {
	Name     string `json:"name"`
	DropRank int    `json:"priority"`
	Content  string `json:"content"`
}

type workingMemory struct {
	IssueID string  `json:"issue_id"`
	Layers  []layer `json:"layers"`
}

func assemble(issueID string, issues map[string]IssueFacts, reader fileReader) (*workingMemory, error) {
	issue, ok := issues[issueID]
	if !ok {
		return nil, fmt.Errorf("issue %s not found in state", issueID)
	}

	graph := graphFromFacts(issues)

	var layers []layer

	layers = append(layers, buildCoreSpec(issue))
	layers = append(layers, buildContextFiles(issue, reader))
	layers = append(layers, buildSnippets(issue))
	layers = append(layers, buildBlockerOutcomes(issue, issues))
	parentLayer, err := buildParentChain(issue, graph, issues)
	if err != nil {
		return nil, err
	}
	layers = append(layers, parentLayer)
	layers = append(layers, buildDecisions(issue))
	layers = append(layers, buildNotes(issue))
	siblingLayer, err := buildSiblingOutcomes(issue, graph, issues)
	if err != nil {
		return nil, err
	}
	layers = append(layers, siblingLayer)

	sort.Slice(layers, func(i, j int) bool {
		return layers[i].DropRank < layers[j].DropRank
	})

	return &workingMemory{
		IssueID: issueID,
		Layers:  layers,
	}, nil
}

func inferRepoRoot(stateDir string) string {
	if root := inferRepoRootByPath(stateDir); root != "" {
		return root
	}
	if root := inferRepoRootByGit(stateDir); root != "" {
		return root
	}
	return filepath.Clean(stateDir)
}

func inferRepoRootByPath(stateDir string) string {
	clean := filepath.Clean(stateDir)
	for dir := clean; dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		base := filepath.Base(dir)
		if base == ".arm" || base == stateDirName {
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

func buildContextFiles(issue IssueFacts, reader fileReader) layer {
	if len(issue.ContextFiles) == 0 {
		return layer{Name: "context_files", DropRank: 2, Content: ""}
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
	return layer{
		Name:     "context_files",
		DropRank: 2,
		Content:  "## Context Files\n" + strings.Join(sections, "\n\n"),
	}
}

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

func buildCoreSpec(issue IssueFacts) layer {
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
	return layer{Name: "core_spec", DropRank: 1, Content: content}
}

func buildSnippets(issue IssueFacts) layer {
	if issue.Context == nil {
		return layer{Name: "snippets", DropRank: 3, Content: ""}
	}
	var ctxMap map[string]any
	if err := json.Unmarshal(issue.Context, &ctxMap); err != nil {
		return layer{Name: "snippets", DropRank: 3, Content: ""}
	}
	if len(ctxMap) == 0 {
		return layer{Name: "snippets", DropRank: 3, Content: ""}
	}
	var lines []string
	for k, v := range ctxMap {
		lines = append(lines, fmt.Sprintf("%s: %v", k, v))
	}
	sort.Strings(lines)
	return layer{Name: "snippets", DropRank: 3, Content: strings.Join(lines, "\n")}
}

func buildBlockerOutcomes(issue IssueFacts, issues map[string]IssueFacts) layer {
	if len(issue.BlockedBy) == 0 {
		return layer{Name: "blocker_outcomes", DropRank: 3, Content: ""}
	}
	var lines []string
	for _, blockerID := range issue.BlockedBy {
		outcome := "outcome unknown"
		var status string
		if blocker, ok := issues[blockerID]; ok {
			status = blocker.Status
			if blocker.Outcome != "" {
				outcome = blocker.Outcome
			}
		}
		outcome = blockerDisplay(outcome, status)
		lines = append(lines, fmt.Sprintf("- %s: %s", blockerID, outcome))
	}
	content := "## Blocking Issue Outcomes\n" + strings.Join(lines, "\n")
	return layer{Name: "blocker_outcomes", DropRank: 4, Content: content}
}

func buildParentChain(issue IssueFacts, graph *dag.Graph, issues map[string]IssueFacts) (layer, error) {
	empty := layer{Name: "parent_chain", DropRank: 5, Content: ""}
	if graph == nil {
		return empty, fmt.Errorf("parent chain: issue graph is missing")
	}
	var lines []string

	ancestors := graph.Ancestry(issue.ID)
	for _, parentID := range ancestors {
		if len(lines) >= parentChainDisplayCap {
			break
		}
		parentIssue, ok := issues[parentID]
		if !ok {
			return empty, fmt.Errorf("parent chain: graph node %s is missing from state", parentID)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s [%s]", parentID, parentIssue.Title, parentIssue.Status))
	}

	if len(lines) == 0 {
		return empty, nil
	}
	content := "## Parent Chain\n" + strings.Join(lines, "\n")
	return layer{Name: "parent_chain", DropRank: 5, Content: content}, nil
}

func buildDecisions(issue IssueFacts) layer {
	if len(issue.Decisions) == 0 {
		return layer{Name: "decisions", DropRank: 6, Content: ""}
	}
	var lines []string
	for _, d := range issue.Decisions {
		lines = append(lines, fmt.Sprintf("- %s: %s — %s", d.Topic, d.Choice, d.Rationale))
	}
	content := "## Decisions\n" + strings.Join(lines, "\n")
	return layer{Name: "decisions", DropRank: 6, Content: content}
}

func buildNotes(issue IssueFacts) layer {
	if len(issue.Notes) == 0 {
		return layer{Name: "notes", DropRank: 7, Content: ""}
	}
	notes := make([]NoteFact, 0, len(issue.Notes))
	for _, note := range issue.Notes {
		if note.Deleted {
			continue
		}
		notes = append(notes, note)
	}
	if len(notes) == 0 {
		return layer{Name: "notes", DropRank: 7, Content: ""}
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
	return layer{Name: "notes", DropRank: 7, Content: content}
}

func buildSiblingOutcomes(issue IssueFacts, graph *dag.Graph, issues map[string]IssueFacts) (layer, error) {
	empty := layer{Name: "sibling_outcomes", DropRank: 8, Content: ""}
	if issue.Parent == "" {
		return empty, nil
	}
	if graph == nil {
		return empty, fmt.Errorf("sibling outcomes: issue graph is missing")
	}

	_, children := graph.Hierarchy(issue.Parent)

	var lines []string
	for _, sibID := range children {
		if sibID == issue.ID {
			continue
		}
		sib, ok := issues[sibID]
		if !ok {
			return empty, fmt.Errorf("sibling outcomes: graph node %s is missing from state", sibID)
		}
		if sib.Status == statusDone || sib.Status == statusMerged {
			outcome := sib.Outcome
			if outcome == "" {
				outcome = "(none)"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", sibID, outcome))
		}
	}
	if len(lines) == 0 {
		return empty, nil
	}
	content := "## Sibling Outcomes\n" + strings.Join(lines, "\n")
	return layer{Name: "sibling_outcomes", DropRank: 8, Content: content}, nil
}

func blockerDisplay(outcome, status string) string {
	if outcome == "outcome unknown" && status != "" {
		return fmt.Sprintf("%s (outcome unknown)", status)
	}
	return outcome
}
