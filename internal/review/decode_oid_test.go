package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gitoid"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeReviewBundle_ObjectFormatOIDs_REQ_PV70(t *testing.T) {
	t.Parallel()

	t.Run("sha1_40_hex", func(t *testing.T) {
		t.Parallel()
		repo := gittest.Init(t)
		gittest.Git(t, repo, "commit", "--allow-empty", "-m", "root")
		base := gitOutput(t, repo, "rev-parse", "HEAD")
		require.Len(t, base, gitoid.LenSHA1)
		require.True(t, gitoid.Valid(base))

		gittest.Git(t, repo, "commit", "--allow-empty", "-m", "head")
		head := gitOutput(t, repo, "rev-parse", "HEAD")
		require.Len(t, head, gitoid.LenSHA1)
		require.NotEqual(t, base, head)

		data := mustBundleJSONWithSHAs(t, base, head)
		got, err := DecodeReviewBundle(data)
		require.NoError(t, err)
		assert.Equal(t, base, got.Delivery.BaseSHA)
		assert.Equal(t, head, got.Delivery.HeadSHA)
	})

	t.Run("sha256_64_hex", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		gittest.InitSHA256Repo(t, repo)
		gittest.Git(t, repo, "config", "user.email", "t@t")
		gittest.Git(t, repo, "config", "user.name", "t")
		gittest.Git(t, repo, "commit", "--allow-empty", "-m", "root")

		base := gitOutput(t, repo, "rev-parse", "HEAD")
		require.Len(t, base, gitoid.LenSHA256, "mutation to 40-only must fail: real sha256 OID is 64 hex")
		require.True(t, gitoid.Valid(base))

		gittest.Git(t, repo, "commit", "--allow-empty", "-m", "head")
		head := gitOutput(t, repo, "rev-parse", "HEAD")
		require.Len(t, head, gitoid.LenSHA256)

		data := mustBundleJSONWithSHAs(t, base, head)
		got, err := DecodeReviewBundle(data)
		require.NoError(t, err, "decode must accept 64-hex delivery SHAs from a sha256 repo")
		assert.Equal(t, base, got.Delivery.BaseSHA)
		assert.Equal(t, head, got.Delivery.HeadSHA)
	})

	t.Run("uppercase_and_wrong_length_rejected", func(t *testing.T) {
		t.Parallel()
		good := strings.Repeat("b", gitoid.LenSHA1)
		// Loosening gitoid.Valid length OR case must fail these decode checks
		// (internal/review owns the contract — not only gitoid/adapters unit tests).
		for _, bad := range []string{
			strings.Repeat("A", gitoid.LenSHA1),     // uppercase 40
			strings.Repeat("A", gitoid.LenSHA256),   // uppercase 64
			strings.Repeat("a", gitoid.LenSHA1-1),   // 39
			strings.Repeat("a", gitoid.LenSHA1+1),   // 41
			strings.Repeat("a", gitoid.LenSHA256-1), // 63
		} {
			require.False(t, gitoid.Valid(bad), "gitoid.Valid must reject %q", bad)
			data := mustBundleJSONWithSHAs(t, bad, good)
			_, err := DecodeReviewBundle(data)
			require.Error(t, err, "decode must reject base_sha %q", bad)
			assert.Contains(t, strings.ToLower(err.Error()), "sha")
		}
	})
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := adapters.NonInteractiveGitCommand(dir, args...).CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(out)))
	return strings.TrimSpace(string(out))
}

func TestReviewBundleSchemaPatternMatchesGitoid_REQ_PV70(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRootForOID(t)
	raw, err := os.ReadFile(filepath.Join(repoRoot, "docs", "schemas", "review-bundle.schema.json"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	delivery, ok := doc["properties"].(map[string]any)["delivery"].(map[string]any)
	require.True(t, ok)
	props, ok := delivery["properties"].(map[string]any)
	require.True(t, ok)
	for _, field := range []string{"base_sha", "head_sha"} {
		pat, ok := props[field].(map[string]any)["pattern"].(string)
		require.True(t, ok, field)
		assert.Equal(t, gitoid.PatternLower, pat, field+" must equal gitoid.PatternLower (drift fails)")
	}
}

func findRepoRootForOID(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for range 10 {
		if _, err := os.Stat(filepath.Join(cwd, "docs", "schemas")); err == nil {
			return cwd
		}
		cwd = filepath.Dir(cwd)
	}
	t.Fatal("could not find repo root")
	return ""
}

func mustBundleJSONWithSHAs(t *testing.T, base, head string) []byte {
	t.Helper()
	bundle := map[string]any{
		"schema_version": 1,
		"bundle_id":      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"issue": map[string]any{
			"id": "TASK-1", "type": "task", "title": "OID decode", "outcome": "done",
		},
		"contract": map[string]any{
			"definition_of_done": "tests pass",
			"acceptance":         []string{"works"},
		},
		"delivery": map[string]any{
			"base_sha":      base,
			"head_sha":      head,
			"changed_files": []string{"impl.go"},
		},
		"fingerprints": map[string]any{
			"contract": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"delivery": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		},
	}
	data, err := json.Marshal(bundle)
	require.NoError(t, err)
	return data
}
