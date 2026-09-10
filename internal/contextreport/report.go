// Package contextreport prices static agent-facing artifacts by byte weight
// and the bytes/4 token estimate used by token_budget / render-context.
package contextreport

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BytesPerToken is the token_budget convention: character budget = tokens * 4.
const BytesPerToken = 4

// EstimationMethod is documented in both human and JSON output so a reader
// does not have to look up how estimated_tokens was computed.
const EstimationMethod = "estimated tokens = bytes/4 (integer division), " +
	"matching the token_budget convention used by render-context " +
	"(character budget = tokens * 4). " +
	"Dynamic structured command payloads are out of scope (AOC-S3-T2)."

const (
	ClassSkill    = "skill"
	ClassGlossary = "glossary"
	ClassCommands = "commands"
	ClassConcepts = "concepts"
	ClassUseCases = "use-cases"
)

var requiredDocs = []struct {
	rel   string
	class string
}{
	{rel: "CONTEXT.md", class: ClassGlossary},
	{rel: "docs/commands.md", class: ClassCommands},
	{rel: "docs/concepts.md", class: ClassConcepts},
	{rel: "docs/use-cases.md", class: ClassUseCases},
}

var classOrder = map[string]int{
	ClassSkill:    0,
	ClassGlossary: 1,
	ClassCommands: 2,
	ClassConcepts: 3,
	ClassUseCases: 4,
}

// Artifact is one priced static agent-facing file or skill tree.
type Artifact struct {
	Path            string `json:"path"`
	Class           string `json:"class"`
	Bytes           int    `json:"bytes"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

// Report is the full static context-weight inventory.
type Report struct {
	EstimationMethod     string     `json:"estimation_method"`
	Artifacts            []Artifact `json:"artifacts"`
	TotalBytes           int        `json:"total_bytes"`
	TotalEstimatedTokens int        `json:"total_estimated_tokens"`
}

// EstimateTokens applies the token_budget bytes/4 heuristic.
func EstimateTokens(byteCount int) int {
	return byteCount / BytesPerToken
}

// Collect inventories embedded skills and the canonical agent-facing docs
// under repoRoot. Dynamic structured command payloads are not included.
func Collect(repoRoot string) (Report, error) {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return Report{}, fmt.Errorf("resolve repository path: %w", err)
	}

	var artifacts []Artifact
	skills, err := collectSkills(abs)
	if err != nil {
		return Report{}, err
	}
	artifacts = append(artifacts, skills...)

	docs, err := collectDocs(abs)
	if err != nil {
		return Report{}, err
	}
	artifacts = append(artifacts, docs...)

	sort.Slice(artifacts, func(i, j int) bool {
		oi, oj := classOrder[artifacts[i].Class], classOrder[artifacts[j].Class]
		if oi != oj {
			return oi < oj
		}
		return artifacts[i].Path < artifacts[j].Path
	})

	report := Report{
		EstimationMethod: EstimationMethod,
		Artifacts:        artifacts,
	}
	for _, a := range artifacts {
		report.TotalBytes += a.Bytes
		report.TotalEstimatedTokens += a.EstimatedTokens
	}
	return report, nil
}

func collectSkills(root string) ([]Artifact, error) {
	skillsRoot := filepath.Join(root, "internal", "skillsembed", "skills")
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return nil, fmt.Errorf("read embedded skills directory %s: %w", skillsRoot, err)
	}

	var artifacts []Artifact
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(skillsRoot, entry.Name())
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat %s: %w", filepath.Join(skillDir, "SKILL.md"), err)
		}
		n, err := sumTreeBytes(skillDir)
		if err != nil {
			return nil, err
		}
		rel := filepath.ToSlash(filepath.Join("internal", "skillsembed", "skills", entry.Name()))
		artifacts = append(artifacts, Artifact{
			Path:            rel,
			Class:           ClassSkill,
			Bytes:           n,
			EstimatedTokens: EstimateTokens(n),
		})
	}
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("no embedded skills with SKILL.md under %s", skillsRoot)
	}
	return artifacts, nil
}

func collectDocs(root string) ([]Artifact, error) {
	var artifacts []Artifact
	var missing []string
	for _, doc := range requiredDocs {
		path := filepath.Join(root, filepath.FromSlash(doc.rel))
		data, err := os.ReadFile(path) //nolint:gosec // path is a fixed relative doc inside the repo root
		if err != nil {
			missing = append(missing, doc.rel)
			continue
		}
		n := len(data)
		artifacts = append(artifacts, Artifact{
			Path:            doc.rel,
			Class:           doc.class,
			Bytes:           n,
			EstimatedTokens: EstimateTokens(n),
		})
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required agent-facing docs: %s", strings.Join(missing, ", "))
	}
	return artifacts, nil
}

func sumTreeBytes(dir string) (int, error) {
	total := 0
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		total += int(info.Size())
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("sum skill tree %s: %w", dir, err)
	}
	return total, nil
}

// RenderHuman prints a table plus the estimation method.
func RenderHuman(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Context report (static agent-facing artifacts)\n")
	fmt.Fprintf(&b, "Estimation method: %s\n\n", report.EstimationMethod)
	fmt.Fprintf(&b, "%-52s %-12s %10s %10s\n", "PATH", "CLASS", "BYTES", "EST_TOKENS")
	for _, a := range report.Artifacts {
		fmt.Fprintf(&b, "%-52s %-12s %10d %10d\n", a.Path, a.Class, a.Bytes, a.EstimatedTokens)
	}
	fmt.Fprintf(&b, "%-52s %-12s %10d %10d\n", "TOTAL", "", report.TotalBytes, report.TotalEstimatedTokens)
	return b.String()
}

// RenderJSON encodes the report, including estimation_method.
func RenderJSON(report Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
