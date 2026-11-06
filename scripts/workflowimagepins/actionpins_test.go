package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionPinFixturesReject_REQ_ACTIONS(t *testing.T) {
	t.Parallel()
	root := workflowPinsRepoRoot(t)
	cases := []struct {
		name   string
		substr string
		path   string
	}{
		{name: "action-tag.yml", substr: `uses movable ref "v4"`, path: "jobs.x.steps[0].uses"},
		{name: "action-branch.yml", substr: `uses movable ref "main"`, path: "jobs.x.steps[0].uses"},
		{name: "action-short-sha.yml", substr: "is a short SHA", path: "jobs.x.steps[0].uses"},
		{name: "action-upper-sha.yml", substr: "not a lowercase commit SHA", path: "jobs.x.steps[0].uses"},
		{name: "action-mixed-sha.yml", substr: "not a lowercase commit SHA", path: "jobs.x.steps[0].uses"},
		{name: "action-subdir-tag.yml", substr: `uses movable ref "v1"`, path: "jobs.x.steps[0].uses"},
		{name: "reusable-job-tag.yml", substr: `uses movable ref "v1"`, path: "jobs.call.uses"},
		{name: "reusable-local.yml", substr: "escapes the checkout", path: "jobs.call.uses"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := filepath.Join(root, "scripts/testdata/workflow-image-pins/unpinned", tc.name)
			body, err := os.ReadFile(fixture)
			require.NoError(t, err)
			if tc.name == "reusable-job-tag.yml" || tc.name == "reusable-local.yml" {
				assert.NotContains(t, string(body), "steps:", "job-level uses must be the only uses site")
			}
			code, _, stderr := runWorkflowFixture(t, fixture)
			assert.Equal(t, 1, code)
			assert.Contains(t, stderr, "FAIL: unpinned GitHub Action uses (blocking):")
			assert.Contains(t, stderr, tc.path)
			assert.Contains(t, stderr, tc.substr)
			assert.NotContains(t, stderr, "container image")
		})
	}
}

func TestActionPinFixturesAccept_REQ_ACTIONS(t *testing.T) {
	t.Parallel()
	root := workflowPinsRepoRoot(t)
	for _, name := range []string{
		"action-tag.yml",
		"action-branch.yml",
		"action-short-sha.yml",
		"action-upper-sha.yml",
		"action-mixed-sha.yml",
		"action-subdir-tag.yml",
		"reusable-job-tag.yml",
		"reusable-local.yml",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := filepath.Join(root, "scripts/testdata/workflow-image-pins/pinned", name)
			code, stdout, stderr := runWorkflowFixture(t, fixture)
			assert.Equal(t, 0, code, stderr)
			assert.NotContains(t, stderr, "FAIL:")
			assert.Contains(t, stdout, "external actions are SHA-pinned")
		})
	}
}

func TestCompositeActionPinFixture_REQ_ACTIONS(t *testing.T) {
	t.Parallel()
	root := workflowPinsRepoRoot(t)
	t.Run("unpinned", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runDirFixture(t, filepath.Join(root, "scripts/testdata/workflow-image-pins/unpinned-dirs/composite-pin"))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "runs.steps[0].uses")
		assert.Contains(t, stderr, "actions/checkout@v4")
		assert.Contains(t, stderr, `uses movable ref "v4"`)
	})
	t.Run("pinned", func(t *testing.T) {
		t.Parallel()
		code, stdout, stderr := runDirFixture(t, filepath.Join(root, "scripts/testdata/workflow-image-pins/pinned-dirs/composite-pin"))
		assert.Equal(t, 0, code, stderr)
		assert.Contains(t, stdout, "external actions are SHA-pinned")
	})
}

func TestDockerAndImageAreNotActions_REQ_ACTIONS(t *testing.T) {
	t.Parallel()
	root := workflowPinsRepoRoot(t)
	base := filepath.Join(root, "scripts/testdata/workflow-image-pins")
	t.Run("unpinned docker uses", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowFixture(t, filepath.Join(base, "unpinned/uses-docker.yml"))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "FAIL: unpinned/unknown container image refs (blocking):")
		assert.NotContains(t, stderr, "external action")
		assert.NotContains(t, stderr, "unpinned GitHub Action")
	})
	t.Run("pinned docker uses", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowFixture(t, filepath.Join(base, "pinned/uses-docker.yml"))
		assert.Equal(t, 0, code, stderr)
		assert.NotContains(t, stderr, "external action")
	})
	t.Run("unpinned image key", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowFixture(t, filepath.Join(base, "unpinned/container-string.yml"))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "container image")
		assert.NotContains(t, stderr, "unpinned GitHub Action")
	})
}

