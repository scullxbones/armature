package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	skillFenceOpenRe = regexp.MustCompile("(?m)^\\s*```(\\w*)\\s*$")
	backtickArmRe    = regexp.MustCompile("`arm [^`]+`")
)

func TestSkillsLeadWithPavedRoad_REQ_NXTTN_S4_T3(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")

	escapePaths := escapeHatchCommandPaths()

	t.Run("workflowLeadsWithPavedRoadAndLabelsEscapeHatches", func(t *testing.T) {
		body := readRepoFile(t, filepath.Join(root, "docs", "agents", "workflow.md"))
		headings := markdownH2Headings(body)
		require.NotEmpty(t, headings, "workflow.md must have ## headings")
		require.Equal(t, "Paved road", headings[0], "workflow.md must lead with the paved-road pipeline")
		var escapeIdx = -1
		for i, h := range headings {
			if strings.Contains(strings.ToLower(h), "escape hatch") {
				escapeIdx = i
				break
			}
		}
		require.Greater(t, escapeIdx, 0, "workflow.md must demote alternatives to a labeled escape-hatch section")
		require.Contains(t, body, escapeHatchHelpMarker)
	})

	t.Run("baseSkillTeachesOnlyPavedRoadCommands", func(t *testing.T) {
		body := readRepoFile(t, filepath.Join(root, "internal", "skillsembed", "skills", "armature", "SKILL.md"))
		for _, path := range escapePaths {
			needle := "arm " + path
			require.NotContains(t, body, needle,
				"base armature skill must not teach off-road command %q", needle)
		}
	})

	t.Run("skillsAndAgentDocsLabelEscapeHatchRecommendations", func(t *testing.T) {
		var files []string
		err := filepath.Walk(filepath.Join(root, "internal", "skillsembed", "skills"), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if info.Name() == "SKILL.md" || strings.HasSuffix(info.Name(), ".md") {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					return relErr
				}
				// Base skill is the "paved only" surface; other skills may
				// document hatches if they label them.
				if rel == filepath.Join("internal", "skillsembed", "skills", "armature", "SKILL.md") {
					return nil
				}
				if rel == filepath.Join("internal", "skillsembed", "skills", "test-skill", "SKILL.md") {
					return nil
				}
				files = append(files, rel)
			}
			return nil
		})
		require.NoError(t, err)
		agentDocs, err := filepath.Glob(filepath.Join(root, "docs", "agents", "*.md"))
		require.NoError(t, err)
		for _, p := range agentDocs {
			rel, relErr := filepath.Rel(root, p)
			require.NoError(t, relErr)
			files = append(files, rel)
		}
		sort.Strings(files)
		require.NotEmpty(t, files)

		var unlabeled []string
		for _, rel := range files {
			body := readRepoFile(t, filepath.Join(root, rel))
			unlabeled = append(unlabeled, unlabeledEscapeHatchHits(rel, body, escapePaths)...)
		}
		require.Empty(t, unlabeled, "off-road arm commands must be labeled %s:\n%s",
			escapeHatchHelpMarker, strings.Join(unlabeled, "\n"))
	})

	t.Run("plannerLoopRegistersSourcesBeforeApply", func(t *testing.T) {
		body := readRepoFile(t, filepath.Join(root, "internal", "skillsembed", "skills", "armature-planner", "SKILL.md"))
		start := strings.Index(body, "```dot")
		require.GreaterOrEqual(t, start, 0, "planner skill must include a DOT loop diagram")
		end := strings.Index(body[start:], "```\n")
		require.GreaterOrEqual(t, end, 0, "planner DOT fence must close")
		diagram := body[start : start+end]

		require.Contains(t, diagram, `"Start: objective/spec" -> "sources add/sync"`,
			"source registration must be the first loop step")
		require.Contains(t, diagram, `"cite at apply" -> "dag apply --dry-run"`,
			"plan source citation must precede dry-run/apply")
		require.NotContains(t, diagram, `"dag transition" -> "sources add/sync"`,
			"sources add/sync must not follow dag apply/transition")
		require.NotContains(t, diagram, `"dag apply --plan plan.json" -> "sources add/sync"`)
	})
}

func escapeHatchCommandPaths() []string {
	var paths []string
	for path, class := range pavedRoadCommands {
		if path == "" {
			continue
		}
		if class.Kind == pavedRoadKindEscape {
			paths = append(paths, path)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) > len(paths[j])
		}
		return paths[i] < paths[j]
	})
	return paths
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func markdownH2Headings(body string) []string {
	var headings []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ") {
			headings = append(headings, strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		}
	}
	return headings
}

func unlabeledEscapeHatchHits(rel, body string, escapePaths []string) []string {
	var hits []string
	hits = append(hits, unlabeledEscapeHatchFences(rel, body, escapePaths)...)
	hits = append(hits, unlabeledEscapeHatchBackticks(rel, body, escapePaths)...)
	return hits
}

func unlabeledEscapeHatchFences(rel, body string, escapePaths []string) []string {
	var hits []string
	lines := strings.Split(body, "\n")
	inBlock := false
	lang := ""
	var block []string
	start := 0
	for i, line := range lines {
		if !inBlock {
			m := skillFenceOpenRe.FindStringSubmatch(line)
			if m != nil {
				inBlock = true
				lang = m[1]
				block = nil
				start = i + 1
			}
			continue
		}
		if strings.TrimSpace(line) == "```" {
			if lang == "" || lang == "bash" || lang == "sh" {
				blockBody := strings.Join(block, "\n")
				if !strings.Contains(blockBody, escapeHatchHelpMarker) {
					for _, cmd := range armInvocationsInText(blockBody, escapePaths) {
						hits = append(hits, fmt.Sprintf("%s:fence:%d: %s", rel, start, cmd))
					}
				}
			}
			inBlock = false
			lang = ""
			block = nil
			continue
		}
		block = append(block, line)
	}
	return hits
}

func unlabeledEscapeHatchBackticks(rel, body string, escapePaths []string) []string {
	var hits []string
	lines := strings.Split(body, "\n")
	inFence := false
	for i, line := range lines {
		if skillFenceOpenRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.Contains(line, escapeHatchHelpMarker) {
			continue
		}
		for _, raw := range backtickArmRe.FindAllString(line, -1) {
			inner := strings.Trim(raw, "`")
			for _, cmd := range armInvocationsInText(inner, escapePaths) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s in %s", rel, i+1, cmd, raw))
			}
		}
	}
	return hits
}

func armInvocationsInText(text string, escapePaths []string) []string {
	var found []string
	seen := map[string]bool{}
	lower := text
	for _, path := range escapePaths {
		needle := "arm " + path
		idx := 0
		for {
			at := strings.Index(lower[idx:], needle)
			if at < 0 {
				break
			}
			at += idx
			end := at + len(needle)
			if end < len(lower) {
				next := lower[end]
				if next == '-' || (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') {
					idx = at + 1
					continue
				}
			}
			if !seen[needle] {
				seen[needle] = true
				found = append(found, needle)
			}
			idx = at + 1
		}
	}
	return found
}
