package skillsembed

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

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