func TestActionPinEdges_REQ_ACTIONS(t *testing.T) {
	t.Parallel()
	sha := strings.Repeat("a", 40)
	t.Run("yaml comment is not part of the pin", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("actions/checkout@"+sha+" # v0.0.1", false))
		assert.Equal(t, 0, code, stderr)
	})
	t.Run("39 character sha", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("actions/checkout@"+strings.Repeat("a", 39), false))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "short SHA")
	})
	t.Run("64 character sha", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("actions/checkout@"+strings.Repeat("ab", 32), false))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "64-character SHA")
	})
	t.Run("missing at", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("actions/checkout", false))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "not pinned to a 40-character lowercase commit SHA")
	})
	t.Run("not owner/repo", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("checkout@"+sha, false))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "not an owner/repo action")
	})
	t.Run("empty ref", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("actions/checkout@", false))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "not pinned to a 40-character lowercase commit SHA")
	})
	t.Run("expression", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("${{ matrix.action }}", false))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "not pinned")
	})
	t.Run("yaml parse", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, "jobs: [\n")
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "yaml parse")
	})
	t.Run("jobs not a mapping", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, "name: t\non: push\njobs: []\n")
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "jobs is not a mapping")
	})
	t.Run("empty uses", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, "name: t\non: push\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: \"\"\n")
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "empty uses")
	})
	t.Run("dollar slash local workflow", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("$/.github/workflows/ci.yml", true))
		assert.Equal(t, 0, code, stderr)
	})
	t.Run("dotdot that stays inside", func(t *testing.T) {
		t.Parallel()
		tmp := seededWorkflow(t, "name: t\non: push\njobs:\n  call:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n")
		base := filepath.Base(tmp)
		body := workflowUses("../"+base+"/.github/workflows/ci.yml", true)
		require.NoError(t, os.WriteFile(filepath.Join(tmp, ".github", "workflows", "ci.yml"), []byte(body), 0o644))
		code, _, stderr := runCapture(t, tmp)
		assert.Equal(t, 0, code, stderr)
	})
	t.Run("local workflow missing", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("./.github/workflows/missing.yml", true))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "local workflow")
		assert.Contains(t, stderr, "does not exist")
	})
	t.Run("local action missing", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := runWorkflowBody(t, workflowUses("./actions/missing", false))
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "no action.yml")
	})
}

func TestLocalSymlinkEscape_REQ_ACTIONS(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	leaked := "name: leaked\nruns:\n  using: composite\n  steps:\n    - uses: actions/checkout@v4\n"
	require.NoError(t, os.WriteFile(filepath.Join(outside, "action.yml"), []byte(leaked), 0o644))
	tmp := seededWorkflow(t, "name: t\non: push\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./escape-link\n")
	require.NoError(t, os.Symlink(outside, filepath.Join(tmp, "escape-link")))
	code, _, stderr := runCapture(t, tmp)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "escapes the checkout")
	assert.NotContains(t, stderr, "actions/checkout@v4")
}

func TestFixturesAndGitAreNotProductionFindings_REQ_ACTIONS(t *testing.T) {
	t.Parallel()
	tmp := seededWorkflow(t, "name: t\non: push\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n")
	bad := "name: t\non: push\njobs:\n  x:\n    steps:\n      - uses: actions/checkout@v4\n"
	for _, rel := range []string{
		filepath.Join("testdata", ".github", "workflows", "bad.yml"),
		filepath.Join("scripts", "testdata", ".github", "workflows", "bad.yml"),
		filepath.Join("vendor", ".github", "workflows", "bad.yml"),
		filepath.Join("node_modules", ".github", "workflows", "bad.yml"),
		filepath.Join(".git", "evil.yml"),
		filepath.Join(".git", "action.yml"),
	} {
		dst := filepath.Join(tmp, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
		payload := bad
		if strings.Contains(rel, ".git") {
			payload = ":\n: not yaml\n"
		}
		require.NoError(t, os.WriteFile(dst, []byte(payload), 0o644))
	}
	code, _, stderr := runCapture(t, tmp)
	assert.Equal(t, 0, code, stderr)
	assert.NotContains(t, stderr, "checkout@v4")
	assert.NotContains(t, stderr, "yaml parse")
}

func TestRootActionAndRealRepo_REQ_ACTIONS(t *testing.T) {
	t.Parallel()
	t.Run("root action unpinned", func(t *testing.T) {
		t.Parallel()
		tmp := seededWorkflow(t, "name: t\non: push\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n")
		rootAction := "name: root\nruns:\n  using: composite\n  steps:\n    - uses: actions/checkout@main\n"
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "action.yml"), []byte(rootAction), 0o644))
		code, _, stderr := runCapture(t, tmp)
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, `uses movable ref "main"`)
		assert.Contains(t, stderr, "action.yml")
	})
	t.Run("real checkout has no action-pin findings", func(t *testing.T) {
		t.Parallel()
		findings, err := checkRepo(workflowPinsRepoRoot(t))
		require.NoError(t, err)
		for _, f := range findings {
			assert.NotEqual(t, kindAction, f.Kind, "%s (%s): %s", f.File, f.Path, f.Msg)
			assert.NotEqual(t, kindRef, f.Kind, "%s (%s): %s", f.File, f.Path, f.Msg)
		}
	})
}

func workflowPinsRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	return root
}

func runCapture(t *testing.T, root string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{root}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func seededWorkflow(t *testing.T, body string) string {
	t.Helper()
	tmp := t.TempDir()
	require.NoError(t, seedFixtureRepo(tmp))
	wf := filepath.Join(tmp, ".github", "workflows")
	require.NoError(t, os.MkdirAll(wf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wf, "ci.yml"), []byte(body), 0o644))
	return tmp
}

func runWorkflowBody(t *testing.T, body string) (int, string, string) {
	t.Helper()
	return runCapture(t, seededWorkflow(t, body))
}

func runWorkflowFixture(t *testing.T, fixture string) (int, string, string) {
	t.Helper()
	body, err := os.ReadFile(fixture)
	require.NoError(t, err)
	return runWorkflowBody(t, string(body))
}

func runDirFixture(t *testing.T, src string) (int, string, string) {
	t.Helper()
	tmp := t.TempDir()
	require.NoError(t, seedFixtureRepo(tmp))
	require.NoError(t, copyTree(src, tmp))
	return runCapture(t, tmp)
}

func workflowUses(uses string, jobLevel bool) string {
	if jobLevel {
		return "name: t\non: push\njobs:\n  call:\n    uses: " + uses + "\n"
	}
	return "name: t\non: push\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n"
}
