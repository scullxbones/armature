package skillsembed

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
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

func TestSkillFrontMatterSumUnderCap_REQ_NXTTN_S3_T4(t *testing.T) {
	t.Parallel()

	sum, err := measureModelInvocableFrontMatter(SkillsFS)
	require.NoError(t, err)
	t.Logf("measured model-invocable front matter sum: %d bytes; cap %d", sum, modelInvocableFrontMatterCapBytes)
	require.Greater(t, sum, 0, "embedded model-invocable skills must contribute front matter bytes")
	require.NoError(t, checkModelInvocableFrontMatterCap(SkillsFS, modelInvocableFrontMatterCapBytes))
	require.LessOrEqual(t, sum, modelInvocableFrontMatterCapBytes)
}

func TestFrontMatterBudgetFailsWhenOverCap(t *testing.T) {
	t.Parallel()

	sum, err := measureModelInvocableFrontMatter(SkillsFS)
	require.NoError(t, err)
	require.Greater(t, sum, 0)

	err = checkModelInvocableFrontMatterCap(SkillsFS, sum-1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds cap")
	require.Contains(t, err.Error(), "front matter")
}

func TestFrontMatterBudgetAllowsExactCap(t *testing.T) {
	t.Parallel()

	sum, err := measureModelInvocableFrontMatter(SkillsFS)
	require.NoError(t, err)
	require.NoError(t, checkModelInvocableFrontMatterCap(SkillsFS, sum))
}

func TestMeasureCountsYAMLBetweenFencesOnly(t *testing.T) {
	t.Parallel()

	yamlBody := "name: sample\ndescription: always loaded\n"
	skill := "---\n" + yamlBody + "---\n# huge body\n" + strings.Repeat("x", 5000) + "\n"
	fsys := fstest.MapFS{
		"skills/sample/SKILL.md": &fstest.MapFile{Data: []byte(skill)},
		"skills/sample/references/extra.md": &fstest.MapFile{
			Data: []byte("---\nname: ignored-ref\n---\n" + strings.Repeat("y", 4000)),
		},
		"skills/sample/references/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: nested\ndisable-model-invocation: false\n---\n"),
		},
		"plugin.json": &fstest.MapFile{Data: []byte(`{}`)},
	}

	sum, err := measureModelInvocableFrontMatter(fsys)
	require.NoError(t, err)
	require.Equal(t, len(yamlBody), sum)
}

func TestMeasureExcludesDisableModelInvocation(t *testing.T) {
	t.Parallel()

	invocable := "---\nname: keep\n---\nbody\n"
	hidden := "---\nname: hidden\ndisable-model-invocation: true\n---\nbody\n"
	explicit := "---\nname: explicit\ndisable-model-invocation: false\n---\n"
	noFM := "# test-skill\nno front matter\n"
	fsys := fstest.MapFS{
		"skills/keep/SKILL.md":       &fstest.MapFile{Data: []byte(invocable)},
		"skills/hidden/SKILL.md":     &fstest.MapFile{Data: []byte(hidden)},
		"skills/explicit/SKILL.md":   &fstest.MapFile{Data: []byte(explicit)},
		"skills/test-skill/SKILL.md": &fstest.MapFile{Data: []byte(noFM)},
		"skills/SKILL.md":            &fstest.MapFile{Data: []byte("---\nname: root\n---\n")},
	}

	sum, err := measureModelInvocableFrontMatter(fsys)
	require.NoError(t, err)
	require.Equal(t, len("name: keep\n")+len("name: explicit\ndisable-model-invocation: false\n"), sum)
}

func TestMeasureRejectsUnclosedAndInvalidYAML(t *testing.T) {
	t.Parallel()

	t.Run("unclosed", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"skills/broken/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: broken\n")},
		}
		_, err := measureModelInvocableFrontMatter(fsys)
		require.Error(t, err)
		require.ErrorIs(t, err, errUnclosedFrontMatter)
		require.Contains(t, err.Error(), "skills/broken/SKILL.md")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"skills/bad/SKILL.md": &fstest.MapFile{Data: []byte("---\n: :\n---\n")},
		}
		_, err := measureModelInvocableFrontMatter(fsys)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse front matter")
	})
}

func TestExtractFrontMatterYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		present bool
		err     error
	}{
		{name: "no front matter", in: "# body\n", present: false},
		{name: "opening ellipsis is not front matter", in: "...\nname: x\n---\n", present: false},
		{name: "crlf", in: "---\r\nname: x\r\n---\r\nbody\n", want: "name: x\r\n", present: true},
		{name: "closing document end", in: "---\nname: x\n...\nbody\n", want: "name: x\n", present: true},
		{name: "empty pair", in: "---\n---\nbody\n", want: "", present: true},
		{name: "bare opening fence", in: "---", err: errUnclosedFrontMatter},
		{name: "unclosed last yaml line", in: "---\nname: x", err: errUnclosedFrontMatter},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, present, err := extractFrontMatterYAML([]byte(tc.in))
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.present, present)
			require.Equal(t, tc.want, string(got))
		})
	}
}

func TestMeasurePropagatesWalkError(t *testing.T) {
	t.Parallel()

	_, err := measureModelInvocableFrontMatter(boomFS{})
	require.Error(t, err)
}

func TestMeasurePropagatesReadError(t *testing.T) {
	t.Parallel()

	inner := fstest.MapFS{
		"skills/keep/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: keep\n---\n")},
	}
	_, err := measureModelInvocableFrontMatter(failReadFS{inner: inner, fail: "skills/keep/SKILL.md"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read failed")
}

func TestCheckCapOnEmptyTree(t *testing.T) {
	t.Parallel()
	require.NoError(t, checkModelInvocableFrontMatterCap(fstest.MapFS{}, 0))
}

func TestCheckCapPropagatesMeasureError(t *testing.T) {
	t.Parallel()
	require.Error(t, checkModelInvocableFrontMatterCap(boomFS{}, 10))
}

type boomFS struct{}

func (boomFS) Open(name string) (fs.File, error) {
	return nil, errors.New("open failed")
}

type failReadFS struct {
	inner fs.FS
	fail  string
}

func (f failReadFS) Open(name string) (fs.File, error) {
	if name == f.fail {
		return nil, errors.New("read failed")
	}
	return f.inner.Open(name)
}
