package skillsembed

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"gopkg.in/yaml.v3"
)

// modelInvocableFrontMatterCapBytes is the skill-load ratchet for the sum of
// YAML front-matter bytes across model-invocable embedded SKILL.md files.
//
// Measured sum when the gate was introduced: 2772 bytes
// (armature 265, activity-indexer 386, auditor 333, coordinator 407,
// planner 374, reviewer 638, worker 369). test-skill has no front matter
// and does not contribute. No skill currently sets disable-model-invocation.
//
// The cap is seeded at that measured sum. This is a starting ratchet for
// always-loaded skill metadata, not a runtime CLI economics budget.
const modelInvocableFrontMatterCapBytes = 2772

var errUnclosedFrontMatter = errors.New("unclosed YAML front matter")

type skillFrontMatter struct {
	DisableModelInvocation bool `yaml:"disable-model-invocation"`
}

func checkModelInvocableFrontMatterCap(fsys fs.FS, capBytes int) error {
	sum, err := measureModelInvocableFrontMatter(fsys)
	if err != nil {
		return err
	}
	if sum > capBytes {
		return fmt.Errorf("model-invocable skill front matter sum %d bytes exceeds cap %d bytes", sum, capBytes)
	}
	return nil
}

func measureModelInvocableFrontMatter(fsys fs.FS) (int, error) {
	sum := 0
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !isBundledSkillMarkdown(p) {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		n, include, err := modelInvocableFrontMatterBytes(data)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if include {
			sum += n
		}
		return nil
	})
	return sum, err
}

func isBundledSkillMarkdown(p string) bool {
	return path.Base(p) == "SKILL.md" && path.Dir(path.Dir(p)) == "skills"
}

func modelInvocableFrontMatterBytes(data []byte) (int, bool, error) {
	yamlBytes, present, err := extractFrontMatterYAML(data)
	if err != nil {
		return 0, false, err
	}
	if !present {
		return 0, false, nil
	}
	if len(bytes.TrimSpace(yamlBytes)) > 0 {
		var meta skillFrontMatter
		if err := yaml.Unmarshal(yamlBytes, &meta); err != nil {
			return 0, false, fmt.Errorf("parse front matter: %w", err)
		}
		if meta.DisableModelInvocation {
			return 0, false, nil
		}
	}
	return len(yamlBytes), true, nil
}

func extractFrontMatterYAML(content []byte) ([]byte, bool, error) {
	line, rest, foundNL := consumeLine(content)
	if !bytes.Equal(line, []byte("---")) {
		return nil, false, nil
	}
	if !foundNL {
		return nil, false, errUnclosedFrontMatter
	}
	var yamlBytes []byte
	leftover := rest
	for {
		line, next, foundNL := consumeLine(leftover)
		if bytes.Equal(line, []byte("---")) || bytes.Equal(line, []byte("...")) {
			return yamlBytes, true, nil
		}
		if !foundNL {
			return nil, false, errUnclosedFrontMatter
		}
		consumed := len(leftover) - len(next)
		yamlBytes = append(yamlBytes, leftover[:consumed]...)
		leftover = next
	}
}

func consumeLine(b []byte) (line, rest []byte, foundNL bool) {
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return bytes.TrimSuffix(b, []byte("\r")), nil, false
	}
	return bytes.TrimSuffix(b[:i], []byte("\r")), b[i+1:], true
}
